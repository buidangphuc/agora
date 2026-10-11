"""Stack access for port-payment-ledger-integrity (area plp-pay).

Container and image names come from the environment with the local compose defaults
(project `agora`: containers `agora-team-<svc>-svc`, images `agora-team-<svc>:local`):

    PLP_<SVC>_CONTAINER / PLP_<SVC>_IMAGE   e.g. PLP_TEAM_PAYMENT_CONTAINER
    PLP_POSTGRES_CONTAINER                  default agora-postgres-1
    PLP_REDPANDA_CONTAINER                  default agora-redpanda-1
    PLP_MAX_WAIT_S                          longest hold-window wait a scenario may sit through

Hold-back scenarios that wait out the window need the short-window overlay
(`platform-e2e/compose/payment-ledger.override.yaml`: PAYOUT_HOLD_WINDOW=20s on team-payment).
The effective window is read from the running container's environment, so a stack without
the overlay fails the wait guard with a message naming the overlay instead of sleeping for
7 days.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import time
import uuid
from datetime import datetime, timezone

from config.settings import get_settings

DEFAULT_HOLD_DAYS = 7
OVERLAY = "platform-e2e/compose/payment-ledger.override.yaml"
SETTLEMENT_GROUP = os.getenv("PLP_SETTLEMENT_GROUP", "team-payment.settlement")
ORDER_EVENTS_TOPIC = "order.events"
DLQ_TOPIC = os.getenv("PLP_SETTLEMENT_DLQ", "order.events.payment-settlement.dlq")
_DOCKER_TIMEOUT_S = 90
_UNITS = {"ns": 1e-9, "us": 1e-6, "µs": 1e-6, "ms": 1e-3, "s": 1.0, "m": 60.0, "h": 3600.0}


def _key(service: str) -> str:
    return service.upper().replace("-", "_")


def container_name(service: str) -> str:
    """Compose container of `team-payment` etc. (default `agora-team-payment-svc`)."""
    return os.getenv(f"PLP_{_key(service)}_CONTAINER", f"agora-{service}-svc")


def image_name(service: str) -> str:
    return os.getenv(f"PLP_{_key(service)}_IMAGE", f"agora-{service}:local")


def postgres_container() -> str:
    return os.getenv("PLP_POSTGRES_CONTAINER", "agora-postgres-1")


def redpanda_container() -> str:
    return os.getenv("PLP_REDPANDA_CONTAINER", "agora-redpanda-1")


def stack_network() -> str:
    return os.getenv("STACK_NETWORK", "agora_default")


def max_wait_s() -> float:
    return float(os.getenv("PLP_MAX_WAIT_S", "180"))


def docker(*args: str, check: bool = True, timeout: int = _DOCKER_TIMEOUT_S):
    return subprocess.run(
        ["docker", *args], capture_output=True, text=True, timeout=timeout, check=check
    )


def container_age_s(service: str) -> float:
    """Seconds since the service's container last started (0 when it is not running)."""
    out = docker("inspect", "-f", "{{.State.StartedAt}}", container_name(service), check=False)
    if out.returncode != 0 or not out.stdout.strip():
        return 0.0
    started = datetime.fromisoformat(out.stdout.strip().replace("Z", "+00:00")[:26] + "+00:00")
    return (datetime.now(timezone.utc) - started).total_seconds()


