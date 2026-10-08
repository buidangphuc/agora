"""team-search production boot guard (port-security-hardening / deploy-runtime).

Runs the real team-search image built by the stack (agora-team-search:local) as a black box with `docker run --rm`; the
assertions are the exit code and the log, nothing in-process.
"""

from __future__ import annotations

import os
import subprocess

from pytest_bdd import given, then, when

from tests.e2e.support.world import World

# the image the agora stack builds (compose project "agora"); override for other stacks
IMAGE = os.environ.get("SEARCH_BOOT_IMAGE", "agora-team-search:local")
GUARD_MARKER = "refusing to start with in-memory saved-search storage"


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


@when("it is started with ENV production and DATABASE_ENABLED false")
def start_production(world: World) -> None:
    world.state.extra["boot"] = _run({"ENV": "production", "DATABASE_ENABLED": "false"})


@then("the process exits non-zero")
def exits_non_zero(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["boot"]
    assert proc.returncode != 0, f"exit {proc.returncode}: {proc.stdout}{proc.stderr}"


@then("its log names the storage setting")
def log_names_storage(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["boot"]
    log = proc.stdout + proc.stderr
    assert "DATABASE_ENABLED" in log and GUARD_MARKER in log, log


@then("the same image with ENV local does not trip the storage guard")
def local_passes_guard(world: World) -> None:
    """Control: the failure above is the guard, not a generic boot failure."""
    # Unreachable OpenSearch makes the local boot exit quickly, after the guard.
    proc = _run(
        {
            "ENV": "local",
            "DATABASE_ENABLED": "false",
            "OPENSEARCH_URL": "http://opensearch.invalid:9200",
        }
    )
    assert GUARD_MARKER not in proc.stdout + proc.stderr, proc.stdout + proc.stderr
