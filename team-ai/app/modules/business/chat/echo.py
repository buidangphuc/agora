from collections.abc import AsyncIterator


class EchoChatService:
    """Offline backend: echoes the message word by word."""

    async def stream(
        self,
        message: str,
        *,
        session_id: str,
        principal_id: str,
        request_id: str,
        deadline_seconds: float | None,
    ) -> AsyncIterator[str]:
        for word in f"echo: {message}".split():
            yield word + " "
