import asyncio

import pytest

from app.core.resilience import (
    BreakerState,
    CircuitBreaker,
    CircuitBreakerPolicy,
    FailureKind,
    RetryPolicy,
    TimeoutPolicy,
    classify_failure,
    classify_status,
)


def test_retry_policy_retries_transient_status_before_max_attempts() -> None:
    policy = RetryPolicy(max_attempts=3, retry_status_codes=(429, 500))

    decision = policy.decision(attempt=1, status_code=429)

    assert decision.should_retry is True
    assert decision.next_delay_seconds == 0.0


def test_retry_policy_stops_at_max_attempts() -> None:
    policy = RetryPolicy(max_attempts=3, retry_status_codes=(429,))

    decision = policy.decision(attempt=3, status_code=429)

    assert decision.should_retry is False
    assert decision.next_delay_seconds is None


def test_retry_policy_ignores_non_retryable_statuses() -> None:
    policy = RetryPolicy(max_attempts=3, retry_status_codes=(429,))

    decision = policy.decision(attempt=1, status_code=400)

    assert decision.should_retry is False


def test_retry_policy_retries_exceptions_when_enabled() -> None:
    policy = RetryPolicy(max_attempts=2, retry_exceptions=True)

    decision = policy.decision(attempt=1, error=RuntimeError("timeout"))

    assert decision.should_retry is True


def test_circuit_breaker_opens_after_threshold_failures() -> None:
    breaker = CircuitBreaker(failure_threshold=2, failure_status_range=range(400, 500))

    breaker.record_failure(status_code=429)
    assert breaker.allows_request() is True

    breaker.record_failure(status_code=400)

    assert breaker.allows_request() is False
    assert breaker.is_open is True


def test_circuit_breaker_ignores_statuses_outside_failure_range() -> None:
    breaker = CircuitBreaker(failure_threshold=1, failure_status_range=range(400, 500))

    breaker.record_failure(status_code=500)

    assert breaker.allows_request() is True


def test_circuit_breaker_success_resets_failures_and_closes() -> None:
    breaker = CircuitBreaker(failure_threshold=1, failure_status_range=range(400, 500))
    breaker.record_failure(status_code=429)

    breaker.record_success()

    assert breaker.failure_count == 0
    assert breaker.allows_request() is True


def test_circuit_breaker_policy_builds_stateful_breaker() -> None:
    policy = CircuitBreakerPolicy(
        failure_threshold=2,
        failure_status_range=range(400, 500),
    )

    breaker = policy.build()

    breaker.record_failure(status_code=429)
    breaker.record_failure(status_code=400)
    assert breaker.is_open is True


def test_timeout_policy_rejects_non_positive_timeout() -> None:
    try:
        TimeoutPolicy(timeout_seconds=0)
    except ValueError as exc:
        assert "positive" in str(exc)
    else:
        raise AssertionError("TimeoutPolicy accepted a non-positive timeout")


# --- classify_failure -------------------------------------------------------


class _StatusExc(Exception):
    def __init__(self, status_code: int) -> None:
        super().__init__(f"status {status_code}")
        self.status_code = status_code


class _Response:
    def __init__(self, status_code: int) -> None:
        self.status_code = status_code


class _HttpxStatusError(Exception):
    def __init__(self, status_code: int) -> None:
        super().__init__("http error")
        self.response = _Response(status_code)


class ReadTimeout(Exception):  # mimics httpx.ReadTimeout by class name
    pass


class ConnectError(Exception):  # mimics httpx.ConnectError by class name
    pass


class APITimeoutError(Exception):  # mimics openai.APITimeoutError
    pass


class APIConnectionError(Exception):  # mimics openai.APIConnectionError
    pass


@pytest.mark.parametrize(
    ("exc", "kind", "counts"),
    [
        (_StatusExc(429), FailureKind.RATE_LIMITED, True),
        (_StatusExc(500), FailureKind.SERVER, True),
        (_StatusExc(503), FailureKind.SERVER, True),
        (_HttpxStatusError(429), FailureKind.RATE_LIMITED, True),
        (_HttpxStatusError(502), FailureKind.SERVER, True),
        (_StatusExc(400), FailureKind.CLIENT, False),
        (_StatusExc(404), FailureKind.CLIENT, False),
        (_HttpxStatusError(401), FailureKind.CLIENT, False),
        (asyncio.TimeoutError(), FailureKind.TIMEOUT, True),  # noqa: UP041
        (TimeoutError(), FailureKind.TIMEOUT, True),
        (ReadTimeout(), FailureKind.TIMEOUT, True),
        (APITimeoutError(), FailureKind.TIMEOUT, True),
        (ConnectError(), FailureKind.CONNECTION, True),
        (APIConnectionError(), FailureKind.CONNECTION, True),
        (ConnectionResetError(), FailureKind.CONNECTION, True),
        (RuntimeError("boom"), FailureKind.UNKNOWN, True),
        (asyncio.CancelledError(), FailureKind.CANCELLED, False),
    ],
)
def test_classify_failure_table(exc, kind, counts) -> None:
    assert classify_failure(exc) is kind
    assert kind.counts_as_failure is counts


