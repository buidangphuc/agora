"""Black-box helpers for change promotion-admin-vouchers (initiative p10-backend-gaps).

Everything goes through the gateway (Connect JSON). Holds are placed and committed with the
service principal over grpcurl (same pattern as `voucher_scope_support`); the only direct SQL is
test fixture shaping (an end date in the past) and reading reservation rows.
"""

from __future__ import annotations

import base64
import json
import os
import subprocess
import time
import uuid
from datetime import datetime, timedelta, timezone
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.support import audit_support as aud
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import voucher_scope_support as vs
from tests.e2e.support.world import World

VOUCHER = "/platform.promotion.v1.VoucherService/"
SERVICE = "platform.promotion.v1.VoucherService"
PLATFORM = "VOUCHER_SCOPE_PLATFORM"
SHOP = "VOUCHER_SCOPE_SHOP"
GENERIC_UNAVAILABLE = "service unavailable"
MONEY_FIELDS = ("scope", "discountType", "discountValue", "minSpend", "maxDiscount")


def state(world: World) -> dict:
    return world.state.extra.setdefault("av", {})


def iso(offset_s: float = 0.0, days: float = 0.0) -> str:
    moment = datetime.now(timezone.utc) + timedelta(seconds=offset_s, days=days)
    return moment.strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


def fresh_code(prefix: str = "AVCH") -> str:
    return f"{prefix}{uuid.uuid4().hex[:8].upper()}"


def gw(token: str | None, method: str, body: dict[str, Any]) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{VOUCHER}{method}"
    return httpx.post(url, json=body, headers=headers, timeout=45)


def gw_path(token: str | None, path: str, body: dict[str, Any]) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{get_settings().gateway_url.rstrip('/')}{path}", json=body, headers=headers, timeout=45
    )


def code_of(resp: httpx.Response) -> str:
    if resp.status_code < 400:
        return "ok"
    try:
        return str(resp.json().get("code", f"http_{resp.status_code}"))
    except ValueError:
        return f"http_{resp.status_code}"


def message_of(resp: httpx.Response) -> str:
    try:
        return str(resp.json().get("message", ""))
    except ValueError:
        return ""


def _decode_error_info(raw: bytes) -> dict[str, str]:
    out: dict[str, str] = {}
    i = 0
    while i < len(raw):
        tag = raw[i]
        i += 1
        field, wire = tag >> 3, tag & 7
        if wire != 2:
            return out
        length = 0
        shift = 0
        while True:
            b = raw[i]
            i += 1
            length |= (b & 0x7F) << shift
            if not b & 0x80:
                break
            shift += 7
        chunk = raw[i : i + length]
        i += length
        if field == 1:
            out["reason"] = chunk.decode()
        elif field == 2:
            out["domain"] = chunk.decode()
    return out


def error_info(resp: httpx.Response) -> dict[str, str]:
    try:
        details = resp.json().get("details") or []
    except ValueError:
        return {}
    for d in details:
        if not str(d.get("type", "")).endswith("google.rpc.ErrorInfo"):
            continue
        debug = d.get("debug") or {}
        if debug.get("reason"):
            return {"reason": str(debug["reason"]), "domain": str(debug.get("domain", ""))}
        raw = d.get("value", "")
        raw += "=" * (-len(raw) % 4)
        return _decode_error_info(base64.b64decode(raw))
    return {}


