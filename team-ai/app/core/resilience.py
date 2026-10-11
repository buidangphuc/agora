from __future__ import annotations

import asyncio
import time
from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum
from typing import Any

Clock = Callable[[], float]


class FailureKind(StrEnum):
    """Coarse classification of a failed upstream call (see ``classify_failure``)."""

    RATE_LIMITED = "rate_limited"
    SERVER = "server"
    TIMEOUT = "timeout"
    CONNECTION = "connection"
    CLIENT = "client"
    UNKNOWN = "unknown"
    CANCELLED = "cancelled"

    @property
    def counts_as_failure(self) -> bool:
        """Whether a breaker should count this kind against the target.

        Request-caused 4xx and caller cancellation say nothing about provider
        health; ``unknown`` exceptions are counted (safe failure).
        """
        return self not in (FailureKind.CLIENT, FailureKind.CANCELLED)


def _status_code_of(exc: BaseException) -> int | None:
    """Pull an HTTP status off the shapes provider SDKs use.

    ``exc.status_code`` (openai/anthropic), ``exc.response.status_code`` (httpx),
    ``exc.http_status`` / ``exc.code`` as int.
    """
    candidates: list[Any] = [
        getattr(exc, "status_code", None),
        getattr(getattr(exc, "response", None), "status_code", None),
        getattr(exc, "http_status", None),
        getattr(exc, "code", None),
    ]
    for value in candidates:
        if (
            isinstance(value, int)
            and not isinstance(value, bool)
            and 100 <= value <= 599
        ):
            return value
    return None


def _class_names(exc: BaseException) -> tuple[str, ...]:
    return tuple(cls.__name__ for cls in type(exc).__mro__)


def classify_failure(exc: BaseException) -> FailureKind:
    """Map an exception from an upstream (LLM) call to a ``FailureKind``.

    Pure and vendor-neutral: SDK exception types are recognised by status
    attributes or class names so this module needs no provider import.
    """
    if isinstance(exc, asyncio.CancelledError):
        return FailureKind.CANCELLED

    status = _status_code_of(exc)
    if status is not None:
        if status == 429:
            return FailureKind.RATE_LIMITED
        if status == 408:
            return FailureKind.TIMEOUT
        if 500 <= status <= 599:
            return FailureKind.SERVER
        if 400 <= status <= 499:
            return FailureKind.CLIENT

    if isinstance(exc, TimeoutError):  # asyncio.TimeoutError is an alias (3.11+)
        return FailureKind.TIMEOUT
    names = _class_names(exc)
    if any("Timeout" in name for name in names):
        return FailureKind.TIMEOUT
    if isinstance(exc, ConnectionError) or any(
        name
        in ("ConnectError", "APIConnectionError", "NetworkError", "ConnectionError")
        for name in names
    ):
        return FailureKind.CONNECTION
    return FailureKind.UNKNOWN


def classify_status(status_code: int | None) -> FailureKind:
    """Classify a bare HTTP status (``None`` -> ``UNKNOWN``)."""
    if status_code is None:
        return FailureKind.UNKNOWN
    if status_code == 429:
        return FailureKind.RATE_LIMITED
    if status_code == 408:
        return FailureKind.TIMEOUT
    if 500 <= status_code <= 599:
        return FailureKind.SERVER
    if 400 <= status_code <= 499:
        return FailureKind.CLIENT
    return FailureKind.UNKNOWN


@dataclass(frozen=True)
class RetryDecision:
    should_retry: bool
    next_delay_seconds: float | None = None


@dataclass(frozen=True)
class RetryPolicy:
    max_attempts: int = 3
    retry_status_codes: tuple[int, ...] = (408, 429, 500, 502, 503, 504)
    retry_exceptions: bool = True
    backoff_seconds: tuple[float, ...] = (0.0, 0.25, 1.0)

    def __post_init__(self) -> None:
        if self.max_attempts <= 0:
            raise ValueError("max_attempts must be positive")
        if not self.backoff_seconds:
            raise ValueError("backoff_seconds must not be empty")
        if any(delay < 0 for delay in self.backoff_seconds):
            raise ValueError("backoff_seconds must not contain negative values")

    def decision(
        self,
        *,
        attempt: int,
        status_code: int | None = None,
        error: BaseException | None = None,
    ) -> RetryDecision:
        if attempt >= self.max_attempts:
            return RetryDecision(should_retry=False)
        if error is not None and self.retry_exceptions:
            return RetryDecision(True, self._delay_for_attempt(attempt))
        if status_code in self.retry_status_codes:
            return RetryDecision(True, self._delay_for_attempt(attempt))
        return RetryDecision(should_retry=False)

    def _delay_for_attempt(self, attempt: int) -> float:
        index = max(min(attempt - 1, len(self.backoff_seconds) - 1), 0)
        return self.backoff_seconds[index]