def test_classify_failure_handles_real_httpx_exceptions() -> None:
    httpx = pytest.importorskip("httpx")
    request = httpx.Request("POST", "https://llm.example/v1")
    assert classify_failure(httpx.ReadTimeout("t", request=request)) is (
        FailureKind.TIMEOUT
    )
    assert classify_failure(httpx.ConnectError("c", request=request)) is (
        FailureKind.CONNECTION
    )
    response = httpx.Response(503, request=request)
    error = httpx.HTTPStatusError("s", request=request, response=response)
    assert classify_failure(error) is FailureKind.SERVER


def test_classify_status() -> None:
    assert classify_status(429) is FailureKind.RATE_LIMITED
    assert classify_status(500) is FailureKind.SERVER
    assert classify_status(404) is FailureKind.CLIENT
    assert classify_status(None) is FailureKind.UNKNOWN


# --- breaker cooldown / half-open --------------------------------------------


class _Clock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


def _breaker(clock: _Clock, *, threshold: int = 2, cooldown: float = 30.0):
    return CircuitBreaker(
        failure_threshold=threshold, cooldown_seconds=cooldown, clock=clock
    )


def test_breaker_opens_at_threshold_for_counted_failures() -> None:
    breaker = _breaker(_Clock())

    breaker.record_failure(counts=True)
    assert breaker.allows_request() is True
    breaker.record_failure(counts=True)

    assert breaker.state is BreakerState.OPEN
    assert breaker.allows_request() is False


def test_breaker_stays_closed_on_request_caused_4xx() -> None:
    breaker = _breaker(_Clock(), threshold=1)

    breaker.record_failure(counts=classify_status(400).counts_as_failure)
    breaker.record_failure(counts=classify_status(404).counts_as_failure)

    assert breaker.failure_count == 0
    assert breaker.state is BreakerState.CLOSED


def test_breaker_admits_exactly_one_probe_after_cooldown() -> None:
    clock = _Clock()
    breaker = _breaker(clock, threshold=1)
    breaker.record_failure(counts=True)

    clock.now += 29
    assert breaker.allows_request() is False
    clock.now += 2
    assert breaker.would_allow() is True  # peek does not claim the probe
    assert breaker.allows_request() is True  # the probe
    assert breaker.state is BreakerState.HALF_OPEN
    assert breaker.allows_request() is False  # concurrent callers still skipped


def test_breaker_probe_success_closes() -> None:
    clock = _Clock()
    breaker = _breaker(clock, threshold=1)
    breaker.record_failure(counts=True)
    clock.now += 31
    assert breaker.allows_request() is True

    breaker.record_success()

    assert breaker.state is BreakerState.CLOSED
    assert breaker.allows_request() is True
    assert breaker.allows_request() is True


def test_breaker_probe_failure_reopens_with_fresh_cooldown() -> None:
    clock = _Clock()
    breaker = _breaker(clock, threshold=1)
    breaker.record_failure(counts=True)
    clock.now += 31
    assert breaker.allows_request() is True

    breaker.record_failure(counts=True)

    assert breaker.state is BreakerState.OPEN
    assert breaker.allows_request() is False
    clock.now += 29
    assert breaker.allows_request() is False  # fresh cooldown, not the old one
    clock.now += 2
    assert breaker.allows_request() is True


def test_breaker_released_or_lost_probe_can_be_retried() -> None:
    clock = _Clock()
    breaker = _breaker(clock, threshold=1)
    breaker.record_failure(counts=True)
    clock.now += 31
    assert breaker.allows_request() is True

    breaker.release_probe()  # e.g. the probing request was cancelled

    assert breaker.allows_request() is True


def test_breaker_without_cooldown_never_recovers() -> None:
    clock = _Clock()
    breaker = CircuitBreaker(failure_threshold=1, clock=clock)
    breaker.record_failure(counts=True)

    clock.now += 10_000

    assert breaker.allows_request() is False


def test_policy_rejects_negative_cooldown() -> None:
    with pytest.raises(ValueError):
        CircuitBreakerPolicy(cooldown_seconds=-1)
