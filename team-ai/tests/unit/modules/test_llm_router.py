from langchain_core.language_models.fake_chat_models import FakeListChatModel

from app.core.config import Settings
from app.core.resilience import CircuitBreakerPolicy, FailureKind
from app.modules.ai.llm.router import ModelRouter
from tests.factories import build_test_settings


def _settings(**overrides: object) -> Settings:
    return build_test_settings(**overrides)


def test_model_router_uses_fake_model_when_default_model_is_empty():
    router = ModelRouter(_settings())

    assert isinstance(router.chat_model("default"), FakeListChatModel)


def test_model_router_uses_judge_model_when_configured():
    created: list[str] = []

    def build(target: str):
        created.append(target)
        return FakeListChatModel(responses=[target])

    router = ModelRouter(
        _settings(
            CHAT_MODEL="openai:gpt-4.1-mini",
            JUDGE_CHAT_MODEL="openai:gpt-4.1",
        ),
        model_builder=build,
    )

    model = router.chat_model("judge")

    assert isinstance(model, FakeListChatModel)
    assert created == ["openai:gpt-4.1"]


def test_model_router_lists_primary_then_all_fallback_targets_in_order():
    router = ModelRouter(
        _settings(
            CHAT_MODEL="openai:gpt-4.1-mini",
            CHAT_FALLBACK_MODELS="anthropic:claude-sonnet-4-5, openai:gpt-4.1",
        )
    )

    assert router.fallback_models("default") == [
        "openai:gpt-4.1-mini",
        "anthropic:claude-sonnet-4-5",
        "openai:gpt-4.1",
    ]
    assert router.fallback_models("judge") == ["openai:gpt-4.1-mini"]


def test_model_router_switches_to_secondary_after_primary_counted_failures():
    router = ModelRouter(
        _settings(
            CHAT_MODEL="openai:gpt-4.1-mini",
            CHAT_FALLBACK_MODELS="anthropic:claude-sonnet-4-5, openai:gpt-4.1",
        )
    )

    assert router.current_target("default") == "openai:gpt-4.1-mini"
    router.record_error("openai:gpt-4.1-mini", status_code=429)
    router.record_error("openai:gpt-4.1-mini", status_code=503)
    assert router.current_target("default") == "openai:gpt-4.1-mini"

    router.record_error("openai:gpt-4.1-mini", error=TimeoutError())

    assert router.current_target("default") == "anthropic:claude-sonnet-4-5"


def test_model_router_does_not_count_request_caused_4xx():
    router = ModelRouter(
        _settings(CHAT_MODEL="primary", CHAT_FALLBACK_MODELS="secondary"),
        breaker_policy=CircuitBreakerPolicy(failure_threshold=1),
    )

    for status in (400, 401, 404, 422):
        router.record_error("primary", status_code=status)

    assert router.eligible_targets("default") == ["primary", "secondary"]


def test_model_router_ignores_targets_outside_the_chain():
    router = ModelRouter(
        _settings(CHAT_MODEL="primary", CHAT_FALLBACK_MODELS="secondary"),
        breaker_policy=CircuitBreakerPolicy(failure_threshold=1),
    )

    router.record_error("stranger", status_code=500)

    assert router.eligible_targets("default") == ["primary", "secondary"]


def test_model_router_resets_failure_count_on_success():
    router = ModelRouter(
        _settings(CHAT_MODEL="primary", CHAT_FALLBACK_MODELS="secondary"),
        breaker_policy=CircuitBreakerPolicy(failure_threshold=2),
    )

    router.record_error("primary", status_code=429)
    router.record_success("primary")
    router.record_error("primary", status_code=429)

    assert router.current_target("default") == "primary"


class _Clock:
    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now


def _three_target_router(clock: _Clock) -> ModelRouter:
    return ModelRouter(
        _settings(CHAT_MODEL="a", CHAT_FALLBACK_MODELS="b, c"),
        breaker_policy=CircuitBreakerPolicy(failure_threshold=2, cooldown_seconds=30),
        clock=clock,
    )


def test_model_router_has_one_breaker_per_target_with_three_targets():
    router = _three_target_router(_Clock())

    router.record_error("a", status_code=500)
    router.record_error("a", status_code=500)
    assert router.eligible_targets("default") == ["b", "c"]

    router.record_error("b", error=ConnectionError())
    router.record_error("b", error=ConnectionError())
    assert router.eligible_targets("default") == ["c"]
    assert router.current_target("default") == "c"

    router.record_error("c", status_code=429)
    router.record_error("c", status_code=429)
    # Everything open: current_target degrades to the primary rather than "".
    assert router.eligible_targets("default") == []
    assert router.current_target("default") == "a"


def test_model_router_half_open_admits_one_probe_then_recovers():
    clock = _Clock()
    router = _three_target_router(clock)
    router.record_error("a", status_code=500)
    router.record_error("a", status_code=500)
    assert router.breaker_state("a") == "open"

    clock.now = 31
    # Peeking does not consume the probe.
    assert router.eligible_targets("default") == ["a", "b", "c"]
    assert router.eligible_targets("default") == ["a", "b", "c"]
    assert router.try_acquire("a") is True
    assert router.try_acquire("a") is False  # second caller is not a probe
    assert router.eligible_targets("default") == ["b", "c"]

    router.record_success("a")

    assert router.breaker_state("a") == "closed"
    assert router.eligible_targets("default") == ["a", "b", "c"]


def test_model_router_failed_probe_reopens_for_fresh_cooldown():
    clock = _Clock()
    router = _three_target_router(clock)
    router.record_error("a", status_code=500)
    router.record_error("a", status_code=500)
    clock.now = 31
    assert router.try_acquire("a") is True

    kind = router.record_error("a", error=TimeoutError())

    assert kind is FailureKind.TIMEOUT
    assert router.breaker_state("a") == "open"
    clock.now = 60  # < 31 + 30
    assert router.eligible_targets("default") == ["b", "c"]
    clock.now = 62
    assert router.eligible_targets("default") == ["a", "b", "c"]


def test_model_router_cancelled_attempt_frees_the_probe_without_judging():
    clock = _Clock()
    router = _three_target_router(clock)
    router.record_error("a", status_code=500)
    router.record_error("a", status_code=500)
    clock.now = 31
    assert router.try_acquire("a") is True

    router.record_error("a", kind=FailureKind.CANCELLED)

    assert router.try_acquire("a") is True
