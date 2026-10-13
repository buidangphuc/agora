class ChatError(Exception):
    """Base class for failures a chat service raises to its transport."""


class ChatQuotaExhausted(ChatError):
    """The principal's reply quota is used up; no model call was made."""

    def __init__(self, retry_after_seconds: int | None = None) -> None:
        super().__init__("chat quota exhausted")
        self.retry_after_seconds = retry_after_seconds


class ChatModelUnavailable(ChatError):
    """Every model failed before producing text."""


class ChatModelRateLimited(ChatError):
    """The provider rate-limited the request before any text."""


class ChatInterrupted(ChatError):
    """The model failed after part of the reply was delivered."""


class ChatDeadlineExceeded(ChatError):
    """The caller's deadline (or the chain deadline) ran out."""
