"""Static checks of docker-compose.services.yaml for analytics-user-erasure task 1.12.

Scenarios AUE-33 (DLQs created, consumer env, recsys erasure service) and AUE-34 (topic-init before the
consumer becomes healthy, non-root long-running service).
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[2]
COMPOSE = ROOT / "docker-compose.services.yaml"
DLQS = ["identity.events.analytics.dlq", "identity.events.recsys.dlq", "identity.events.team-ai.dlq"]


@pytest.fixture(scope="module")
def services() -> dict:
    return yaml.safe_load(COMPOSE.read_text())["services"]


def _env(svc: dict) -> dict:
    env = svc.get("environment", {})
    if isinstance(env, list):
        return dict(e.split("=", 1) for e in env)
    return {k: str(v) for k, v in env.items()}


def _depends(svc: dict) -> dict:
    dep = svc.get("depends_on", {})
    return {d: {} for d in dep} if isinstance(dep, list) else dep


def test_aue33_topic_init_creates_the_three_dlq_topics(services):
    command = "\n".join(services["redpanda-init"]["command"])
    created = re.search(r"rpk topic create (.*?) -X brokers", command, re.S).group(1).split()
    for topic in DLQS:
        assert topic in created


def test_aue33_analytics_consumer_env(services):
    env = _env(services["team-analytics"])
    assert env.get("IDENTITY_EVENTS_TOPIC") == "identity.events"
    assert env.get("ERASURE_CONSUMER_GROUP") == "team-analytics.user-erasure"
    assert env.get("ERASURE_DLQ_TOPIC") == "identity.events.analytics.dlq"


def test_aue33_team_ai_consumer_env(services):
    env = _env(services["team-ai"])
    by_suffix = {k: v for k, v in env.items() if "ERASURE" in k}
    assert "team-ai.user-erasure" in by_suffix.values()
    assert "identity.events.team-ai.dlq" in by_suffix.values()


def test_aue33_recsys_erasure_service_is_long_running_with_env(services):
    svc = services["platform-recsys-erasure"]
    assert svc["entrypoint"] == ["python", "-m", "recsys.erasure"]
    assert svc.get("restart") in ("unless-stopped", "always", "on-failure")
    assert "profiles" not in svc
    env = _env(svc)
    assert env["KAFKA_BROKERS"] == "redpanda:9092"
    assert env["REDIS_HOST"] == "redis"
    assert env["QDRANT_URL"] == "http://qdrant:6333"
    assert env["ERASURE_CONSUMER_GROUP"] == "platform-recsys.erasure"
    assert env["ERASURE_DLQ_TOPIC"] == "identity.events.recsys.dlq"


def test_aue34_recsys_erasure_waits_for_topic_init_and_is_non_root(services):
    svc = services["platform-recsys-erasure"]
    dep = _depends(svc)
    assert "redpanda" in dep and "redpanda-init" in dep
    assert svc.get("user") not in ("0", 0, "root", "0:0", "root:root")
    dockerfile = (ROOT / "platform-recsys" / "Dockerfile").read_text()
    assert re.search(r"^USER\s+(?!root|0\b)\S+", dockerfile, re.M)
    assert svc["build"]["dockerfile"] == "platform-recsys/Dockerfile"


def test_aue34_consumers_wait_for_topic_init(services):
    for name in ("team-analytics", "team-ai"):
        assert "redpanda-init" in _depends(services[name]), name
