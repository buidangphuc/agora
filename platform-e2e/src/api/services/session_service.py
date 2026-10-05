"""Session service client for platform.identity.v1.SessionService.

Every RPC acts on the caller's own sessions, so build one instance per token
(`SessionService(token=...)`) rather than going through the shared ServiceFactory.
"""

from __future__ import annotations

from typing import Any

import httpx

from src.api.services.base_service import BaseService
from src.constants import gateway_endpoints as ep


class SessionService(BaseService):
    def list_sessions(self) -> list[dict[str, Any]]:
        """The caller's sessions (newest activity first)."""
        return self.post(ep.SESSION_LIST, {}).get("sessions", [])

    def revoke_session(self, session_id: str) -> dict[str, Any]:
        return self.post(ep.SESSION_REVOKE, {"sessionId": session_id})

    def list_sessions_raw(self) -> httpx.Response:
        """ListSessions WITHOUT raising on 4xx: the status code is the assertion."""
        return self.send("POST", ep.SESSION_LIST, json_body={})
