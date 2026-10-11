"""Admin bootstrap steps (OpenSpec change secure-seller-analytics-and-admin-seed, area c2)."""

from __future__ import annotations

import base64
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path

import httpx
import pytest
import yaml
from pytest_bdd import given, then, when

from config.settings import get_settings
from src.api.services import AuthService
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

GITOPS = pe.REPO_ROOT / "platform-gitops"
MIGRATE_IMAGE = "migrate/migrate:v4.17.1"
POSTGRES_CONTAINER = "agora-postgres-1"
BOOT_SECONDS = 8
RUN_TIMEOUT_S = 60


def _postgres_container() -> str:
    return os.getenv("POSTGRES_CONTAINER", POSTGRES_CONTAINER)


def _psql(db: str, sql: str, user: str = "postgres") -> str:
    out = pe.docker("exec", _postgres_container(), "psql", "-U", user, "-d", db, "-tAc", sql)
    return out.stdout.strip()


def _identity_env() -> dict[str, str]:
    env = pe.compose_env("team-identity")
    for key in [k for k in env if k.startswith("SEED_ADMIN_")]:
        del env[key]
    return env


def _docker_run_env(env: dict[str, str]) -> list[str]:
    args: list[str] = []
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    return args


@given("a scratch identity database migrated with the real migrations")
def scratch_database(world: World) -> None:
    db = f"e2e_noadmin_{uuid.uuid4().hex[:8]}"
    _psql("postgres", f"CREATE DATABASE {db} OWNER identity_svc")
    world.add_cleanup(
        lambda: pe.docker(
            "exec",
            _postgres_container(),
            "psql",
            "-U",
            "postgres",
            "-c",
            f"DROP DATABASE IF EXISTS {db} WITH (FORCE)",
            check=False,
        )
    )
    migrations = pe.REPO_ROOT / "team-identity" / "migrations"
    url = f"postgres://identity_svc:identity_pass@postgres:5432/{db}?sslmode=disable"
    run = pe.docker(
        "run", "--rm", "--network", pe.stack_network(),
        "-v", f"{migrations}:/migrations:ro", MIGRATE_IMAGE,
        "-path=/migrations", f"-database={url}", "up",
        check=False,
    )  # fmt: skip
    assert run.returncode == 0, f"migrate failed: {run.stdout}{run.stderr}"
    world.state.extra["scratch_db"] = db
    world.state.extra["scratch_url"] = url


@when("the team-identity image is started against it with no SEED_ADMIN variables")
def start_without_seed(world: World) -> None:
    env = _identity_env()
    env.update(
        DATABASE_URL=world.state.extra["scratch_url"],
        KAFKA_ENABLED="false",
        OUTBOX_ENABLED="false",
    )
    name = f"e2e-noadmin-identity-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    pe.docker(
        "run", "-d", "--name", name, "--network", pe.stack_network(),
        *_docker_run_env(env), pe.identity_boot_image(),
    )  # fmt: skip
    time.sleep(BOOT_SECONDS)
    state = pe.docker("inspect", name, "--format", "{{.State.Status}}").stdout.strip()
    logs = pe.docker("logs", name, check=False)
    assert state == "running", f"identity did not boot: {state}\n{logs.stdout}{logs.stderr}"


@then("the scratch database holds no admin user and no user named admin")
def scratch_has_no_admin(world: World) -> None:
    db = world.state.extra["scratch_db"]
    # The migrations ran (the table exists) and the booted service left it empty of admins.
    assert _psql(db, "SELECT count(*) FROM users WHERE 'admin' = ANY(roles)") == "0"
    assert _psql(db, "SELECT count(*) FROM users WHERE username = 'admin'") == "0"


@then("a login as admin with the password admin123 fails at the gateway")
def builtin_login_fails(world: World) -> None:
    url = get_settings().gateway_url.rstrip("/")
    r = httpx.post(
        f"{url}/platform.identity.v1.AuthService/Login",
        json={"username": "admin", "password": "admin123"},
        timeout=10,
    )
    assert r.status_code in (
        401,
        403,
        400,
    ), f"built-in admin login was not refused: {r.status_code} {r.text}"
    assert "token" not in r.text


