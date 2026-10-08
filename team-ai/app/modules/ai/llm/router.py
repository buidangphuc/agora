from __future__ import annotations

from collections.abc import Callable
from typing import TYPE_CHECKING, Literal

from app.core.config import Settings
from app.core.resilience import (
    CircuitBreaker,
    CircuitBreakerPolicy,
    Clock,
    FailureKind,
    classify_failure,
    classify_status,
)
from app.modules.ai._deps import require_langchain

if TYPE_CHECKING:
    from langchain_core.language_models.chat_models import BaseChatModel

ModelRole = Literal["default", "judge"]
ModelBuilder = Callable[[str], "BaseChatModel"]


def _default_model_builder(target: str) -> BaseChatModel:
    """LangChain's provider-routing builder — imported lazily so the module
    stays importable without the ``[ai]`` extra."""
    require_langchain()
    from langchain.chat_models import init_chat_model

    # OpenAI targets: an OpenAI-compatible endpoint (OPENAI_BASE_URL) makes
    # langchain-openai skip ``stream_options.include_usage`` by default, and usage is
    # metered, so ask for it. ``max_retries=0``: the SDK would otherwise retry
    # 429/5xx itself; retries belong only to the streamer's bounded attempt loop.
    kwargs: dict[str, object] = (
        {"stream_usage": True, "max_retries": 0} if target.startswith("openai:") else {}
    )
    return init_chat_model(target, **kwargs)


class ModelRouter:
    """Resolves LLM targets for a role and tracks per-target health.

    The chain for the default role is ``CHAT_MODEL`` followed by every entry of
    ``CHAT_FALLBACK_MODELS`` in order; each target has its own circuit breaker
    (``(role, target)``). Callers ask ``eligible_targets`` for the targets worth
    trying, claim each with ``try_acquire`` right before calling it (this is what
    admits the single half-open probe), and report the outcome with
    ``record_success`` / ``record_error``.
    """

    def __init__(
        self,
        settings: Settings,
        *,
        model_builder: ModelBuilder | None = None,
        breaker_policy: CircuitBreakerPolicy | None = None,
        clock: Clock | None = None,
    ) -> None:
        self.settings = settings
        self.model_builder = model_builder or _default_model_builder
        self.breaker_policy = breaker_policy or CircuitBreakerPolicy(
            failure_threshold=settings.LLM_BREAKER_THRESHOLD,
            cooldown_seconds=settings.LLM_BREAKER_COOLDOWN_SECONDS,
        )
        self._clock = clock
        self._breakers: dict[tuple[ModelRole, str], CircuitBreaker] = {}

    def chat_model(self, role: ModelRole = "default") -> BaseChatModel:
        target = self.current_target(role)
        if not target:
            require_langchain()
            from langchain_core.language_models.fake_chat_models import (
                FakeListChatModel,
            )

            return FakeListChatModel(responses=["fake response"])
        return self.model_builder(target)

    def build_model(self, target: str) -> BaseChatModel:
        """Build the chat model for one explicit target (callers memoise).

        An empty target (no ``CHAT_MODEL`` configured) yields the offline fake.
        """
        if not target:
            return self.chat_model()
        return self.model_builder(target)

    def primary_target(self, role: ModelRole = "default") -> str:
        if role == "judge" and self.settings.JUDGE_CHAT_MODEL:
            return self.settings.JUDGE_CHAT_MODEL
        return self.settings.CHAT_MODEL

    def secondary_target(self, role: ModelRole = "default") -> str | None:
        """First fallback entry (kept for compatibility; see ``fallback_models``)."""
        chain = self.fallback_models(role)
        primary = self.primary_target(role)
        rest = chain[1:] if chain and chain[0] == primary else chain
        return rest[0] if rest else None

    def fallback_models(self, role: ModelRole = "default") -> list[str]:
        """Ordered chain: the primary, then every fallback (default role only)."""
        primary = self.primary_target(role)
        targets = [primary] if primary else []
        if role == "default":
            for target in _split_csv(self.settings.CHAT_FALLBACK_MODELS):
                if target not in targets:
                    targets.append(target)
        return targets

    def eligible_targets(self, role: ModelRole = "default") -> list[str]:
        """Chain targets whose breaker would currently admit a call (read-only)."""
        return [
            target
            for target in self.fallback_models(role)
            if self._breaker(role, target).would_allow()
        ]

    def try_acquire(self, target: str, *, role: ModelRole = "default") -> bool:
        """Claim a call slot on ``target``; in half-open this is the one probe."""
        return self._breaker(role, target).allows_request()

    def current_target(self, role: ModelRole = "default") -> str:
        """First eligible target, else the primary (kept for compatibility)."""
        eligible = self.eligible_targets(role)
        return eligible[0] if eligible else self.primary_target(role)

    def record_success(
        self,
        target: str,
        *,
        role: ModelRole = "default",
    ) -> None:
        if target in self.fallback_models(role):
            self._breaker(role, target).record_success()

    def record_error(
        self,
        target: str,
        *,
        status_code: int | None = None,
        error: BaseException | None = None,
        kind: FailureKind | None = None,
        role: ModelRole = "default",
    ) -> FailureKind:
        """Record a failed attempt and return how it was classified.

        ``kind`` wins over ``error``, which wins over ``status_code``. Only
        429/5xx/timeout/connection/unknown failures count against the breaker.
        """
        if kind is None:
            if error is not None:
                kind = classify_failure(error)
            else:
                kind = classify_status(status_code)
        if target in self.fallback_models(role):
            breaker = self._breaker(role, target)
            if kind is FailureKind.CANCELLED:
                breaker.release_probe()
            else:
                breaker.record_failure(counts=kind.counts_as_failure)
        return kind

    def breaker_state(self, target: str, *, role: ModelRole = "default") -> str:
        return self._breaker(role, target).state.value

    def _breaker(self, role: ModelRole, target: str) -> CircuitBreaker:
        key = (role, target)
        if key not in self._breakers:
            self._breakers[key] = self.breaker_policy.build(clock=self._clock)
        return self._breakers[key]


def _split_csv(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]
