from collections.abc import AsyncIterator
from typing import Protocol


class ChatService(Protocol):
    def stream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str,
        request_id: str,
        deadline_seconds: float | None,
    ) -> AsyncIterator[str]:
        """Yield text deltas of one reply; raises ``ChatError`` subclasses.

        ``principal_id`` is "" for an anonymous caller (stateless, unmetered).
        ``deadline_seconds`` (None = none) is the caller's remaining time.
        """
        ...
