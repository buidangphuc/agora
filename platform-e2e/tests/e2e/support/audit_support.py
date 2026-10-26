"""Shared e2e helper for the audit trail (slice audit-query-filters, initiative q3-audit-log).

Black box through the gateway as the seeded admin. Events are seeded by direct SQL into
``audit_db`` (team-audit has no host port and the e2e venv has no gRPC client); that is the
only seeding path and it lets a test choose ``created_at``. Events written by a producer
(the thing under test elsewhere) always arrive through the producer RPC, never ``seed_events``.

Env: AUDIT_CONTAINER (default agora-team-audit-svc), AUDIT_POSTGRES_CONTAINER (default
agora-postgres-1). Other slices import this module and never edit it.
"""

from __future__ import annotations

import json
import os
import subprocess
import time
import uuid

import httpx

from config.settings import get_settings
from tests.e2e.support.world import World

AUDIT = "/platform.audit.v1.AuditService"
_QUERY = f"{AUDIT}/QueryAuditLog"
_RECOVERY_S = 60
RECONNECT_GRACE_S = float(os.environ.get("E2E_AUDIT_RECONNECT_GRACE_S", "15"))


class AuditUnavailable(AssertionError):
    """The gateway answered 5xx (or the transport failed) on QueryAuditLog."""


def _audit_container() -> str:
    return os.getenv("AUDIT_CONTAINER", "agora-team-audit-svc")


def _pg_container() -> str:
    return os.getenv("AUDIT_POSTGRES_CONTAINER", "agora-postgres-1")


def fresh_target(prefix: str = "e2e-aud") -> tuple[str, str]:
    """(target_type, target_id), both unique per call."""
    suffix = uuid.uuid4().hex[:10]
    return f"{prefix}-type-{suffix}", f"{prefix}-id-{suffix}"


def admin_token(world: World) -> str:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    return world.service_factory.auth.login(admin.username, admin.password)


def query_events(
    token: str | None,
    *,
    target_type: str | None = None,
    target_id: str | None = None,
    actor_id: str | None = None,
    action: str | None = None,
    created_after: str | None = None,
    created_before: str | None = None,
    page_size: int = 100,
    cursor: str | None = None,
) -> httpx.Response:
    """Raw QueryAuditLog through the gateway (Connect JSON). Only 5xx / transport errors raise
    AuditUnavailable; 4xx responses are returned for the caller to assert."""
    body: dict = {"page": {"pageSize": page_size}}
    if cursor:
        body["page"]["cursor"] = cursor
    for key, val in (
        ("targetType", target_type),
        ("targetId", target_id),
        ("actorId", actor_id),
        ("action", action),
        ("createdAfter", created_after),
        ("createdBefore", created_before),
    ):
        if val is not None:
            body[key] = val
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{_QUERY}"
    try:
        resp = httpx.post(url, json=body, headers=headers, timeout=20)
    except httpx.HTTPError as exc:
        raise AuditUnavailable(f"QueryAuditLog transport error: {exc}") from exc
    if resp.status_code >= 500:
        raise AuditUnavailable(f"QueryAuditLog answered {resp.status_code}: {resp.text}")
    return resp


def _checked(resp: httpx.Response) -> dict:
    if resp.status_code >= 500:
        raise AuditUnavailable(f"QueryAuditLog answered {resp.status_code}: {resp.text}")
    assert resp.status_code == 200, f"QueryAuditLog failed: {resp.status_code} {resp.text}"
    return resp.json()


def events_for(
    world: World,
    target_type: str,
    target_id: str | None = None,
    *,
    action: str | None = None,
    actor_id: str | None = None,
    token: str | None = None,
) -> list[dict]:
    """All matching events newest first, following nextCursor."""
    token = token or admin_token(world)
    out: list[dict] = []
    cursor: str | None = None
    for _ in range(1000):
        data = _checked(
            query_events(
                token,
                target_type=target_type,
                target_id=target_id,
                action=action,
                actor_id=actor_id,
                cursor=cursor,
            )
        )
        out.extend(data.get("events", []))
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            return out
    raise AssertionError("QueryAuditLog paging did not terminate")


