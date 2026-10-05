"""Chat service client for platform.chat.v1.ChatService (buyer <-> seller threads)."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService
from src.constants import gateway_endpoints as ep


class ChatService(BaseService):
    def get_or_create_thread(self, seller_id: str, listing_id: str = "") -> dict[str, Any]:
        """The buyer's thread with a seller (the ``thread`` object)."""
        body = {"sellerId": seller_id, "listingId": listing_id}
        return self.post(ep.CHAT_GET_OR_CREATE_THREAD, body).get("thread", {})

    def list_threads(self) -> list[dict[str, Any]]:
        """The caller's threads (as buyer or seller)."""
        return self.post(ep.CHAT_LIST_THREADS, {}).get("threads", [])

    def get_thread_messages(self, thread_id: str) -> list[dict[str, Any]]:
        return self.post(ep.CHAT_GET_THREAD_MESSAGES, {"threadId": thread_id}).get("messages", [])

    def send_message(self, thread_id: str, content: str) -> dict[str, Any]:
        """The sent ``message`` object."""
        return self.post(ep.CHAT_SEND_MESSAGE, {"threadId": thread_id, "content": content}).get(
            "message", {}
        )