@when("the team-identity image is started with SEED_ADMIN_ENABLED=true and no SEED_ADMIN_PASSWORD")
def start_enabled_without_password(world: World) -> None:
    env = _identity_env()
    env.update(SEED_ADMIN_ENABLED="true", SEED_ADMIN_PASSWORD="")
    name = f"e2e-seedpw-boot-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    cmd = ["docker", "run", "--rm", "--name", name, "--network", pe.stack_network()]
    cmd += _docker_run_env(env) + [pe.identity_boot_image()]
    try:
        world.state.extra["pe_boot"] = subprocess.run(
            cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S
        )
    except subprocess.TimeoutExpired as exc:
        pe.docker("rm", "-f", name, check=False)
        raise AssertionError(
            f"identity kept running instead of refusing to start: {exc.stdout}"
        ) from exc


@then("the identity process exits non-zero and its log names SEED_ADMIN_PASSWORD")
def refused_naming_password(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["pe_boot"]
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0, f"exit 0 with seeding enabled and no password:\n{log}"
    assert "SEED_ADMIN_PASSWORD" in log, log


@when("the e2e admin logs in with the credentials from the local compose environment")
def admin_logs_in(world: World) -> None:
    compose = pe.compose_env("team-identity")
    password = compose["SEED_ADMIN_PASSWORD"]
    # Compose writes `${SEED_ADMIN_PASSWORD:-<dev default>}`: an override from the environment
    # wins, otherwise the dev default is the password the stack was started with.
    m = re.fullmatch(r"\$\{SEED_ADMIN_PASSWORD:-(.*)\}", password)
    if m:
        password = os.environ.get("SEED_ADMIN_PASSWORD") or m.group(1)
    world.state.extra["admin_token"] = AuthService().login(compose["SEED_ADMIN_USERNAME"], password)


@then("the login succeeds and the token carries the admin role")
def admin_token_has_role(world: World) -> None:
    token = world.state.extra["admin_token"]
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    claims = json.loads(base64.urlsafe_b64decode(payload))
    blob = json.dumps(claims)
    assert "admin" in blob, f"no admin role/scope in claims: {claims}"


def _render(service: str, env: str | None) -> str:
    if shutil.which("helm") is None:
        pytest.fail("helm is not installed")
    files = [f"envs/services/{service}.yaml"]
    if env:
        files = [
            f"envs/{env}/values.yaml",
            f"envs/services/{service}.yaml",
            f"envs/{env}/services.yaml",
            f"envs/{env}/services/{service}.yaml",
        ]
    cmd = ["helm", "template", service, "charts/service", "--namespace", "marketplace"]
    for f in files:
        if (GITOPS / f).exists():
            cmd += ["-f", f]
    run = subprocess.run(cmd, cwd=GITOPS, capture_output=True, text=True, timeout=120)
    assert run.returncode == 0, f"helm template failed: {run.stderr[:600]}"
    return run.stdout


@when("the platform-gitops manifests for team-identity are rendered for every environment")
def render_identity(world: World) -> None:
    envs = [None] + sorted(
        p.name for p in (GITOPS / "envs").iterdir() if (p / "values.yaml").exists()
    )
    world.state.extra["rendered"] = {e or "base": _render("team-identity", e) for e in envs}
    assert len(world.state.extra["rendered"]) >= 3, world.state.extra["rendered"].keys()


@then("they contain no SEED_ADMIN_ENABLED=true and no admin password literal")
def no_seed_in_rendered(world: World) -> None:
    for env, text in world.state.extra["rendered"].items():
        for doc in yaml.safe_load_all(text):
            flat = yaml.safe_dump(doc or {})
            assert not re.search(
                r"SEED_ADMIN_PASSWORD", flat
            ), f"{env}: SEED_ADMIN_PASSWORD present"
            enabled = re.search(
                r"SEED_ADMIN_ENABLED['\"]?\s*:\s*['\"]?(true|yes|on|1)\b", flat, re.I
            )
            assert not enabled, f"{env}: SEED_ADMIN_ENABLED enabled"


@then("the platform-gitops seed check passes on them and fails on a crafted bad sample")
def seed_check_bites(world: World) -> None:
    script = GITOPS / "scripts" / "check_identity_seed.py"
    clean = subprocess.run(
        [sys.executable, str(script), str(GITOPS)], capture_output=True, text=True
    )
    assert clean.returncode == 0, clean.stdout + clean.stderr
    with tempfile.TemporaryDirectory() as tmp:
        bad = Path(tmp) / "env.yaml"
        bad.write_text(
            "env:\n  - SEED_ADMIN_ENABLED=true\n  - SEED_ADMIN_PASSWORD=hunter2hunter2\n"
        )
        dirty = subprocess.run([sys.executable, str(script), tmp], capture_output=True, text=True)
    assert dirty.returncode == 1, dirty.stdout + dirty.stderr
    assert "SEED_ADMIN" in dirty.stdout + dirty.stderr
