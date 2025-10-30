"""Registry semantic search scenario."""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass
from typing import List, Sequence

from ..client import RegistryClient


LOGGER = logging.getLogger("scenario.registry_semantic_search")


@dataclass
class ScenarioConfig:
    tool_owner: str
    primary_tool_name: str
    primary_description: str
    policy_principal: str
    audit_actor: str
    summarizer_variants: Sequence[str]
    other_tools: Sequence[str]
    suffix: str
    vectorizer_disabled: bool


def _strip_suffix(value: str, suffix: str) -> str:
    if suffix and value.endswith(f"-{suffix}"):
        return value[: -(len(suffix) + 1)]
    return value


def _extract_tool_id(response: str) -> str:
    payload = json.loads(response)
    return payload["tool"]["id"]


def _assert_primary_registration(response: str, owner: str, name: str, principal: str) -> str:
    payload = json.loads(response)
    tool = payload["tool"]
    policies = payload.get("policies", [])
    if tool.get("owner") != owner:
        raise AssertionError("owner mismatch")
    if tool.get("name") != name:
        raise AssertionError("name mismatch")
    if len(policies) != 1 or policies[0].get("principal") != principal:
        raise AssertionError("policy mismatch")
    return tool["id"]


def run(client: RegistryClient, config: ScenarioConfig) -> List[str]:
    created_tool_ids: List[str] = []
    summarizer_ids: List[str] = []

    LOGGER.info("Registering primary tool %s", config.primary_tool_name)
    primary_payload = {
        "owner": config.tool_owner,
        "name": config.primary_tool_name,
        "description": config.primary_description,
        "inputs": {"text": "string"},
        "outputs": {"summary": "string"},
        "policies": [
            {
                "principal": config.policy_principal,
                "allowed_scope": ["summaries"],
                "conditions": {"region": "us-east"},
            }
        ],
    }
    status, body = client.request("POST", "/v1/tools", payload=primary_payload)
    if status != 201:
        raise AssertionError(f"primary tool creation failed: status={status} body={body}")
    primary_id = _assert_primary_registration(body, config.tool_owner, config.primary_tool_name, config.policy_principal)
    created_tool_ids.append(primary_id)
    summarizer_ids.append(primary_id)

    LOGGER.info("Registering %d summarizer variants", len(config.summarizer_variants))
    for entry in config.summarizer_variants:
        key, description = entry.split("::", maxsplit=1)
        payload = {
            "owner": config.tool_owner,
            "name": f"vector-summarize-{key}-{config.suffix}",
            "description": description,
            "inputs": {"text": "string"},
            "outputs": {"summary": "string"},
        }
        status, body = client.request("POST", "/v1/tools", payload=payload)
        if status != 201:
            raise AssertionError(f"variant creation failed: status={status} body={body}")
        tool_id = _extract_tool_id(body)
        created_tool_ids.append(tool_id)
        summarizer_ids.append(tool_id)
        LOGGER.debug("Registered summarizer variant %s (%s)", _strip_suffix(payload["name"], config.suffix), tool_id)

    LOGGER.info("Registering %d auxiliary tools", len(config.other_tools))
    for entry in config.other_tools:
        key, description = entry.split("::", maxsplit=1)
        payload = {
            "owner": config.tool_owner,
            "name": f"{key}-{config.suffix}",
            "description": description,
            "inputs": {"text": "string"},
            "outputs": {"result": "string"},
        }
        status, body = client.request("POST", "/v1/tools", payload=payload)
        if status != 201:
            raise AssertionError(f"auxiliary tool creation failed: status={status} body={body}")
        tool_id = _extract_tool_id(body)
        created_tool_ids.append(tool_id)
        LOGGER.debug("Registered auxiliary tool %s (%s)", _strip_suffix(payload["name"], config.suffix), tool_id)

    search_payload = {
        "description": config.primary_description,
        "limit": 3,
    }
    LOGGER.info("Invoking semantic search for description: %s", config.primary_description)
    status, body = client.request("POST", "/v1/tools/search", payload=search_payload)
    if config.vectorizer_disabled:
        if status != 503:
            raise AssertionError(f"expected 503 when vectorizer disabled, got {status}")
        payload = json.loads(body or "{}")
        if payload.get("error") != "search tools failed":
            raise AssertionError("unexpected error payload")
        details = payload.get("details", "").lower()
        if "vectorizer" not in details:
            raise AssertionError("error details should reference vectorizer")
        LOGGER.info("Search returned expected 503 because vectorizer is disabled")
    else:
        if status != 200:
            raise AssertionError(f"expected 200 from search endpoint, got {status}")
        results = json.loads(body or "[]")
        returned = {item.get("id") for item in results}
        expected = set(summarizer_ids)
        LOGGER.info("Search returned %d results", len(returned))
        if len(returned) != len(expected):
            raise AssertionError(f"expected {len(expected)} summarizer tools, got {len(returned)}")
        if expected - returned:
            raise AssertionError(f"missing summarizer ids: {expected - returned}")
        if returned - expected:
            raise AssertionError(f"unexpected tool ids in search results: {returned - expected}")
        for item in results:
            LOGGER.info("Match: %s — %s", _strip_suffix(item.get("name", ""), config.suffix), item.get("description"))
        LOGGER.debug("Search result IDs: %s", sorted(returned))

    LOGGER.info("Listing tools to confirm primary registration")
    status, body = client.get_with_query("/v1/tools", {"owner": config.tool_owner, "name": config.primary_tool_name})
    if status != 200:
        raise AssertionError(f"list tools failed: status={status}")
    tools = json.loads(body or "[]")
    if not any(item.get("id") == primary_id for item in tools):
        raise AssertionError("primary tool missing from list response")

    LOGGER.info("Fetching tool details for %s", primary_id)
    status, body = client.request("GET", f"/v1/tools/{primary_id}")
    if status != 200:
        raise AssertionError(f"get tool failed: status={status}")
    payload = json.loads(body)
    tool = payload.get("tool", {})
    policies = payload.get("policies", [])
    audits = payload.get("audits", [])
    if tool.get("name") != config.primary_tool_name:
        raise AssertionError("tool name mismatch")
    if len(policies) != 1 or policies[0].get("principal") != config.policy_principal:
        raise AssertionError("policy mismatch")
    if audits:
        raise AssertionError("expected no audits prior to creation")

    LOGGER.info("Creating audit entry for %s", primary_id)
    audit_payload = {
        "event_type": "invocation",
        "actor": config.audit_actor,
        "payload": {"status": "success"},
    }
    status, body = client.request("POST", f"/v1/tools/{primary_id}/audits", payload=audit_payload)
    if status != 201:
        raise AssertionError(f"audit creation failed: status={status} body={body}")
    audit_response = json.loads(body)
    if audit_response.get("actor") != config.audit_actor:
        raise AssertionError("audit actor mismatch")
    if audit_response.get("event_type") != "invocation":
        raise AssertionError("audit event type mismatch")

    LOGGER.info("Fetching tool details after audit for %s", primary_id)
    status, body = client.request("GET", f"/v1/tools/{primary_id}")
    if status != 200:
        raise AssertionError(f"get tool after audit failed: status={status}")
    payload = json.loads(body)
    audits = payload.get("audits", [])
    if not audits:
        raise AssertionError("expected audits after recording")
    latest = audits[0]
    if latest.get("actor") != config.audit_actor:
        raise AssertionError("audit actor mismatch after retrieval")
    if latest.get("event_type") != "invocation":
        raise AssertionError("audit event type mismatch after retrieval")

    LOGGER.info("Scenario completed; created %d tools", len(created_tool_ids))
    return created_tool_ids
