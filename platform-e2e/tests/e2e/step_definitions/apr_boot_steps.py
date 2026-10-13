"""Boot guard of team-ai's production-only settings validation (black box, `docker run`).

The real team-ai image is started on the stack network with the compose environment plus
the offending setting; the assertions are the exit code and the log. The other
production-only validations (weak API key, wildcard hosts) are satisfied, so the refusal can
only come from the docs guard. team-ai reads `ENVIRONMENT`; `ENV` is passed too, since the
spec names it. The rate-limit boot guard no longer exists: the limits are always on, in Redis.
"""

from __future__ import annotations

import hashlib
import os
import secrets
import subprocess
import time
import uuid

from pytest_bdd import given, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

RUN_TIMEOUT_S = 60
SETTING = "DOCS_ENABLED"


def _x(world: World) -> dict:
    return world.state.extra


def _image() -> str:
    explicit = os.getenv("AI_BOOT_IMAGE")
    if explicit:
        return explicit
    return pe.docker("inspect", pe.ai_container(), "--format", "{{.Config.Image}}").stdout.strip()


def _env(environment: str) -> dict[str, str]:
    env = pe.compose_env("team-ai")
    env.update(
        ENVIRONMENT=environment,
        ENV=environment,
        # AUTH_API_KEYS holds sha256 digests only: a fresh random token for a read-only principal.
        AUTH_API_KEYS="e2e-boot:ai.classify:"
        + hashlib.sha256(secrets.token_urlsafe(32).encode()).hexdigest(),
        DOCS_ENABLED="true",
        CORS_ALLOW_ORIGINS="https://example.invalid",
        TRUSTED_HOSTS="example.invalid",
    )
    return env


def _docker_run(world: World, env: dict[str, str], *flags: str) -> tuple[str, list[str]]:
    name = f"e2e-apr-ai-boot-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    cmd = ["docker", "run", "--name", name, "--network", pe.stack_network(), *flags]
    for key, value in env.items():
        cmd += ["-e", f"{key}={value}"]
    return name, [*cmd, _image()]


@given("the team-ai image")
def team_ai_image(world: World) -> None:
    _x(world)["apr_image"] = _image()


@when("it is started with ENV=production and DOCS_ENABLED=true")
def start_production(world: World) -> None:
    name, cmd = _docker_run(world, _env("production"), "--rm")
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    except subprocess.TimeoutExpired as exc:
        pe.docker("rm", "-f", name, check=False)
        raise AssertionError(
            f"team-ai kept running instead of refusing to boot: {exc.stdout}"
        ) from exc
    _x(world)["apr_boot"] = proc


@then("the process exits non-zero and its log names DOCS_ENABLED")
def refused(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["apr_boot"]
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0, f"exit 0, booted with the API docs on in production:\n{log}"
    assert SETTING in log, log


@then("the same image with ENV=local does not trip the guard")
def local_boots(world: World) -> None:
    name, cmd = _docker_run(world, _env("local"), "-d")
    pe.docker(*cmd[1:])
    deadline = time.monotonic() + 30
    log = ""
    while time.monotonic() < deadline:
        state = pe.docker(
            "inspect", name, "--format", "{{.State.Status}}", check=False
        ).stdout.strip()
        out = pe.docker("logs", name, check=False)
        log = out.stdout + out.stderr
        assert SETTING not in log, f"the guard tripped in a local ENV:\n{log}"
        if state != "running":
            raise AssertionError(f"team-ai exited ({state}) in a local ENV:\n{log}")
        if "Uvicorn running" in log or "gRPC" in log or "grpc" in log:
            return
        time.sleep(1)
    # still running after the wait without naming the setting: the guard did not fire
    assert SETTING not in log, log
