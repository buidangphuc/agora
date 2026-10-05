"""Notification service client for platform.notification.v1.NotificationService."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService
from src.constants import gateway_endpoints as ep


class NotificationService(BaseService):
    def list_notifications(self, page_size: int = 50) -> list[dict[str, Any]]:
        return self.post(ep.NOTIFICATION_LIST, {"pageSize": page_size}).get("notifications", [])
