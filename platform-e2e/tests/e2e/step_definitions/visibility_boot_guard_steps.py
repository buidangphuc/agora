"""team-search boot guard: a deployed ENV needs a reachable database (migrate-team-search-to-go-sdk).

Runs the real team-search image built by the stack (agora-team-search:local) as a black box with `docker run --rm`; the
assertions are the exit code and the log, nothing in-process.
"""

from __future__ import annotations

import os
import subprocess
import time

from pytest_bdd import given, then, when

from tests.e2e.support.world import World

# the image the agora stack builds (compose project "agora"); override for other stacks
IMAGE = os.environ.get("SEARCH_BOOT_IMAGE", "agora-team-search:local")
# go-sdk v1 requires brokers for a strict ENV; set so the database check is what fails.
BROKERS = "redpanda:9092"
UNREACHABLE_DB = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
OPENSEARCH_URL = os.environ.get("SEARCH_BOOT_OPENSEARCH_URL", "http://host.docker.internal:9200")
FALLBACK_MARKER = "saved searches use an in-memory store"


def _run(env: dict[str, str]) -> subprocess.CompletedProcess[str]:
    args = ["docker", "run", "--rm"]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    args.append(IMAGE)
    return subprocess.run(args, capture_output=True, text=True, timeout=120, check=False)


@given("the team-search image")
def team_search_image(world: World) -> None:
    probe = subprocess.run(
        ["docker", "image", "inspect", IMAGE], capture_output=True, text=True, check=False
    )
    assert probe.returncode == 0, f"{IMAGE} is not built locally: {probe.stderr}"


@when("it is started with ENV production and no DATABASE_URL")
def start_production(world: World) -> None:
    world.state.extra["boot"] = _run(
        {"ENV": "production", "GRPC_PORT": "50052", "KAFKA_BROKERS": BROKERS}
    )


@then("the process exits non-zero")
def exits_non_zero(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["boot"]
    assert proc.returncode != 0, f"exit {proc.returncode}: {proc.stdout}{proc.stderr}"


@then("its log names the database setting")
def log_names_database(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["boot"]
    log = proc.stdout + proc.stderr
    assert "DATABASE_ENABLED" in log and "DATABASE_URL" in log, log


@then(
    "the same image with ENV production, DATABASE_ENABLED true and an unreachable database exits non-zero"
)
def production_unreachable(world: World) -> None:
    proc = _run(
        {
            "ENV": "production",
            "GRPC_PORT": "50052",
            "KAFKA_BROKERS": BROKERS,
            "DATABASE_ENABLED": "true",
            "DATABASE_URL": UNREACHABLE_DB,
            "OPENSEARCH_URL": OPENSEARCH_URL,
        }
    )
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0 and "postgres" in log, log


@then("the same image with ENV local and DATABASE_ENABLED false falls back to in-memory storage")
def local_falls_back(world: World) -> None:
    """Control: the failure above is the database rule, not a generic boot failure."""
    env = {
        "ENV": "local",
        "GRPC_PORT": "50052",
        "DATABASE_ENABLED": "false",
        "DATABASE_URL": UNREACHABLE_DB,
        "OPENSEARCH_URL": OPENSEARCH_URL,
    }
    args = ["docker", "run", "-d", "--rm"]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    cid = subprocess.run([*args, IMAGE], capture_output=True, text=True, check=True).stdout.strip()
    log = ""
    try:
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline and FALLBACK_MARKER not in log:
            time.sleep(1)
            logs = subprocess.run(["docker", "logs", cid], capture_output=True, text=True)
            log = logs.stdout + logs.stderr
    finally:
        subprocess.run(["docker", "rm", "-f", cid], capture_output=True, check=False)
    assert FALLBACK_MARKER in log and "ping postgres" not in log, log
