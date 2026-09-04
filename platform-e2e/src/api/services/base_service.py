"""HTTP base for gateway Connect/JSON services (mirrors bds axios `BaseService`).

All marketplace RPCs are `POST /<pkg>.<Service>/<Method>` with a JSON body. This
wrapper centralizes the client, bearer auth, timing logs, and a reproducible cURL
dump when DEBUG is on.
"""

from __future__ import annotations

import json
import shlex
import time
from typing import Any

import httpx

from config.settings import get_settings


class GatewayError(RuntimeError):
    """Raised when a gateway call returns a non-2xx response."""

    def __init__(self, endpoint: str, status: int, body: str) -> None:
        super().__init__(f"{endpoint} -> HTTP {status}: {body[:500]}")
        self.status = status
        self.body = body


class BaseService:
    def __init__(self, token: str | None = None) -> None:
        settings = get_settings()
        self._debug = settings.debug
        self._token = token
        self._client = httpx.Client(
            base_url=settings.gateway_url,
            timeout=30.0,
            headers={"Content-Type": "application/json"},
        )

    def set_token(self, token: str | None) -> None:
        self._token = token

    def close(self) -> None:
        self._client.close()

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self._token:
            headers["Authorization"] = f"bearer {self._token}"
        return headers

    def post(self, endpoint: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        body = payload or {}
        headers = self._headers()
        if self._debug:
            self._log_curl(endpoint, headers, body)

        started = time.perf_counter()
        resp = self._client.post(endpoint, json=body, headers=headers)
        elapsed_ms = (time.perf_counter() - started) * 1000
        if self._debug:
            print(f"[api] POST {endpoint} -> {resp.status_code} ({elapsed_ms:.0f}ms)")

        if resp.status_code >= 400:
            raise GatewayError(endpoint, resp.status_code, resp.text)
        return resp.json() if resp.content else {}

    def get(self, endpoint: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        """GET a raw edge REST endpoint (e.g. /api/admin/metrics). Raises on non-2xx."""
        headers = self._headers()
        resp = self._client.get(endpoint, params=params, headers=headers)
        if self._debug:
            print(f"[api] GET {endpoint} -> {resp.status_code}")
        if resp.status_code >= 400:
            raise GatewayError(endpoint, resp.status_code, resp.text)
        return resp.json() if resp.content else {}

    def send(
        self,
        method: str,
        endpoint: str,
        *,
        json_body: dict[str, Any] | None = None,
        content: str | bytes | None = None,
        extra_headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Low-level request returning the raw response WITHOUT raising on 4xx/5xx.

        Used by edge collectors (e.g. POST /api/track) where the status code itself
        is the assertion target (204 accepted, 4xx rejected).
        """
        headers = self._headers()
        if content is not None and json_body is None:
            # Beacon transport uses text/plain to avoid a CORS preflight.
            headers["Content-Type"] = "text/plain;charset=UTF-8"
        if extra_headers:
            headers.update(extra_headers)
        resp = self._client.request(
            method, endpoint, json=json_body, content=content, headers=headers
        )
        if self._debug:
            print(f"[api] {method} {endpoint} -> {resp.status_code}")
        return resp

    def _log_curl(self, endpoint: str, headers: dict[str, str], body: dict[str, Any]) -> None:
        base = str(self._client.base_url).rstrip("/")
        header_args = " ".join(f"-H {shlex.quote(f'{k}: {v}')}" for k, v in headers.items())
        data = shlex.quote(json.dumps(body))
        print(f"[curl] curl -X POST {base}{endpoint} {header_args} -d {data}")
