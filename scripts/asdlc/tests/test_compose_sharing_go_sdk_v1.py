"""Task 2.1 of sharing-go-sdk-v1, scenario: The removed enable flag is ignored.

team-sharing no longer reads DATABASE_ENABLED, so its compose service must not set it; the rest of its
environment stays untouched.
"""

from __future__ import annotations

from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
COMPOSE = ROOT / "docker-compose.services.yaml"


def _sharing_env() -> dict:
    svc = yaml.safe_load(COMPOSE.read_text())["services"]["team-sharing"]
    env = svc["environment"]
    if isinstance(env, dict):
        return {k: str(v) for k, v in env.items()}
    return dict(item.split("=", 1) for item in env)


def test_removed_enable_flag_is_ignored_team_sharing_does_not_set_database_enabled():
    assert "DATABASE_ENABLED" not in _sharing_env()


def test_removed_enable_flag_is_ignored_team_sharing_keeps_other_environment():
    env = _sharing_env()
    assert env["DATABASE_URL"].startswith("postgres://sharing_svc:")
    assert env["GRPC_PORT"] == "50065"
    assert env["ENV"] == "local"


REQUIRED_UPSTREAMS = {
    "team-domain": ["UPSTREAM_AUDIT_ADDR", "UPSTREAM_AI_ADDR"],
    "team-engagement": ["UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR"],
    "team-identity": ["UPSTREAM_AUDIT_ADDR"],
    "team-notification": ["UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR"],
    "team-order": [
        "UPSTREAM_DOMAIN_ADDR",
        "UPSTREAM_IDENTITY_ADDR",
        "UPSTREAM_PROMOTION_ADDR",
        "UPSTREAM_AUDIT_ADDR",
    ],
    "team-payment": ["UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR"],
    "team-promotion": ["UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR"],
    "team-verification": ["UPSTREAM_AUDIT_ADDR"],
    "team-search-indexer": ["UPSTREAM_AI_ADDR"],
}
GO_SERVICES = [
    "team-chat",
    "team-verification",
    "team-search",
    "team-search-indexer",
    "team-audit",
    "team-notification",
    "team-promotion",
    "team-payment",
    "team-engagement",
    "team-domain",
    "team-order",
    "team-identity",
    "team-sharing",
    "team-referral",
    "team-gateway",
    "team-analytics",
    "ga4-relay",
]


def _all() -> dict:
    return yaml.safe_load(COMPOSE.read_text())["services"]


def _env_of(svc: dict) -> dict:
    env = svc.get("environment") or {}
    if isinstance(env, dict):
        return {k: str(v) for k, v in env.items()}
    return dict(item.partition("=")[::2] for item in env)


def test_a_go_service_waits_for_its_migration_and_no_removed_key_remains_required_upstreams_set():
    services = _all()
    for name, keys in REQUIRED_UPSTREAMS.items():
        env = _env_of(services[name])
        for key in keys:
            assert env.get(key), (name, key)


def test_a_go_service_waits_for_its_migration_and_no_removed_key_remains_every_go_service_has_env():
    services = _all()
    for name in GO_SERVICES:
        assert _env_of(services[name]).get("ENV") == "local", name


def test_a_go_service_waits_for_its_migration_and_no_removed_key_remains_enable_flags_absent_everywhere():
    removed = {
        "KAFKA_ENABLED",
        "OUTBOX_ENABLED",
        "DATABASE_ENABLED",
        "FEATURE_FLAGS_ENABLED",
        "GA4_RELAY_PROBE_PORT",
        "GA4_EXPORT_SOURCE",
    }
    for name, svc in _all().items():
        if name in ("team-ai", "team-frontend"):
            continue
        assert not removed & set(_env_of(svc)), name


def test_a_migrate_service_runs_the_service_image_chat_uses_explicit_server_entrypoint_command():
    mig = _all()["team-chat-migrate"]
    assert mig["image"] == "team-chat:local"
    assert mig["command"] == ["/server", "migrate", "up"]


def test_a_go_service_waits_for_its_migration_and_no_removed_key_remains_local_timing_overrides():
    services = _all()
    assert _env_of(services["team-chat"])["OUTBOX_CLAIM_LOCK"] == "60s"
    assert _env_of(services["team-engagement"])["OUTBOX_POLL_INTERVAL"] == "500ms"
    for name in ("team-chat", "team-engagement", "team-order"):
        assert "OUTBOX_CLAIM_LOCK_SECONDS" not in _env_of(services[name]), name


def test_the_new_dead_letter_topics_are_created_locally_every_consumer_dlq_has_a_topic():
    command = _all()["redpanda-init"]["command"]
    text = command if isinstance(command, str) else "\n".join(command)
    created = set(text.split("rpk topic create", 1)[1].split("-X", 1)[0].split())
    for topic in (
        "analytics.events.dlq",
        "listing.events.analytics.dlq",
        "identity.events.dlq",
        "analytics.ga4-relay.dlq",
        "engagement.events.analytics.dlq",
        "engagement.events.order.dlq",
        "order.events.payment-settlement.dlq",
        "listing.events.dlq",
        "chat.events.dlq",
        "order.events.dlq",
        "payment.events.dlq",
    ):
        assert topic in created, topic
