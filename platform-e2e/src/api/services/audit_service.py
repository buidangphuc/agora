"""Audit service: write an audit event and query the audit log back.

Gateway Connect/JSON RPCs for platform.audit.v1.AuditService.
"""

from __future__ import annotations

from .base_service import BaseService

_SVC = "/platform.audit.v1.AuditService"


class AuditService(BaseService):
    def write_audit_event(
        self, actor_id: str, action: str, target_type: str, target_id: str
    ) -> dict:
        return self.post(
            f"{_SVC}/WriteAuditEvent",
            {
                "actorId": actor_id,
                "action": action,
                "targetType": target_type,
                "targetId": target_id,
            },
        )

    def query_audit_log(
        self, actor_id: str = "", target_type: str = "", page_size: int = 50
    ) -> dict:
        payload: dict = {"page": {"pageSize": page_size}}
        if actor_id:
            payload["actorId"] = actor_id
        if target_type:
            payload["targetType"] = target_type
        return self.post(f"{_SVC}/QueryAuditLog", payload)
