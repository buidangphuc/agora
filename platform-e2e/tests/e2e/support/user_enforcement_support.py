"""Shared e2e helper for user enforcement (slice identity-user-enforcement, initiative q6-trust-enforcement).

Black box through the gateway: an admin sets a user's status with AccountAdminService, and the
helper observes the effects (login refusal, token rejection, SSE end, audit rows, the
`UserStatusChanged` event on `identity.events`). Only two things go around the gateway, both
test-environment setup that has no RPC: promoting a registered user to role `admin` (SQL on
identity_db) and reading the Kafka topic.

Env: POSTGRES_CONTAINER (default agora-postgres-1), KAFKA_BROKERS (default localhost:19092).
Other slices import this module and never edit it.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import threading
import time
from datetime import datetime, timedelta, timezone

import httpx

from config.settings import get_settings
from src.utils import data as fake
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

IDENTITY = "/platform.identity.v1"
LOGIN = f"{IDENTITY}.AuthService/Login"
REGISTER = f"{IDENTITY}.AuthService/Register"
REFRESH = f"{IDENTITY}.AuthService/RefreshSession"
SESSIONS = f"{IDENTITY}.SessionService/ListSessions"
SET_STATUS = f"{IDENTITY}.AccountAdminService/SetUserStatus"
LIST_USERS = f"{IDENTITY}.AccountAdminService/ListUsers"

LOCKED_MESSAGE = "tài khoản bị khóa"
REVOKE_WITHIN_S = 60

_STATUS = {
    "active": "USER_STATUS_ACTIVE",
    "restricted": "USER_STATUS_RESTRICTED",
    "suspended": "USER_STATUS_SUSPENDED",
    "banned": "USER_STATUS_BANNED",
}


def _gw(path: str) -> str:
    return f"{get_settings().gateway_url.rstrip('/')}{path}"


def call(path: str, body: dict | None = None, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(_gw(path), json=body or {}, headers=headers, timeout=20)


def body_of(resp: httpx.Response) -> dict:
    """The response message; the gateway may wrap it in `result`."""
    data = resp.json()
    return data.get("result", data) if isinstance(data, dict) else {}


def code_of(resp: httpx.Response) -> str:
    """Connect error code of a failed call ('' when the body is not an error)."""
    try:
        return str(resp.json().get("code", ""))
    except ValueError:
        return ""


def claims(token: str) -> dict:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))


def user_id_of(token: str) -> str:
    return claims(token)["sub"]


# ── accounts ─────────────────────────────────────────────────────────────────
def register_user(role: str = "buyer", prefix: str = "ue") -> dict:
    """Register a fresh user and log in once. Returns username, password, id, token, refresh."""
    username = fake.unique_username(prefix)
    password = get_settings().seed_password
    resp = call(REGISTER, {"username": username, "password": password, "role": role})
    assert resp.status_code == 200, f"Register({role}) failed: {resp.status_code} {resp.text}"
    user = {"username": username, "password": password, "role": role}
    user.update(login(user))
    return user


def register_buyer(world: World | None = None) -> dict:
    return register_user("buyer", "ue_buyer")


def login(user: dict, password: str | None = None) -> dict:
    """Successful login; returns id, token, refresh. Raises AssertionError otherwise."""
    resp = login_raw(user["username"], password or user["password"])
    assert resp.status_code == 200, f"Login failed: {resp.status_code} {resp.text}"
    res = body_of(resp)
    return {"id": user_id_of(res["token"]), "token": res["token"], "refresh": res.get("refreshToken", "")}


def login_raw(username: str, password: str) -> httpx.Response:
    return call(LOGIN, {"username": username, "password": password})


def _psql(sql: str) -> None:
    container = os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")
    proc = subprocess.run(
        ["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", "identity_db",
         "-v", "ON_ERROR_STOP=1", "-q"],
        input=sql, capture_output=True, text=True, timeout=60,
    )
    assert proc.returncode == 0, f"identity_db update failed: {proc.stderr}"


def register_second_admin() -> dict:
    """A registered user promoted to role admin (no RPC creates a second admin)."""
    user = register_user("buyer", "ue_admin2")
    _psql(f"UPDATE users SET roles = ARRAY['admin'] WHERE id = '{user['id']}';")
    user.update(login(user))
    assert "admin" in json.dumps(claims(user["token"])), "promoted user token carries no admin"
    return user


def admin_token(world: World) -> str:
    return aud.admin_token(world)


def admin_id(world: World) -> str:
    return user_id_of(admin_token(world))


def active_session_count(token: str) -> int:
    resp = call(SESSIONS, {}, token)
    assert resp.status_code == 200, f"ListSessions failed: {resp.status_code} {resp.text}"
    sessions = body_of(resp).get("sessions", [])
    return len([s for s in sessions if not s.get("revokedAt")])


# ── admin calls ──────────────────────────────────────────────────────────────
def set_user_status(
    world: World | None,
    user_id: str,
    status: str,
    reason: str = "e2e_reason",
    until: datetime | None = None,
    note: str = "",
    token: str | None = None,
) -> httpx.Response:
    """SetUserStatus through the gateway; `status` is active|restricted|suspended|banned.
    Uses the seeded admin unless `token` is given."""
    token = token if token is not None else admin_token(world)
    body: dict = {"userId": user_id, "status": _STATUS[status], "reasonCode": reason}
    if note:
        body["note"] = note
    if until is not None:
        body["until"] = until.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return call(SET_STATUS, body, token)


def in_future(seconds: int) -> datetime:
    return datetime.now(timezone.utc) + timedelta(seconds=seconds)


def list_users(token: str | None, query: str = "") -> httpx.Response:
    return call(LIST_USERS, {"query": query} if query else {}, token)


def stored_user(world: World, user_id: str) -> dict:
    """The AdminUser row as the admin sees it (ListUsers by exact id)."""
    resp = list_users(admin_token(world), user_id)
    assert resp.status_code == 200, f"ListUsers failed: {resp.status_code} {resp.text}"
    users = body_of(resp).get("users", [])
    assert len(users) == 1, f"expected exactly 1 user for id {user_id}, got {users}"
    return users[0]


def wait_until(predicate, timeout_s: float, interval_s: float = 1.0):
    """Poll `predicate` (returns a truthy value) until the deadline; returns the value or None."""
    deadline = time.monotonic() + timeout_s
    while True:
        value = predicate()
        if value:
            return value
        if time.monotonic() >= deadline:
            return None
        time.sleep(interval_s)


# ── SSE ──────────────────────────────────────────────────────────────────────
class Stream:
    """One open `/api/events/live` connection read in a thread; records when it closed."""

    def __init__(self, token: str, room: str, limit_s: float = 90.0) -> None:
        self.lines: list[str] = []
        self.status = 0
        self.closed = threading.Event()
        self._opened = threading.Event()
        url = _gw(f"/api/events/live?room={room}")
        self._thread = threading.Thread(
            target=self._run, args=(url, {"Authorization": f"bearer {token}"}, limit_s), daemon=True
        )
        self._thread.start()
        assert self._opened.wait(15), "the SSE connection never opened"
        assert self.status == 200, f"SSE open answered {self.status}"

    def _run(self, url: str, headers: dict, limit_s: float) -> None:
        try:
            with httpx.stream("GET", url, headers=headers, timeout=limit_s) as resp:
                self.status = resp.status_code
                self._opened.set()
                for line in resp.iter_lines():
                    self.lines.append(line)
        except Exception:  # noqa: BLE001 - a transport error also ends the stream
            pass
        finally:
            self._opened.set()
            self.closed.set()

    def wait_closed(self, timeout_s: float) -> bool:
        return self.closed.wait(timeout_s)


# ── Kafka: UserStatusChanged on identity.events ──────────────────────────────
def _fields(buf: bytes) -> dict[int, list]:
    out: dict[int, list] = {}
    i = 0
    while i < len(buf):
        key, i = _varint(buf, i)
        field, wire = key >> 3, key & 7
        if wire == 0:
            val, i = _varint(buf, i)
        elif wire == 2:
            n, i = _varint(buf, i)
            val, i = buf[i : i + n], i + n
        else:
            raise ValueError(f"unsupported wire type {wire}")
        out.setdefault(field, []).append(val)
    return out


def _varint(buf: bytes, i: int) -> tuple[int, int]:
    shift = result = 0
    while True:
        b = buf[i]
        i += 1
        result |= (b & 0x7F) << shift
        if not b & 0x80:
            return result, i
        shift += 7


class StatusEventTap:
    """Reads `identity.events` from the moment it is created (end offsets)."""

    TOPIC = "identity.events"
    TYPE = "platform.identity.v1.UserStatusChanged"

    def __init__(self) -> None:
        from confluent_kafka import Consumer, TopicPartition

        brokers = os.getenv("KAFKA_BROKERS", "localhost:19092")
        self._consumer = Consumer(
            {"bootstrap.servers": brokers, "group.id": f"e2e-ue-{time.time_ns()}",
             "enable.auto.commit": False}
        )
        meta = self._consumer.list_topics(self.TOPIC, timeout=15)
        parts = list(meta.topics[self.TOPIC].partitions)
        assigns = []
        for p in parts:
            _, high = self._consumer.get_watermark_offsets(TopicPartition(self.TOPIC, p), timeout=15)
            assigns.append(TopicPartition(self.TOPIC, p, high))
        self._consumer.assign(assigns)
        self.seen: list[dict] = []

    def status_events(self, user_id: str, wait_s: float = 15.0) -> list[dict]:
        """Decoded UserStatusChanged events for the user seen so far plus those arriving within wait_s."""
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            msg = self._consumer.poll(1.0)
            if msg is None or msg.error():
                continue
            if (msg.key() or b"").decode(errors="ignore") != user_id:
                continue
            env = _fields(msg.value())
            if env.get(2, [b""])[0].decode() != self.TYPE:
                continue
            payload = _fields(env[7][0])
            self.seen.append(
                {
                    "user_id": payload.get(1, [b""])[0].decode(),
                    "status": payload.get(2, [0])[0],
                    "previous_status": payload.get(3, [0])[0],
                    "reason_code": payload.get(4, [b""])[0].decode(),
                }
            )
        return [e for e in self.seen if e["user_id"] == user_id]

    def close(self) -> None:
        self._consumer.close()


STATUS_ENUM = {"active": 1, "restricted": 2, "suspended": 3, "banned": 4}
