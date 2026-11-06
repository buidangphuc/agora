from __future__ import annotations

import time
import uuid
from datetime import datetime
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import plp_stack as stack

VOUCHER = "/platform.promotion.v1.VoucherService"
PROMOTION_SERVICE = "team-promotion"
REFERRAL_SERVICE = "team-referral"
OVERLAY = "platform-e2e/compose/new-user-voucher.override.yaml"
KIND_NEW_USER = "VOUCHER_KIND_NEW_USER"
POLL_INTERVAL_S = 0.5
ISSUE_DEADLINE_S = 10.0
QUIET_S = 4.0
WELCOME_AMOUNT = 20000
INVITED_AMOUNT = 30000
MIN_SPEND = 100000
TTL_S = 336 * 3600


def register(oic: s.OicWorld, name: str, code: str | None = None) -> s.Actor:
    body: dict[str, Any] = {
        "username": f"nuv_{uuid.uuid4().hex[:10]}",
        "password": get_settings().seed_password,
        "role": "buyer",
    }
    if code is not None:
        body["referralCode"] = code
    resp = s.post(oic, None, s.AUTH, "Register", body)
    token = (s.ok(resp).get("result") or {}).get("token", "")
    assert token, f"register {name}: {resp.text[:300]}"
    actor = s.Actor(body["username"], token, "buyer", s._sub(token))
    oic.actors[name] = actor
    oic.data.setdefault("nuv_registered_at", {})[name] = time.monotonic()
    return actor


def list_my_response(oic: s.OicWorld, actor: s.Actor | None) -> httpx.Response:
    return s.post(oic, actor, VOUCHER, "ListMyVouchers", {})


def my_vouchers(oic: s.OicWorld, actor: s.Actor) -> list[dict[str, Any]]:
    return s.ok(list_my_response(oic, actor)).get("vouchers", [])


def wait_for_vouchers(
    oic: s.OicWorld, actor: s.Actor, deadline_s: float = ISSUE_DEADLINE_S
) -> list[dict[str, Any]]:
    deadline = time.monotonic() + deadline_s
    found = my_vouchers(oic, actor)
    while not found and time.monotonic() < deadline:
        time.sleep(POLL_INTERVAL_S)
        found = my_vouchers(oic, actor)
    assert found, f"no voucher listed for {actor.username} within {deadline_s} s"
    return found


def wait_until_gone(
    oic: s.OicWorld, actor: s.Actor, code: str, deadline_s: float = ISSUE_DEADLINE_S
) -> list[dict[str, Any]]:
    deadline = time.monotonic() + deadline_s
    found = my_vouchers(oic, actor)
    while any(v.get("code") == code for v in found) and time.monotonic() < deadline:
        time.sleep(POLL_INTERVAL_S)
        found = my_vouchers(oic, actor)
    return found


def num(voucher: dict[str, Any], key: str) -> int:
    return int(voucher.get(key) or 0)


def ts(value: str | None) -> float:
    assert value, "missing timestamp"
    return datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()


def assert_new_user_voucher(voucher: dict[str, Any], amount: int) -> None:
    assert voucher.get("kind") == KIND_NEW_USER, voucher
    assert num(voucher, "discountValue") == amount, voucher
    assert num(voucher, "minSpend") == MIN_SPEND, voucher
    assert num(voucher, "quota") == 1, voucher
    assert num(voucher, "perUserLimit") == 1, voucher
    lifetime = ts(voucher.get("endsAt")) - ts(voucher.get("startsAt"))
    assert abs(lifetime - TTL_S) <= 1.0, (lifetime, voucher)


def stopped_referral_guard(oic: s.OicWorld) -> None:
    if oic.data.get("nuv_referral_stopped"):
        stack.docker("start", stack.container_name(REFERRAL_SERVICE), check=False)
        oic.data["nuv_referral_stopped"] = False


def require_fast_verdict() -> tuple[float, float]:
    env = stack.container_env(PROMOTION_SERVICE)
    timeout = stack.parse_go_duration(env.get("NEW_USER_VERDICT_TIMEOUT", "").strip() or "30m")
    sweep = stack.parse_go_duration(env.get("NEW_USER_VERDICT_SWEEP_INTERVAL", "").strip() or "1m")
    assert timeout is not None and sweep is not None
    assert timeout <= 2.0 and sweep <= 1.0, (
        f"the new-user overlay is not active (verdict timeout {timeout}s, sweep {sweep}s); "
        f"start the stack with `-f {OVERLAY}`"
    )
    return timeout, sweep
