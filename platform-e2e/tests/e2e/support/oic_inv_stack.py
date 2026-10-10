"""Stack access for port-order-inventory-correctness (area oic-inv).

Container and image names come from the environment with the local compose defaults
(project `agora`: containers `agora-team-<svc>-svc`, images `agora-team-<svc>:local`):

    OIC_INV_<SVC>_CONTAINER / OIC_INV_<SVC>_IMAGE   e.g. OIC_INV_TEAM_DOMAIN_CONTAINER
    OIC_INV_POSTGRES_CONTAINER                      default agora-postgres-1
    OIC_INV_MAX_WAIT_S                              longest sweep wait a scenario may sit through

Reservation TTL scenarios need the short-TTL overlay
(`platform-e2e/compose/order-inventory.override.yaml`: RESERVATION_TTL=20s,
RESERVATION_SWEEP_INTERVAL=2s on team-domain and team-order). The effective cadence is read
from the running container's environment, so a stack without the overlay fails the wait
guard with a message naming the overlay instead of sleeping for 17 minutes.
"""

from __future__ import annotations

import json
import os
import re
import subprocess

DEFAULT_TTL_S = 15 * 60.0
DEFAULT_SWEEP_S = 60.0
OVERLAY = "platform-e2e/compose/order-inventory.override.yaml"
_DOCKER_TIMEOUT_S = 90
_UNITS = {"ns": 1e-9, "us": 1e-6, "µs": 1e-6, "ms": 1e-3, "s": 1.0, "m": 60.0, "h": 3600.0}


def _key(service: str) -> str:
    return service.upper().replace("-", "_")


def container_name(service: str) -> str:
    """Compose container of `team-domain` etc. (default `agora-team-domain-svc`)."""
    return os.getenv(f"OIC_INV_{_key(service)}_CONTAINER", f"agora-{service}-svc")


def image_name(service: str) -> str:
    return os.getenv(f"OIC_INV_{_key(service)}_IMAGE", f"agora-{service}:local")


def postgres_container() -> str:
    return os.getenv("OIC_INV_POSTGRES_CONTAINER", "agora-postgres-1")


def stack_network() -> str:
    return os.getenv("STACK_NETWORK", "agora_default")


def max_wait_s() -> float:
    return float(os.getenv("OIC_INV_MAX_WAIT_S", "180"))


def docker(*args: str, check: bool = True, timeout: int = _DOCKER_TIMEOUT_S):
    return subprocess.run(
        ["docker", *args], capture_output=True, text=True, timeout=timeout, check=check
    )


# ── Go durations ─────────────────────────────────────────────────────────
def parse_go_duration(text: str) -> float | None:
    """Seconds of a Go duration such as `20s`, `1m30s`, `15m0s`; None when unparsable."""
    text = text.strip()
    parts = re.findall(r"(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)", text)
    if not parts or "".join(n + u for n, u in parts) != text:
        return None
    return sum(float(n) * _UNITS[u] for n, u in parts)


def go_duration_string(seconds: float) -> str:
    """Format whole seconds the way Go's Duration.String() does (`20s`, `1m0s`, `15m0s`)."""
    total = int(round(seconds))
    h, rest = divmod(total, 3600)
    m, s = divmod(rest, 60)
    if h:
        return f"{h}h{m}m{s}s"
    if m:
        return f"{m}m{s}s"
    return f"{s}s"


# ── Effective reservation cadence ────────────────────────────────────────
def container_env(service: str) -> dict[str, str]:
    out = docker("inspect", container_name(service), "--format", "{{json .Config.Env}}").stdout
    env: dict[str, str] = {}
    for item in json.loads(out) or []:
        name, _, value = item.partition("=")
        env[name] = value
    return env


def configured_cadence(service: str) -> tuple[str | None, str | None]:
    """RESERVATION_TTL / RESERVATION_SWEEP_INTERVAL as set on the running container."""
    env = container_env(service)
    return env.get("RESERVATION_TTL"), env.get("RESERVATION_SWEEP_INTERVAL")


def effective_cadence_s(service: str) -> tuple[float, float]:
    """TTL and sweep interval (seconds) the service is expected to run with."""
    ttl_raw, sweep_raw = configured_cadence(service)
    ttl = parse_go_duration(ttl_raw) if ttl_raw else None
    sweep = parse_go_duration(sweep_raw) if sweep_raw else None
    return (
        ttl if ttl and ttl > 0 else DEFAULT_TTL_S,
        sweep if sweep and sweep > 0 else DEFAULT_SWEEP_S,
    )


def sweep_wait_s(margin_s: float = 3.0) -> float:
    """More than the TTL plus two sweep intervals, of BOTH services (the larger one)."""
    waits = []
    for service in ("team-domain", "team-order"):
        ttl, sweep = effective_cadence_s(service)
        waits.append(ttl + 2 * sweep)
    return max(waits) + margin_s


def require_wait_within_budget() -> float:
    """The wait a TTL scenario will sit through; fails fast when the overlay is not active."""
    wait = sweep_wait_s()
    if wait > max_wait_s():
        raise AssertionError(
            f"the reservation sweep wait is {wait:.0f}s (> OIC_INV_MAX_WAIT_S={max_wait_s():.0f}s): "
            f"the short-TTL overlay is not active on team-domain/team-order; start the stack with "
            f"`docker compose -f docker-compose.yaml -f {OVERLAY} up -d` "
            f"(or raise OIC_INV_MAX_WAIT_S to wait out the default 15m TTL)"
        )
    return wait


def container_logs(service: str) -> str:
    proc = docker("logs", container_name(service), check=False)
    return proc.stdout + proc.stderr


# ── Throwaway team-domain boot (config fallback scenario) ────────────────
def create_scratch_database(name: str) -> str:
    """Empty database owned by the listing role, for a throwaway team-domain process."""
    docker(
        "exec", postgres_container(), "psql", "-U", "postgres", "-c",
        f'CREATE DATABASE "{name}" OWNER listing_svc',
    )  # fmt: skip
    return f"postgres://listing_svc:listing_pass@postgres:5432/{name}?sslmode=disable"


def drop_scratch_database(name: str) -> None:
    docker(
        "exec", postgres_container(), "psql", "-U", "postgres", "-c",
        f'DROP DATABASE IF EXISTS "{name}" WITH (FORCE)',
        check=False,
    )  # fmt: skip
