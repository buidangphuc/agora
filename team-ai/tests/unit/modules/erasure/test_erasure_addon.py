"""AUE-28..AUE-31 wiring: erasure addon settings, DLQ topic and registration."""

from __future__ import annotations

import asyncio
from contextlib import AsyncExitStack

import pytest
from fakeredis import FakeAsyncRedis

from app.bootstrap.addons import erasure as erasure_module
from app.bootstrap.addons.base import default_resource_addons
from app.bootstrap.addons.erasure import ErasureAddon
from app.bootstrap.resources import ApplicationResources
from app.modules.messaging.kafka.consumer import supervise
from tests.factories import build_test_settings


class _Idle:
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.stopped = False

    async def start(self) -> None:
        self.started.set()

    async def stop(self) -> None:
        self.stopped = True

    async def getone(self):
        await asyncio.Event().wait()
        raise AssertionError("unreachable")

    async def commit(self, record) -> None: ...

    async def send(self, record, *, error_type: str) -> None: ...


@pytest.fixture(autouse=True)
def real_supervisor(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(erasure_module, "supervise", supervise)


def test_aue_31_settings_defaults() -> None:
    settings = build_test_settings()

    assert settings.ERASURE_TOPIC == "identity.events"
    assert settings.ERASURE_GROUP == "team-ai.user-erasure"
    assert settings.ERASURE_DLQ_TOPIC == "identity.events.team-ai.dlq"


def test_aue_28_addon_registered() -> None:
    assert "erasure" in [a.name for a in default_resource_addons()]
    assert ErasureAddon().name == "erasure"


async def test_aue_31_open_consumes_identity_events_and_parks_on_team_ai_dlq() -> None:
    source, sink = _Idle(), _Idle()
    source_args: list[dict[str, str]] = []
    dlq_args: list[dict[str, str]] = []

    def build_source(*, brokers: str, topic: str, group: str) -> _Idle:
        source_args.append({"brokers": brokers, "topic": topic, "group": group})
        return source

    def build_dlq(*, brokers: str, topic: str) -> _Idle:
        dlq_args.append({"brokers": brokers, "topic": topic})
        return sink

    settings = build_test_settings(KAFKA_BROKERS="b:9092")
    resources = ApplicationResources()
    resources.redis = FakeAsyncRedis(decode_responses=True)  # type: ignore[assignment]
    addon = ErasureAddon(build_source=build_source, build_dlq=build_dlq)

    async with AsyncExitStack() as stack:
        await addon.open(resources, settings, stack)
        await asyncio.wait_for(source.started.wait(), 1)
        assert source_args == [
            {"brokers": "b:9092", "topic": "identity.events", "group": "team-ai.user-erasure"}
        ]
        assert dlq_args == [{"brokers": "b:9092", "topic": "identity.events.team-ai.dlq"}]

    assert source.stopped is True
    assert sink.stopped is True
