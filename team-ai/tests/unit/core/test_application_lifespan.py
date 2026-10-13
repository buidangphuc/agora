import sys
import types
from contextlib import asynccontextmanager

import fakeredis.aioredis as fake_aioredis
import pytest
from fastapi import FastAPI

from app.bootstrap import (
    application as application_module,
    resources as resources_module,
)
from app.bootstrap.application import _build_lifespan, create_app
from app.bootstrap.resources import ApplicationResources
from app.bootstrap.services import (
    default_completion_guards_factory,
    default_completion_handler_factory,
)
from app.core.config import Settings
from app.core.health import HealthService
from app.core.middleware import InFlightTracker


class _FakeLangfuseClient:
    def __init__(self) -> None:
        self.flushed = False

    def flush(self) -> None:
        self.flushed = True


def _install_fake_langfuse(monkeypatch: pytest.MonkeyPatch) -> _FakeLangfuseClient:
    client = _FakeLangfuseClient()
    fake_module = types.ModuleType("langfuse")
    fake_module.get_client = lambda: client  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "langfuse", fake_module)
    return client


def _bare_app() -> FastAPI:
    app = FastAPI()
    app.state.resources = ApplicationResources()
    app.state.in_flight_tracker = InFlightTracker()
    app.state.health_service = HealthService(check_external_dependencies=False)
    return app


def _patch_resource_lifecycle(monkeypatch: pytest.MonkeyPatch) -> None:
    @asynccontextmanager
    async def _noop_runtime(settings, **_kwargs):
        yield ApplicationResources()

    @asynccontextmanager
    async def _no_grpc(resources, settings, *, services_factory):
        yield 0

    monkeypatch.setattr(application_module, "open_runtime", _noop_runtime)
    monkeypatch.setattr(application_module, "serve_grpc", _no_grpc)


async def test_lifespan_flushes_langfuse_client_when_enabled(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    fake_client = _install_fake_langfuse(monkeypatch)
    _patch_resource_lifecycle(monkeypatch)
    keys = {
        "LANGFUSE_PUBLIC_KEY": "pk",
        "LANGFUSE_SECRET_KEY": "sk",  # pragma: allowlist secret
    }
    settings = test_settings.model_copy(update=keys)

    lifespan = _build_lifespan(
        settings,
        init_resources=True,
        addons=(),
        completion_handler_factory=default_completion_handler_factory,
        completion_guards_factory=default_completion_guards_factory,
    )

    async with lifespan(_bare_app()):
        assert fake_client.flushed is False

    assert fake_client.flushed is True


async def test_lifespan_skips_flush_when_init_resources_is_false(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    flush_calls: list[str] = []

    def _should_not_be_called() -> None:
        flush_calls.append("called")

    monkeypatch.setattr(
        application_module,
        "_flush_langfuse_client",
        _should_not_be_called,
    )
    _patch_resource_lifecycle(monkeypatch)
    keys = {
        "LANGFUSE_PUBLIC_KEY": "pk",
        "LANGFUSE_SECRET_KEY": "sk",  # pragma: allowlist secret
    }
    settings = test_settings.model_copy(update=keys)

    lifespan = _build_lifespan(
        settings,
        init_resources=False,
        addons=(),
        completion_handler_factory=default_completion_handler_factory,
        completion_guards_factory=default_completion_guards_factory,
    )

    async with lifespan(_bare_app()):
        pass

    assert flush_calls == []


async def test_lifespan_skips_flush_when_langfuse_disabled(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    flush_calls: list[str] = []

    monkeypatch.setattr(
        application_module,
        "_flush_langfuse_client",
        lambda: flush_calls.append("called"),
    )
    _patch_resource_lifecycle(monkeypatch)

    lifespan = _build_lifespan(
        test_settings,
        init_resources=True,
        addons=(),
        completion_handler_factory=default_completion_handler_factory,
        completion_guards_factory=default_completion_guards_factory,
    )

    async with lifespan(_bare_app()):
        pass

    assert flush_calls == []


async def test_lifespan_flush_tolerates_missing_langfuse_package(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    monkeypatch.setitem(sys.modules, "langfuse", None)
    _patch_resource_lifecycle(monkeypatch)
    keys = {
        "LANGFUSE_PUBLIC_KEY": "pk",
        "LANGFUSE_SECRET_KEY": "sk",  # pragma: allowlist secret
    }
    settings = test_settings.model_copy(update=keys)

    lifespan = _build_lifespan(
        settings,
        init_resources=True,
        addons=(),
        completion_handler_factory=default_completion_handler_factory,
        completion_guards_factory=default_completion_guards_factory,
    )

    async with lifespan(_bare_app()):
        pass


class _TrackedRedis(fake_aioredis.FakeRedis):
    closed = False

    async def aclose(self, *args, **kwargs) -> None:
        self.closed = True
        await super().aclose(*args, **kwargs)


async def test_app_lifespan_owns_the_redis_resource(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    redis = _TrackedRedis(decode_responses=True)
    monkeypatch.setattr(resources_module, "build_redis_client", lambda s: redis)
    app = create_app(settings=test_settings, init_resources=True)

    assert app.state.resources.redis is None

    async with app.router.lifespan_context(app):
        assert app.state.resources.redis is redis
        assert redis.closed is False

    assert redis.closed is True
    assert app.state.resources.redis is None


async def test_redis_opens_even_when_init_resources_is_false(
    monkeypatch: pytest.MonkeyPatch,
    test_settings: Settings,
):
    redis = _TrackedRedis(decode_responses=True)
    monkeypatch.setattr(resources_module, "build_redis_client", lambda s: redis)
    app = create_app(settings=test_settings, init_resources=False)

    async with app.router.lifespan_context(app):
        assert app.state.resources.redis is redis
        assert app.state.resources.recommendation is not None

    assert redis.closed is True


def test_resource_lifecycle_uses_named_open_steps():
    for name in ("_open_redis", "build_health_service"):
        assert callable(getattr(resources_module, name))


async def test_opener_registers_the_redis_readiness_check_with_the_timeout(
    test_settings: Settings,
):
    settings = test_settings.model_copy(update={"HEALTH_CHECK_TIMEOUT_SECONDS": 0.75})

    async with resources_module.open_application_resources(
        settings, init_resources=True
    ) as resources:
        service = resources_module.build_health_service(resources, init_resources=True)

        assert [name for name, _ in service.checks] == ["redis"]
        assert service.check_timeout_seconds == 0.75