def assert_one_event(
    world: World,
    *,
    target_type: str,
    target_id: str,
    action: str,
    actor_id: str,
    metadata: dict[str, str] | None = None,
) -> dict:
    """Exactly one event of `action` for the target, actor equal, every `metadata` key equal."""
    found = events_for(world, target_type, target_id, action=action)
    found = [e for e in found if e.get("action") == action]
    assert len(found) == 1, f"expected exactly 1 '{action}' event for {target_type}/{target_id}, got {found}"
    event = found[0]
    assert event.get("actorId") == actor_id, f"actor mismatch: {event}"
    got = event.get("metadata") or {}
    for key, want in (metadata or {}).items():
        assert got.get(key) == want, f"metadata[{key}]={got.get(key)!r}, want {want!r}: {event}"
    return event


def assert_no_event(
    world: World, *, target_type: str, target_id: str, action: str | None = None
) -> None:
    found = events_for(world, target_type, target_id, action=action)
    if action:
        found = [e for e in found if e.get("action") == action]
    assert not found, f"expected no event for {target_type}/{target_id} (action={action}), got {found}"


def _sql_lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def seed_events(rows: list[dict]) -> list[str]:
    """Insert rows into audit_db.audit_events; returns the generated ids."""
    ids: list[str] = []
    stmts: list[str] = []
    for row in rows:
        eid = f"e2e-aud-{uuid.uuid4().hex[:8]}"
        ids.append(eid)
        created = _sql_lit(row["created_at"]) + "::timestamptz" if row.get("created_at") else "NOW()"
        meta = json.dumps(row.get("metadata") or {})
        stmts.append(
            "INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata, created_at) "
            f"VALUES ({_sql_lit(eid)}, {_sql_lit(row.get('actor_id', 'e2e-actor'))}, "
            f"{_sql_lit(row['action'])}, {_sql_lit(row['target_type'])}, {_sql_lit(row['target_id'])}, "
            f"{_sql_lit(meta)}::jsonb, {created});"
        )
    if not stmts:
        return ids
    proc = subprocess.run(
        ["docker", "exec", "-i", _pg_container(), "psql", "-U", "postgres", "-d", "audit_db",
         "-v", "ON_ERROR_STOP=1", "-q"],
        input="\n".join(stmts),
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 0, f"seeding audit_db failed: {proc.stderr}"
    return ids


def _docker(*args: str) -> None:
    proc = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=120)
    assert proc.returncode == 0, f"docker {' '.join(args)} failed: {proc.stderr}"


def _wait_until_up(world: World) -> None:
    from tests.e2e.flows import wait_for_gateway

    wait_for_gateway(get_settings().gateway_url)
    token = admin_token(world)
    deadline = time.monotonic() + _RECOVERY_S
    while time.monotonic() < deadline:
        try:
            if query_events(token, page_size=1).status_code == 200:
                time.sleep(RECONNECT_GRACE_S)
                return
        except AuditUnavailable:
            pass
        time.sleep(1)
    raise TimeoutError("team-audit did not recover after the stop")


def stop_audit(world: World) -> None:
    """docker stop team-audit; a world cleanup starts it again and waits for a 200. Idempotent."""
    state = world.state.extra
    if state.get("audit_stopped"):
        return
    _docker("stop", _audit_container())
    state["audit_stopped"] = True

    def restore() -> None:
        if state.get("audit_stopped"):
            _docker("start", _audit_container())
            state["audit_stopped"] = False
        _wait_until_up(world)

    world.add_cleanup(restore)


def start_audit(world: World) -> None:
    """Start it early (the cleanup is then a no-op start) and wait until QueryAuditLog answers 200."""
    state = world.state.extra
    if state.get("audit_stopped"):
        _docker("start", _audit_container())
        state["audit_stopped"] = False
    _wait_until_up(world)
