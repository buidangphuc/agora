import asyncio
import hashlib
import json
from typing import Any

import httpx

from app.modules.business.recommendation.ports import Assignment

_NONE = Assignment(None, None, "none")
_UNASSIGNED = Assignment(None, None, "unassigned")


class FliptExperimentAssigner:
    def __init__(
        self, client: httpx.AsyncClient, *, salt: str, timeout_s: float
    ) -> None:
        self._client = client
        self._salt = salt
        self._timeout_s = timeout_s

    async def assign(self, flag: str | None, entity_id: str | None) -> Assignment:
        if not flag or not entity_id:
            return _NONE
        hashed = hashlib.sha256(f"{self._salt}:{entity_id}".encode()).hexdigest()
        try:
            response = await asyncio.wait_for(
                self._client.post(
                    "/evaluate/v1/variant",
                    json={
                        "namespaceKey": "default",
                        "flagKey": flag,
                        "entityId": hashed,
                        "context": {},
                    },
                ),
                self._timeout_s,
            )
        except (TimeoutError, httpx.HTTPError):
            return _UNASSIGNED
        if response.status_code == 404:
            return _NONE
        if response.status_code != 200:
            return _UNASSIGNED
        try:
            body = response.json()
            if not isinstance(body, dict):
                return _UNASSIGNED
            if not body.get("match"):
                return _NONE
            variant = body.get("variantKey")
            if not isinstance(variant, str) or not variant:
                return _UNASSIGNED
            return Assignment(variant, _ranking_model(body), "assigned")
        except ValueError:
            return _UNASSIGNED


def _ranking_model(body: dict[str, Any]) -> str | None:
    raw = body.get("variantAttachment")
    if not raw:
        return None
    try:
        attachment = json.loads(raw)
    except (TypeError, ValueError):
        return None
    ranking = attachment.get("ranking") if isinstance(attachment, dict) else None
    model = ranking.get("model") if isinstance(ranking, dict) else None
    return model if isinstance(model, str) else None