def voucher_of(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    voucher = resp.json().get("voucher") or {}
    assert voucher.get("id"), f"no voucher in response: {resp.text}"
    return voucher


def create_body(
    code: str,
    *,
    scope: str = PLATFORM,
    quota: int = 100,
    starts_at: str | None = None,
    ends_at: str | None = None,
    discount_value: int = 10,
    min_spend: int = 0,
    max_discount: int = 50000,
) -> dict[str, Any]:
    return {
        "code": code,
        "scope": scope,
        "discountType": "DISCOUNT_TYPE_PERCENT",
        "discountValue": str(discount_value),
        "minSpend": str(min_spend),
        "maxDiscount": str(max_discount),
        "quota": str(quota),
        "startsAt": starts_at or iso(days=-1),
        "endsAt": ends_at or iso(days=30),
    }


def create_voucher(token: str, code: str, **kw: Any) -> dict[str, Any]:
    return voucher_of(gw(token, "CreateVoucher", create_body(code, **kw)))


def get_voucher(token: str, code: str) -> dict[str, Any] | None:
    resp = gw(token, "GetVoucher", {"code": code})
    if resp.status_code == 404 or code_of(resp) == "not_found":
        return None
    assert resp.status_code == 200, f"GetVoucher {code}: {resp.status_code} {resp.text}"
    return resp.json().get("voucher") or None


def deactivate(token: str | None, voucher_id: str) -> httpx.Response:
    return gw(token, "DeactivateVoucher", {"voucherId": voucher_id})


def update(
    token: str | None, voucher_id: str, *, quota: int | None = None, ends_at: str | None = None
) -> httpx.Response:
    body: dict[str, Any] = {"voucherId": voucher_id}
    if quota is not None:
        body["quota"] = str(quota)
    if ends_at is not None:
        body["endsAt"] = ends_at
    return gw(token, "UpdateVoucher", body)


def reserve(
    code: str, reservation_id: str | None = None, subtotal: int = 1_000_000
) -> tuple[str, str, Any]:
    rid = reservation_id or vs.fresh_reservation("avch")
    status, body = vs.validate_and_reserve(code, rid, subtotal)
    return rid, status, body


def grpc_voucher(method: str, body: dict[str, Any]) -> tuple[str, Any]:
    pdir = fh.proto_dir()
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        fh.grpcurl_image(), "-plaintext", "-import-path", "/proto", "-proto", fh.PROTO_FILE,
        "-H", f"x-principal-id: {vs.ORDER_PRINCIPAL}", "-H", "x-principal-type: service",
        "-H", f"x-principal-scopes: {vs.RESERVE_SCOPE}",
        "-d", json.dumps(body), fh.promotion_addr(), f"{SERVICE}/{method}",
    ]  # fmt: skip
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    return f"grpcurl_exit_{proc.returncode}", (proc.stderr + proc.stdout).strip()


def commit(reservation_id: str) -> tuple[str, Any]:
    return grpc_voucher("CommitReservation", {"reservationId": reservation_id})


def release(reservation_id: str) -> tuple[str, Any]:
    return grpc_voucher("ReleaseReservation", {"reservationId": reservation_id})


def redeem(code: str, times: int) -> None:
    for _ in range(times):
        rid, status, body = reserve(code)
        assert status == "ok" and body.get("valid"), f"seed reservation refused: {status} {body}"
        cstatus, cbody = commit(rid)
        assert cstatus == "ok" and cbody.get("committed"), f"seed commit failed: {cstatus} {cbody}"


def backdate_end(code: str) -> None:
    fh.psql(
        "UPDATE vouchers SET starts_at = now() - interval '3 days', ends_at = now() - interval '1 hour' "
        f"WHERE code = {fh.sql_lit(code)}"
    )


def audit_count(world: World, code: str, action: str) -> int:
    return len(
        [
            e
            for e in aud.events_for(world, "voucher", code, action=action)
            if e.get("action") == action
        ]
    )


def audit_mentions(world: World, action: str, needle: str, since: str) -> list[dict]:
    token = aud.admin_token(world)
    found: list[dict] = []
    cursor: str | None = None
    for _ in range(50):
        resp = aud.query_events(token, action=action, created_after=since, cursor=cursor)
        assert resp.status_code == 200, resp.text
        data = resp.json()
        found.extend(e for e in data.get("events", []) if needle in json.dumps(e))
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            break
    return found


def promotion_container() -> str:
    return os.getenv("PLP_TEAM_PROMOTION_CONTAINER", "agora-team-promotion-svc")


def _docker(*args: str) -> None:
    proc = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=120)
    assert proc.returncode == 0, f"docker {' '.join(args)} failed: {proc.stderr}"


def _wait_promotion_up(token: str, code: str) -> None:
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            if gw(token, "GetVoucher", {"code": code}).status_code in (200, 404):
                time.sleep(aud.RECONNECT_GRACE_S)
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("team-promotion did not recover after the stop")


def stop_promotion(world: World, token: str) -> None:
    st = state(world)
    if st.get("promotion_stopped"):
        return
    _docker("stop", promotion_container())
    st["promotion_stopped"] = True

    def restore() -> None:
        if st.get("promotion_stopped"):
            _docker("start", promotion_container())
            st["promotion_stopped"] = False
        _wait_promotion_up(token, "AVCH-PROBE")

    world.add_cleanup(restore)


def start_promotion(world: World, token: str) -> None:
    st = state(world)
    if st.get("promotion_stopped"):
        _docker("start", promotion_container())
        st["promotion_stopped"] = False
    _wait_promotion_up(token, "AVCH-PROBE")
