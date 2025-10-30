"""HTTP client helpers for talking to the registry API."""
from __future__ import annotations

import json
import logging
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, Iterable, Optional, Tuple

LOGGER = logging.getLogger(__name__)


class RegistryClient:
    """Minimal HTTP client for the registry."""

    def __init__(self, base_url: str, timeout: float = 20.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def request(
        self,
        method: str,
        path: str,
        payload: Optional[Any] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> Tuple[int, str]:
        if not path.startswith("/"):
            path = "/" + path
        url = f"{self.base_url}{path}"

        data: Optional[bytes] = None
        request_headers: Dict[str, str] = {} if headers is None else dict(headers)

        if payload is not None:
            if isinstance(payload, (dict, list)):
                data = json.dumps(payload).encode("utf-8")
                request_headers.setdefault("Content-Type", "application/json")
            elif isinstance(payload, (str, bytes)):
                data = payload.encode("utf-8") if isinstance(payload, str) else payload
                request_headers.setdefault("Content-Type", "application/json")
            else:
                raise TypeError(f"Unsupported payload type: {type(payload)!r}")

        req = urllib.request.Request(url, data=data, headers=request_headers, method=method.upper())
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                body = resp.read().decode("utf-8")
                return resp.getcode(), body
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            LOGGER.debug("HTTP %s %s failed: %s", method, url, exc)
            return exc.code, body

    def request_json(
        self,
        method: str,
        path: str,
        payload: Optional[Any] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> Tuple[int, Any]:
        status, body = self.request(method, path, payload=payload, headers=headers)
        parsed = None
        if body:
            try:
                parsed = json.loads(body)
            except json.JSONDecodeError as err:
                raise ValueError(f"Invalid JSON response from {path}: {err}") from err
        return status, parsed

    def get_with_query(self, path: str, params: Optional[Dict[str, str]] = None) -> Tuple[int, str]:
        query = ""
        if params:
            query = "?" + urllib.parse.urlencode(params)
        return self.request("GET", f"{path}{query}")

    def delete_tool(self, tool_id: str) -> None:
        status, body = self.request("DELETE", f"/v1/tools/{tool_id}")
        if status not in (200, 202, 204, 404):
            LOGGER.warning("Failed to delete tool %s: status=%s body=%s", tool_id, status, body)
