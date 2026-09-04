"""Sharing service: create a short share link and resolve it back to its target.

Gateway Connect/JSON RPCs for platform.sharing.v1.SharingService.
"""

from __future__ import annotations

from .base_service import BaseService

_SVC = "/platform.sharing.v1.SharingService"


class SharingService(BaseService):
    def create_share_link(
        self, target_type: str, target_id: str, utm: dict[str, str] | None = None
    ) -> dict:
        payload: dict = {"targetType": target_type, "targetId": target_id}
        if utm:
            payload["utm"] = utm
        return self.post(f"{_SVC}/CreateShareLink", payload)

    def resolve_share_link(self, short_code: str) -> dict:
        return self.post(f"{_SVC}/ResolveShareLink", {"shortCode": short_code})
