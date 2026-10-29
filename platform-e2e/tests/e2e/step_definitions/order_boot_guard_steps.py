"""team-order boot guard: a deployed ENV needs DATABASE_URL (deploy-go-sdk-v1 / deploy-runtime).

Runs the real team-order image as a black box with `docker run --rm`; the assertions are the exit code and the
log, nothing in-process.
"""

from __future__ import annotations

import os
import subprocess

from pytest_bdd import given, then, when

from tests.e2e.support.world import World

IMAGE = os.environ.get("ORDER_BOOT_IMAGE", "team-order:local")
PRODUCTION_BASE = {
    "ENV": "production",
    "GRPC_PORT": "50056",
    "KAFKA_BROKERS": "redpanda:9092",
    "LOG_KEY_FP_SECRET": "boot-guard-log-key-fp-secret",
    "UPSTREAM_AUDIT_ADDR": "team-audit:50055",
    "UPSTREAM_DOMAIN_ADDR": "team-domain:50051",
    "UPSTREAM_IDENTITY_ADDR": "team-identity:50053",
    "UPSTREAM_PROMOTION_ADDR": "team-promotion:50061",
}


def _run(env: dict[str, str]) -> subprocess.CompletedProcess[str]:
    args = ["docker", "run", "--rm"]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    args.append(IMAGE)
    return subprocess.run(args, capture_output=True, text=True, timeout=120, check=False)


@given("the team-order image")
def team_order_image(world: World) -> None:
    probe = subprocess.run(
        ["docker", "image", "inspect", IMAGE], capture_output=True, text=True, check=False
    )
    assert probe.returncode == 0, f"{IMAGE} is not built locally: {probe.stderr}"


@when("it is started with ENV production and no DATABASE_URL")
def order_start_production(world: World) -> None:
    world.state.extra["order_boot"] = _run(dict(PRODUCTION_BASE))


@then("the process exits non-zero")
def order_exits_non_zero(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["order_boot"]
    assert proc.returncode != 0, f"exit {proc.returncode}: {proc.stdout}{proc.stderr}"


@then("its log names DATABASE_URL")
def order_log_names_database_url(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["order_boot"]
    log = proc.stdout + proc.stderr
    assert "DATABASE_URL" in log, log
