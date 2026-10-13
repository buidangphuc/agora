import time
from collections.abc import Callable

from loguru import logger


class ThrottledWarning:
    """Logs a warning at most once per ``every_s``, for failures that repeat per request."""

    def __init__(
        self,
        event: str,
        *,
        clock: Callable[[], float] = time.monotonic,
        every_s: float = 60.0,
    ) -> None:
        self._event = event
        self._clock = clock
        self._every_s = every_s
        self._last: float | None = None

    def log(self, exc: Exception) -> None:
        now = self._clock()
        if self._last is None or now - self._last >= self._every_s:
            self._last = now
            logger.warning("{} err={}", self._event, exc)
