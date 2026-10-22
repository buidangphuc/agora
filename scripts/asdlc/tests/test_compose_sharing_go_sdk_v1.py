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