def wait_settled(service: str, settle_s: float = 35.0, timeout: float = 90.0) -> None:
    """Block until the restarted service has been up long enough for its callers'
    gRPC clients to re-resolve it (deadline loop, no fixed sleep)."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline and container_age_s(service) < settle_s:
        time.sleep(1)


# ── Go durations ─────────────────────────────────────────────────────────
def parse_go_duration(text: str) -> float | None:
    """Seconds of a Go duration such as `20s`, `1m30s`, `168h0m0s`; None when unparsable."""
    text = text.strip()
    parts = re.findall(r"(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)", text)
    if not parts or "".join(n + u for n, u in parts) != text:
        return None
    return sum(float(n) * _UNITS[u] for n, u in parts)


# ── Effective hold window ────────────────────────────────────────────────
def container_env(service: str) -> dict[str, str]:
    out = docker("inspect", container_name(service), "--format", "{{json .Config.Env}}").stdout
    env: dict[str, str] = {}
    for item in json.loads(out) or []:
        name, _, value = item.partition("=")
        env[name] = value
    return env


def hold_window_s() -> float:
    """Hold window (seconds) the running team-payment is configured with.

    PAYOUT_HOLD_WINDOW (Go duration) wins over PAYOUT_HOLD_DAYS (default 7), as in the spec.
    """
    env = container_env("team-payment")
    raw = (env.get("PAYOUT_HOLD_WINDOW") or "").strip()
    if raw:
        parsed = parse_go_duration(raw)
        if parsed is not None:
            return parsed
    days = (env.get("PAYOUT_HOLD_DAYS") or "").strip()
    if days.lstrip("-").isdigit():
        return int(days) * 86400.0
    return DEFAULT_HOLD_DAYS * 86400.0


def require_hold_wait(margin_s: float = 4.0) -> float:
    """Seconds a scenario waits for a credit to leave the window; fails fast without the overlay."""
    window = hold_window_s()
    wait = window + margin_s
    if wait > max_wait_s():
        raise AssertionError(
            f"the payout hold window is {window:.0f}s (wait {wait:.0f}s > PLP_MAX_WAIT_S="
            f"{max_wait_s():.0f}s): the short-window overlay is not active on team-payment; start "
            f"the stack with `docker compose -f docker-compose.yaml -f {OVERLAY} up -d` "
            f"(or raise PLP_MAX_WAIT_S to wait out the default 7-day hold)"
        )
    return window


# ── payment_db through psql ──────────────────────────────────────────────
def psql(sql: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    """Run one statement in payment_db as the superuser (rows as `a|b|c`, no headers)."""
    return docker(
        "exec", postgres_container(), "psql", "-U", "postgres", "-d", "payment_db",
        "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        check=check,
    )  # fmt: skip


def psql_rows(sql: str) -> list[list[str]]:
    out = psql(sql).stdout.strip()
    return [line.split("|") for line in out.splitlines() if line]


def sql_lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


# ── team-payment lifecycle (destructive lane) ────────────────────────────
def stop_payment() -> None:
    docker("stop", container_name("team-payment"))


def start_payment() -> None:
    docker("start", container_name("team-payment"))


def payment_running() -> bool:
    out = docker(
        "inspect", container_name("team-payment"), "--format", "{{.State.Running}}", check=False
    ).stdout
    return out.strip() == "true"


# ── rpk on the redpanda container ────────────────────────────────────────
def rpk(*args: str, check: bool = True, timeout: int = 60) -> subprocess.CompletedProcess[str]:
    return docker("exec", redpanda_container(), "rpk", *args, check=check, timeout=timeout)


def group_exists(group: str = SETTLEMENT_GROUP) -> bool:
    out = rpk("group", "list", check=False).stdout
    return any(line.split()[-1:] == [group] for line in out.splitlines())


def group_lag(group: str = SETTLEMENT_GROUP) -> int | None:
    """TOTAL-LAG of the group, None when the group does not exist yet."""
    proc = rpk("group", "describe", group, check=False)
    if proc.returncode != 0 or re.search(r"STATE\s+Dead", proc.stdout):
        return None
    m = re.search(r"TOTAL-LAG\s+(\d+)", proc.stdout)
    return int(m.group(1)) if m else None


def seek_group_to_ms(timestamp_ms: int, group: str = SETTLEMENT_GROUP) -> None:
    """Move the (inactive) group back to the first offset at or after `timestamp_ms`."""
    proc = rpk("group", "seek", group, "--to", str(timestamp_ms), check=False)
    assert proc.returncode == 0, (
        f"rpk group seek {group} --to {timestamp_ms} failed (group missing or still active?): "
        f"{(proc.stdout + proc.stderr).strip()[:400]}"
    )


def wait_group_caught_up(group: str = SETTLEMENT_GROUP, timeout: float = 60.0) -> None:
    deadline = time.monotonic() + timeout
    lag: int | None = None
    while time.monotonic() < deadline:
        lag = group_lag(group)
        if lag == 0:
            return
        time.sleep(1)
    raise AssertionError(f"consumer group {group} did not catch up (lag {lag!r})")


# ── Kafka records through confluent-kafka (bytes kept verbatim) ──────────
def _brokers() -> str:
    return get_settings().kafka_brokers


def produce(topic: str, key: bytes, value: bytes, headers: list[tuple[str, bytes]] | None = None):
    from confluent_kafka import Producer

    p = Producer({"bootstrap.servers": _brokers()})
    p.produce(topic, key=key, value=value, headers=headers or None)
    left = p.flush(20)
    assert left == 0, f"{left} record(s) not delivered to {topic}"


def find_records(topic: str, needle: bytes, timeout_s: float = 20.0, settle_s: float = 2.0):
    """Every record on `topic` whose key, value or a header value contains `needle`.

    Scans from the earliest offset; stops `settle_s` after the last match (or at the timeout).
    A topic that does not exist yields an empty list.
    """
    from confluent_kafka import OFFSET_BEGINNING, Consumer, TopicPartition

    c = Consumer(
        {
            "bootstrap.servers": _brokers(),
            "group.id": f"plp-e2e-{uuid.uuid4().hex}",
            "enable.auto.commit": False,
            "auto.offset.reset": "earliest",
        }
    )
    found: list[dict] = []
    try:
        meta = c.list_topics(topic, timeout=10).topics.get(topic)
        if meta is None or meta.error is not None:
            return found
        c.assign([TopicPartition(topic, p, OFFSET_BEGINNING) for p in meta.partitions])
        deadline = time.monotonic() + timeout_s
        last = 0.0
        while time.monotonic() < deadline:
            if found and time.monotonic() - last >= settle_s:
                break
            msg = c.poll(0.5)
            if msg is None or msg.error():
                continue
            hdrs = msg.headers() or []
            hay = [msg.key() or b"", msg.value() or b""] + [v or b"" for _, v in hdrs]
            if any(needle in h for h in hay):
                found.append(
                    {
                        "key": msg.key() or b"",
                        "value": msg.value() or b"",
                        "headers": hdrs,
                        "offset": msg.offset(),
                        "timestamp_ms": msg.timestamp()[1],
                    }
                )
                last = time.monotonic()
    finally:
        c.close()
    return found


# ── throwaway team-payment boot (startup validation scenario) ────────────
def run_payment_with_env(env: dict[str, str], timeout_s: float = 12.0) -> tuple[int | None, str]:
    """Start the team-payment image on the stack network with `env`; (exit code | None, logs).

    The exit code is None when the process was still running after `timeout_s` (it is removed).
    Database and Kafka are off: only configuration loading and validation run.
    """
    name = f"plp-startup-{uuid.uuid4().hex[:8]}"
    args = ["run", "-d", "--name", name, "--network", stack_network()]
    base = {"DATABASE_ENABLED": "false", "KAFKA_ENABLED": "false", "GRPC_PORT": "50999"}
    for k, v in {**base, **env}.items():
        args += ["-e", f"{k}={v}"]
    args.append(image_name("team-payment"))
    docker(*args)
    try:
        deadline = time.monotonic() + timeout_s
        code: int | None = None
        while time.monotonic() < deadline:
            state = docker(
                "inspect", name, "--format", "{{.State.Running}} {{.State.ExitCode}}"
            ).stdout.split()
            if state[0] == "false":
                code = int(state[1])
                break
            time.sleep(0.5)
        logs = docker("logs", name, check=False)
        return code, logs.stdout + logs.stderr
    finally:
        docker("rm", "-f", name, check=False)
