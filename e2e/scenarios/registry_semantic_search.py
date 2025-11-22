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
    summarizer_variants: Sequence[str]
    other_tools: Sequence[str]
    suffix: str


def _strip_suffix(value: str, suffix: str) -> str:
    if suffix and value.endswith(f"-{suffix}"):
        return value[: -(len(suffix) + 1)]
    return value


def run(client: RegistryClient, config: ScenarioConfig) -> List[str]:
    created_tools: List[str] = []
    summarizer_names: List[str] = []

    LOGGER.info("Registering primary tool %s", config.primary_tool_name)
    primary_payload = {
        "owner": config.tool_owner,
        "name": config.primary_tool_name,
        "description": config.primary_description,
    }
    status, body = client.request("POST", "/v1/tools", payload=primary_payload)
    if status != 201:
        raise AssertionError(f"primary tool creation failed: status={status} body={body}")
    created_tools.append(primary_payload["name"])
    summarizer_names.append(primary_payload["name"])

    LOGGER.info("Registering %d summarizer variants", len(config.summarizer_variants))
    for entry in config.summarizer_variants:
        key, description = entry.split("::", maxsplit=1)
        payload = {
            "owner": config.tool_owner,
            "name": f"vector-summarize-{key}-{config.suffix}",
            "description": description,
        }
        status, body = client.request("POST", "/v1/tools", payload=payload)
        if status != 201:
            raise AssertionError(f"variant creation failed: status={status} body={body}")
        created_tools.append(payload["name"])
        summarizer_names.append(payload["name"])
        LOGGER.debug("Registered summarizer variant %s", _strip_suffix(payload["name"], config.suffix))

    LOGGER.info("Registering %d auxiliary tools", len(config.other_tools))
    for entry in config.other_tools:
        key, description = entry.split("::", maxsplit=1)
        payload = {
            "owner": config.tool_owner,
            "name": f"{key}-{config.suffix}",
            "description": description,
        }
        status, body = client.request("POST", "/v1/tools", payload=payload)
        if status != 201:
            raise AssertionError(f"auxiliary tool creation failed: status={status} body={body}")
        created_tools.append(payload["name"])
        LOGGER.debug("Registered auxiliary tool %s", _strip_suffix(payload["name"], config.suffix))

    search_payload = {
        "description": config.primary_description,
        "limit": 3,
    }
    LOGGER.info("Invoking semantic search for description: %s", config.primary_description)
    status, body = client.request("POST", "/v1/tools/search", payload=search_payload)
    if status != 200:
        raise AssertionError(f"expected 200 from search endpoint, got {status}")
    results = json.loads(body or "[]")
    returned = {item.get("tool", {}).get("name") for item in results}
    LOGGER.info("Search returned %d results", len(returned))
    for item in results:
        tool = item.get("tool", {})
        LOGGER.info("Match: %s", _strip_suffix(tool.get("name", ""), config.suffix))

    LOGGER.info("Listing tools to confirm primary registration")
    status, body = client.get_with_query("/v1/tools", {"owner": config.tool_owner, "name": config.primary_tool_name})
    if status != 200:
        raise AssertionError(f"list tools failed: status={status}")
    tools = json.loads(body or "[]")
    if not any(item.get("name") == config.primary_tool_name for item in tools):
        raise AssertionError("primary tool missing from list response")

    LOGGER.info("Fetching tool details for %s", config.primary_tool_name)
    status, body = client.request("GET", f"/v1/tools/{config.tool_owner}/{config.primary_tool_name}")
    if status != 200:
        raise AssertionError(f"get tool failed: status={status}")
    payload = json.loads(body)
    tool = payload
    if tool.get("name") != config.primary_tool_name:
        raise AssertionError("tool name mismatch")

    LOGGER.info("Scenario completed; created %d tools", len(created_tools))
    return created_tools
