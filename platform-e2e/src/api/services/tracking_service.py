"""Tracking edge-collector client (emit-tracking-events).

Wraps the gateway edge endpoint `POST /api/track`. The beacon carries only
behavioral context (type, listing/session/anonymous ids, path, referrer, result
position, query, properties) — never authenticated user identity, which the
gateway stamps onto the `EventEnvelope.principal` from the resolved principal.

The endpoint answers 204 on accept (best-effort produce) and a 4xx for a
malformed / unknown-type beacon. Both are asserted via the returned status code,
so this client uses `BaseService.send` (no-raise) rather than `post`.
"""

from __future__ import annotations

from typing import Any

from src.constants import gateway_endpoints as ep

from .base_service import BaseService

# Beacon action types the collector recognises -> spec EventType mapping.
VIEW = "view"
CLICK = "click"
ADD_TO_CART = "add_to_cart"
IMPRESSION = "impression"


class TrackingService(BaseService):
    def emit(
        self,
        event_type: str,
        *,
        listing_id: str = "",
        session_id: str = "",
        anonymous_id: str = "",
        page: str = "",
        referrer: str = "",
        position: int | None = None,
        query: str = "",
        properties: dict[str, Any] | None = None,
    ) -> int:
        """POST a well-formed browsing beacon. Returns the HTTP status (204 = accepted)."""
        body: dict[str, Any] = {
            "type": event_type,
            "listingId": listing_id,
            "sessionId": session_id,
            "anonymousId": anonymous_id,
            "path": page,
            "referrer": referrer,
            "query": query,
            "properties": properties or {},
        }
        if position is not None:
            body["position"] = position
        resp = self.send("POST", ep.TRACK, json_body=body)
        return resp.status_code

    def emit_raw(self, raw_body: str) -> int:
        """POST an arbitrary (possibly malformed) body as the beacon transport does."""
        resp = self.send("POST", ep.TRACK, content=raw_body)
        return resp.status_code
