"""Helpers for change identity-account-deletion (initiative trust-legal).

Black box through the gateway (Connect JSON). Two things go around the edge, both test-environment
access that has no RPC: a read-only SELECT on identity_db (rows really gone, proof rows kept) and
the `identity.events` topic read for `UserErased`. The service-principal call is a grpcurl call to
team-identity on the stack network.

Env: POSTGRES_CONTAINER (default agora-postgres-1), KAFKA_BROKERS (default localhost:19092),
IDENTITY_CONTAINER (default team-identity-svc), IDENTITY_ADDR (default team-identity-svc:50053),
STACK_NETWORK.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import time
from datetime import datetime, timezone
from pathlib import Path

import httpx

from tests.e2e.support import plp_stack as stack
from tests.e2e.support import user_enforcement_support as ue

PRIVACY = "/platform.identity.v1.PrivacyService"
REQUEST = f"{PRIVACY}/RequestAccountDeletion"
CANCEL = f"{PRIVACY}/CancelAccountDeletion"
STATUS = f"{PRIVACY}/GetAccountDeletionStatus"
ORDER_OBLIGATIONS = "/platform.order.v1.OrderService/GetUserObligations"
PAYMENT_OBLIGATIONS = "/platform.payment.v1.PaymentService/GetPayoutObligations"
REFRESH = ue.REFRESH

DEFAULT_WINDOW_S = 48 * 3600.0
OVERRIDE_WINDOW_S = 20.0
OVERRIDE_SWEEP_S = 2.0
TIMESTAMP_SKEW_S = 5.0

CATEGORY_WORDS = {
    "open orders": r"order|đơn hàng|don hang",
    "open returns": r"return|trả hàng|tra hang",
    "wallet balance": r"wallet|balance|số dư|so du|ví",
    "pending payouts": r"payout|rút tiền|rut tien",
}


def identity_container() -> str:
    return os.getenv("IDENTITY_CONTAINER", "team-identity-svc")


def identity_env() -> dict[str, str]:
    out = subprocess.run(
        ["docker", "inspect", "-f", "{{json .Config.Env}}", identity_container()],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert out.returncode == 0, f"cannot inspect {identity_container()}: {out.stderr.strip()[:300]}"
    env: dict[str, str] = {}
    for item in json.loads(out.stdout) or []:
        name, _, value = item.partition("=")
        env[name] = value
    return env


def configured_window_s() -> float | None:
    raw = identity_env().get("ACCOUNT_DELETION_CANCEL_WINDOW", "").strip()
    return stack.parse_go_duration(raw) if raw else None


def configured_sweep_s() -> float | None:
    raw = identity_env().get("ACCOUNT_ERASURE_SWEEP_INTERVAL", "").strip()
    return stack.parse_go_duration(raw) if raw else None


# ── calls ────────────────────────────────────────────────────────────────────
def request_deletion(token: str | None, password: str) -> httpx.Response:
    return ue.call(REQUEST, {"password": password}, token)


def cancel_deletion(token: str | None) -> httpx.Response:
    return ue.call(CANCEL, {}, token)


def status_of(token: str | None) -> httpx.Response:
    return ue.call(STATUS, {}, token)


def status_body(token: str) -> dict:
    resp = status_of(token)
    assert (
        resp.status_code == 200
    ), f"GetAccountDeletionStatus {resp.status_code}: {resp.text[:300]}"
    return ue.body_of(resp)


def is_pending(token: str) -> bool:
    return bool(status_body(token).get("pending", False))


def erase_after_of(body: dict) -> float | None:
    value = body.get("eraseAfter") or body.get("erase_after")
    return parse_ts(value) if value else None


def parse_ts(value: str) -> float:
    base, _, rest = value.rstrip("Z").partition(".")
    frac = rest[:6].ljust(6, "0") if rest else "000000"
    dt = datetime.strptime(f"{base}.{frac}", "%Y-%m-%dT%H:%M:%S.%f").replace(tzinfo=timezone.utc)
    return dt.timestamp()


def code(resp: httpx.Response) -> str:
    """Connect error code in lower snake case ('' for a success)."""
    return ue.code_of(resp).lower() if resp.status_code >= 400 else ""


def message_of(resp: httpx.Response) -> str:
    try:
        return str(resp.json().get("message", ""))
    except ValueError:
        return resp.text


def names_category(message: str, category: str) -> bool:
    return re.search(CATEGORY_WORDS[category], message, re.I) is not None


def sessions_active(token: str) -> int:
    return ue.active_session_count(token)


def refresh_with(refresh_token: str) -> httpx.Response:
    return ue.call(REFRESH, {"refreshToken": refresh_token})


# ── service principal, direct on team-identity ───────────────────────────────
def service_principal_request(user_id: str, password: str) -> tuple[str, str]:
    """RequestAccountDeletion straight on team-identity as a service principal: (code, text)."""
    root = Path(__file__).resolve().parents[4]
    pdir = root / "team-identity" / "proto"
    assert (pdir / "platform" / "identity" / "v1" / "identity.proto").is_file(), pdir
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        os.getenv("ERASURE_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest"),
        "-plaintext", "-emit-defaults", "-import-path", "/proto",
        "-proto", "platform/identity/v1/identity.proto",
        "-H", f"x-principal-id: {user_id}", "-H", "x-principal-type: service",
        "-d", json.dumps({"password": password}),
        os.getenv("IDENTITY_ADDR", "team-identity-svc:50053"),
        "platform.identity.v1.PrivacyService/RequestAccountDeletion",
    ]  # fmt: skip
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", proc.stdout
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    snake = re.sub(r"(?<!^)(?=[A-Z])", "_", m.group(1)).lower() if m else "grpcurl_error"
    return snake, err


# ── identity_db (read-only) ──────────────────────────────────────────────────
def psql(sql: str) -> str:
    res = subprocess.run(
        [
            "docker", "exec", os.getenv("POSTGRES_CONTAINER", "agora-postgres-1"),
            "psql", "-U", "postgres", "-d", "identity_db", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )  # fmt: skip
    assert res.returncode == 0, f"psql failed: {res.stderr.strip()[:400]}"
    return res.stdout.strip()


def lit(value: str) -> str:
    assert value.replace("-", "").replace("_", "").isalnum(), f"unexpected id {value!r}"
    return "'" + value + "'"


def user_row(user_id: str) -> dict[str, str]:
    raw = psql(
        "SELECT username, status, password_hash, array_to_string(roles, ',') "
        f"FROM users WHERE id = {lit(user_id)};"
    )
    assert raw, f"no users row for {user_id}"
    username, status, password_hash, roles = (raw.split("|") + ["", "", "", ""])[:4]
    return {"username": username, "status": status, "password_hash": password_hash, "roles": roles}


ERASED_TABLE_PATTERNS = {
    "addresses": r"address",
    "sessions": r"^sessions$",
    "login history": r"login",
    "handoff codes": r"handoff",
    "reset tokens": r"reset",
}


def erased_table_counts(user_id: str) -> dict[str, int]:
    """Row count per erased category for the user (tables found by name pattern with a user_id column)."""
    names = psql(
        "SELECT table_name FROM information_schema.columns "
        "WHERE table_schema = 'public' AND column_name = 'user_id' ORDER BY table_name;"
    ).splitlines()
    counts: dict[str, int] = {}
    for label, pattern in ERASED_TABLE_PATTERNS.items():
        tables = [n for n in names if re.search(pattern, n)]
        assert tables, f"no identity_db table for '{label}' (pattern {pattern}) among {names}"
        counts[label] = sum(
            int(psql(f"SELECT count(*) FROM {t} WHERE user_id = {lit(user_id)};")) for t in tables
        )
    return counts


def consent_count(user_id: str) -> int:
    return int(psql(f"SELECT count(*) FROM consents WHERE user_id = {lit(user_id)};"))


# ── identity.events: UserErased ──────────────────────────────────────────────
class ErasedTap:
    """Reads `identity.events` for `UserErased` from the moment it is created (end offsets)."""

    TOPIC = "identity.events"
    TYPE = "platform.identity.v1.UserErased"

    def __init__(self) -> None:
        from confluent_kafka import Consumer, TopicPartition

        self._consumer = Consumer(
            {
                "bootstrap.servers": os.getenv("KAFKA_BROKERS", "localhost:19092"),
                "group.id": f"e2e-del-{time.time_ns()}",
                "enable.auto.commit": False,
            }
        )
        meta = self._consumer.list_topics(self.TOPIC, timeout=15)
        assigns = []
        for p in list(meta.topics[self.TOPIC].partitions):
            _, high = self._consumer.get_watermark_offsets(
                TopicPartition(self.TOPIC, p), timeout=15
            )
            assigns.append(TopicPartition(self.TOPIC, p, high))
        self._consumer.assign(assigns)
        self.seen: list[dict] = []

    def poll(self, wait_s: float) -> None:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            msg = self._consumer.poll(1.0)
            if msg is None or msg.error():
                continue
            env = ue._fields(msg.value())
            if env.get(2, [b""])[0].decode() != self.TYPE:
                continue
            payload = ue._fields(env[7][0])
            self.seen.append(
                {
                    "key": (msg.key() or b"").decode(errors="ignore"),
                    "user_id": payload.get(1, [b""])[0].decode(),
                    "roles": [r.decode() for r in payload.get(2, [])],
                    "has_erased_at": 3 in payload,
                }
            )

    def events_for(self, user_id: str) -> list[dict]:
        return [e for e in self.seen if e["user_id"] == user_id]

    def wait_for(self, user_id: str, wait_s: float) -> list[dict]:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            self.poll(1.0)
            if self.events_for(user_id):
                break
        return self.events_for(user_id)

    def close(self) -> None:
        self._consumer.close()
