"""Task 1.1.1 of deploy-go-sdk-v1: local compose after the go-sdk v1 migration.

Scenarios: A migrate service runs the service image; A Go service waits for its migration and no
removed key remains; The gateway listens on GRPC_PORT; The relay block sets its gRPC port and
environment; The new dead-letter topics are created locally; The compose file parses.
"""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest
import yaml

ROOT = Path(__file__).resolve().parents[3]
COMPOSE = ROOT / "docker-compose.services.yaml"
BASE_COMPOSE = ROOT / "docker-compose.yaml"

DB_SERVICES = [
    "chat",
    "verification",
    "search",
    "audit",
    "notification",
    "promotion",
    "payment",
    "engagement",
    "domain",
    "order",
    "identity",
    "sharing",
    "referral",
]
GO_BLOCKS = [f"team-{s}" for s in DB_SERVICES] + [
    "team-search-indexer",
    "team-gateway",
    "team-analytics",
    "ga4-relay",
]
REMOVED_KEYS = [
    "KAFKA_ENABLED",
    "OUTBOX_ENABLED",
    "DATABASE_ENABLED",
    "FEATURE_FLAGS_ENABLED",
    "HTTP_PORT",
    "GA4_RELAY_PROBE_PORT",
    "GA4_EXPORT_SOURCE",
    "EDGE_REFLECTION_ENABLED",
    "IDENTITY_CONSUMER_ENABLED",
    "MOCK_PAYMENTS",
    "PAYMENT_SETTLEMENT_CONSUMER_ENABLED",
    "OUTBOX_MAX_ATTEMPTS",
    "OUTBOX_CLAIM_LOCK_SECONDS",
    "FEATURE_FLAGS_EVAL_TIMEOUT_MS",
    "UPSTREAM_CALL_TIMEOUT_SECONDS",
    "ENGAGEMENT_OUTBOX_RELAY_INTERVAL",
]


def _services() -> dict:
    return yaml.safe_load(COMPOSE.read_text())["services"]


def _env(svc: dict) -> dict:
    env = svc.get("environment") or {}
    if isinstance(env, dict):
        return {k: str(v) for k, v in env.items()}
    out = {}
    for item in env:
        k, _, v = item.partition("=")
        out[k] = v
    return out


def _depends(svc: dict) -> dict:
    dep = svc.get("depends_on") or {}
    if isinstance(dep, dict):
        return {k: (v or {}).get("condition") for k, v in dep.items()}
    return {k: None for k in dep}


def _command(svc: dict) -> list:
    cmd = svc.get("command")
    if isinstance(cmd, str):
        return cmd.split()
    return list(cmd or [])


def test_migrate_service_runs_service_image_all_database_services():
    services = _services()
    for s in DB_SERVICES:
        mig = services[f"team-{s}-migrate"]
        assert mig["image"] == f"team-{s}:local", s
        assert "migrate/migrate" not in mig["image"], s
        cmd = _command(mig)
        assert cmd[-2:] == ["migrate", "up"], (s, cmd)
        assert cmd[:-2] in ([], ["/server"]), (s, cmd)
        assert "volumes" not in mig or not mig["volumes"], s
        assert str(mig.get("restart")) == "no", s
        assert _depends(mig).get("postgres") == "service_healthy", s


def test_migrate_services_run_service_image_environment_matches_service():
    services = _services()
    for s in DB_SERVICES:
        mig_env = _env(services[f"team-{s}-migrate"])
        svc_env = _env(services[f"team-{s}"])
        assert mig_env.get("ENV") == "local", s
        assert mig_env.get("DATABASE_URL"), s
        assert mig_env["DATABASE_URL"] == svc_env["DATABASE_URL"], s


def test_migrate_services_run_service_image_order_sharing_referral_named_examples():
    services = _services()
    for s in ("order", "sharing", "referral"):
        mig = services[f"team-{s}-migrate"]
        assert mig["image"] == f"team-{s}:local"
        assert _command(mig) == ["migrate", "up"]
        assert _env(mig)["ENV"] == "local"


def test_migrate_services_run_service_image_no_golang_migrate_image_anywhere():
    for name, svc in _services().items():
        if name.endswith("-migrate"):
            assert "migrate/migrate" not in str(svc.get("image", "")), name


def test_go_services_depend_on_migrate_and_carry_no_removed_key():
    services = _services()
    for s in DB_SERVICES:
        dep = _depends(services[f"team-{s}"])
        assert dep.get(f"team-{s}-migrate") == "service_completed_successfully", s
    indexer = services["team-search-indexer"]
    assert "team-search-migrate" not in _depends(indexer)
    for name in GO_BLOCKS:
        env = _env(services[name])
        present = [k for k in REMOVED_KEYS if k in env]
        assert not present, (name, present)
    indexer_env = _env(indexer)
    assert not [k for k in indexer_env if k.startswith("DATABASE_")]


def test_go_services_depend_on_migrate_and_carry_no_removed_key_overrides():
    services = _services()
    assert _env(services["team-engagement"])["LOG_KEY_FP_SECRET"] == "local-e2e-log-key-fp-secret"
    assert _env(services["team-order"])["LOG_KEY_FP_SECRET"] == "local-dev-key-fp-secret"
    assert _env(services["team-promotion"])["FEATURE_FLAGS_EVAL_TIMEOUT"] == "500ms"
    assert _env(services["team-payment"])["MOCK_DATA"] == "payment-provider"
    domain_dep = _depends(services["team-domain"])
    assert domain_dep.get("redpanda") == "service_healthy"
    assert "redpanda-init" in domain_dep


def test_go_services_enable_reflection_locally():
    services = _services()
    for name in GO_BLOCKS:
        env = _env(services[name])
        assert env.get("GRPC_REFLECTION_ENABLED") == "true", name
        assert env.get("ENV") == "local", name


def test_gateway_uses_grpc_port():
    gw = _services()["team-gateway"]
    env = _env(gw)
    assert env["GRPC_PORT"] == "8080"
    assert env["ENV"] == "local"
    assert "HTTP_PORT" not in env
    assert "EDGE_REFLECTION_ENABLED" not in env
    assert "8080:8080" in [str(p) for p in gw["ports"]]


def test_relay_sets_grpc_port_and_env():
    env = _env(_services()["ga4-relay"])
    assert env["GRPC_PORT"] == "50059"
    assert env["ENV"] == "local"
    assert env["GRPC_REFLECTION_ENABLED"] == "true"
    assert "GA4_RELAY_PROBE_PORT" not in env


def test_redpanda_init_creates_new_dlq_topics():
    command = _services()["redpanda-init"]["command"]
    text = command if isinstance(command, str) else "\n".join(command)
    topics = set()
    for line in text.splitlines():
        if "rpk topic create" in line:
            topics.update(line.split("rpk topic create", 1)[1].split("-X", 1)[0].split())
    for topic in ("analytics.events.dlq", "listing.events.analytics.dlq", "identity.events.dlq"):
        assert topic in topics, topic


def test_compose_config_parses():
    result = subprocess.run(
        ["docker", "compose", "-f", str(BASE_COMPOSE), "-f", str(COMPOSE), "config", "-q"],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