@dataclass(frozen=True)
class TimeoutPolicy:
    timeout_seconds: float = 10.0

    def __post_init__(self) -> None:
        if self.timeout_seconds <= 0:
            raise ValueError("timeout_seconds must be positive")


@dataclass(frozen=True)
class CircuitBreakerPolicy:
    failure_threshold: int = 3
    failure_status_codes: tuple[int, ...] = ()
    failure_status_range: range | None = None
    # Seconds an open breaker waits before admitting one half-open probe.
    # ``None`` keeps the legacy behaviour: once open, it never recovers.
    cooldown_seconds: float | None = None

    def __post_init__(self) -> None:
        if self.failure_threshold <= 0:
            raise ValueError("failure_threshold must be positive")
        if self.cooldown_seconds is not None and self.cooldown_seconds < 0:
            raise ValueError("cooldown_seconds must not be negative")

    def build(self, *, clock: Clock | None = None) -> CircuitBreaker:
        return CircuitBreaker(
            failure_threshold=self.failure_threshold,
            failure_status_codes=self.failure_status_codes,
            failure_status_range=self.failure_status_range,
            cooldown_seconds=self.cooldown_seconds,
            clock=clock,
        )


class BreakerState(StrEnum):
    CLOSED = "closed"
    OPEN = "open"
    HALF_OPEN = "half_open"


class CircuitBreaker:
    """Closed -> open (at threshold) -> half-open (after cooldown) -> closed/open.

    In half-open exactly one caller is admitted as the probe; its success closes
    the breaker, its failure re-opens it for a fresh cooldown. ``would_allow`` is a
    read-only peek; ``allows_request`` is the admitting call (it claims the probe).
    Single-threaded asyncio use: no locking needed.
    """

    def __init__(
        self,
        *,
        failure_threshold: int = 3,
        failure_status_codes: tuple[int, ...] = (),
        failure_status_range: range | None = None,
        cooldown_seconds: float | None = None,
        clock: Clock | None = None,
    ) -> None:
        if failure_threshold <= 0:
            raise ValueError("failure_threshold must be positive")
        self.failure_threshold = failure_threshold
        self.failure_status_codes = failure_status_codes
        self.failure_status_range = failure_status_range
        self.cooldown_seconds = cooldown_seconds
        self._clock: Clock = clock or time.monotonic
        self.failure_count = 0
        self.state = BreakerState.CLOSED
        self._opened_at = 0.0
        self._probe_started_at: float | None = None

    @property
    def is_open(self) -> bool:
        return self.state is not BreakerState.CLOSED

    def would_allow(self) -> bool:
        """Read-only: would ``allows_request`` admit a caller right now?"""
        if self.state is BreakerState.CLOSED:
            return True
        if self.cooldown_seconds is None:
            return False
        now = self._clock()
        if self.state is BreakerState.OPEN:
            return now - self._opened_at >= self.cooldown_seconds
        # Half-open: a probe is in flight; re-admit only if it looks lost.
        started = self._probe_started_at
        return started is None or now - started >= self.cooldown_seconds

    def allows_request(self) -> bool:
        """Admit a caller; in open/half-open this claims the single probe slot."""
        if not self.would_allow():
            return False
        if self.state is not BreakerState.CLOSED:
            self.state = BreakerState.HALF_OPEN
            self._probe_started_at = self._clock()
        return True

    def record_success(self) -> None:
        self.failure_count = 0
        self.state = BreakerState.CLOSED
        self._probe_started_at = None

    def record_failure(
        self,
        *,
        status_code: int | None = None,
        counts: bool | None = None,
    ) -> None:
        """Record a failed call.

        ``counts`` lets the caller decide (e.g. via ``classify_failure``); when
        omitted the legacy status-code configuration decides.
        """
        if counts is None:
            counts = self._counts_as_failure(status_code)
        if self.state is BreakerState.HALF_OPEN:
            if counts:
                self._open()
            else:
                # Probe failed for a reason that says nothing about health:
                # free the probe slot and stay half-open.
                self._probe_started_at = None
            return
        if not counts:
            return
        self.failure_count += 1
        if self.failure_count >= self.failure_threshold:
            self._open()

    def release_probe(self) -> None:
        """Give back a claimed probe slot without judging the target (cancel)."""
        if self.state is BreakerState.HALF_OPEN:
            self._probe_started_at = None

    def reset(self) -> None:
        self.record_success()

    def _open(self) -> None:
        self.state = BreakerState.OPEN
        self._opened_at = self._clock()
        self._probe_started_at = None

    def _counts_as_failure(self, status_code: int | None) -> bool:
        if status_code is None:
            return False
        if status_code in self.failure_status_codes:
            return True
        return (
            self.failure_status_range is not None
            and status_code in self.failure_status_range
        )
