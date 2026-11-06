"""Steps for frontend/referral_invite.feature (change: frontend-referral-invite).

Black-box: /s/<shortCode> is requested over HTTP without following redirects, register and redeem
are driven in the browser, and attribution / reward state is read back through the gateway reads
the referral page itself renders (GetMyReferral, ListReferralRewards).
"""

from __future__ import annotations

import os
import re
import subprocess
import time
import uuid
from collections.abc import Callable
from datetime import datetime
from typing import Any
from urllib.parse import parse_qs, quote, urlsplit
from zoneinfo import ZoneInfo

import httpx
from playwright.sync_api import Error as PlaywrightError
from playwright.sync_api import Locator, Response, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import (
    AddressService,
    AuthService,
    CartService,
    OrderService,
    PaymentService,
)
from src.api.services.base_service import BaseService, GatewayError
from src.api.services.referral_service import ReferralService
from src.api.services.sharing_service import SharingService
from src.constants import gateway_endpoints as ep
from src.constants import timeouts
from src.core.base_page import goto_streamed
from src.models import User
from src.pages.register_page import RegisterPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support import d2_support as d2
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
BASE_HOST = urlsplit(BASE).netloc
COOKIE = "agora_ref"
CODE_RE = re.compile(r"^[A-Za-z0-9_-]{1,32}$")
ATTRIBUTION_WAIT_S = 45
RELEASE_WAIT_S = int(os.environ.get("REFERRAL_E2E_RELEASE_WAIT_S", "150"))
SETTLE_S = 4
CODE_PLACEHOLDER = "Nhập mã của bạn bè"
RULE_TEXT = "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành."
EMPTY_REWARDS = "Chưa có phần thưởng nào."
HCM = ZoneInfo("Asia/Ho_Chi_Minh")


def _x(world: World) -> dict[str, Any]:
    return world.state.extra


def _until(fn: Callable[[], Any], *, timeout_s: float, what: str, interval_s: float = 1.0) -> Any:
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while time.monotonic() < deadline:
        last = fn()
        if last:
            return last
        time.sleep(interval_s)
    raise AssertionError(f"timed out after {timeout_s:.0f}s waiting for {what} (last={last!r})")


def _xu(n: int) -> str:
    return f"{n:,}".replace(",", ".") + " xu"


def _int(value: Any) -> int:
    return int(value or 0)


def _register_api(username: str, referral_code: str = "") -> str:
    body: dict[str, Any] = {
        "username": username,
        "password": SETTINGS.seed_password,
        "role": "buyer",
    }
    if referral_code:
        body["referralCode"] = referral_code
    data = BaseService().post(ep.AUTH_REGISTER, body)
    return (data.get("result") or {}).get("token", "")


def _new_buyer() -> tuple[str, str]:
    username = fake.unique_username("ri_buyer")
    return username, AuthService().register(username, SETTINGS.seed_password, "buyer")


def _referral(token: str) -> dict[str, Any]:
    return ReferralService(token=token).get_my_referral()


def _code_of(token: str) -> str:
    svc = ReferralService(token=token)
    code = svc.get_my_referral().get("code", "")
    if not code:
        code = svc.create_referral_code().get("code", "")
    assert code, "no referral code available for the buyer"
    return code


def _invited(token: str) -> int:
    return _int(_referral(token).get("invitedCount"))


def _make_inviter(world: World, label: str) -> dict[str, Any]:
    username, token = _new_buyer()
    inviter = {
        "username": username,
        "token": token,
        "code": _code_of(token),
    }
    inviter["base"] = _invited(token)
    _x(world)[label] = inviter
    return inviter


def _inviter(world: World, label: str = "ri_inviter") -> dict[str, Any]:
    return _x(world)[label]


def _short(world: World, short: str, kind: str = "last") -> None:
    ex = _x(world)
    ex["ri_short"] = short
    ex.setdefault("ri_links", {})[kind] = short


def _get_short(short: str, headers: dict[str, str] | None = None) -> dict[str, Any]:
    with httpx.Client(follow_redirects=False, timeout=30) as client:
        resp = client.get(f"{BASE}/s/{short}", headers=headers or {})
    location = urlsplit(resp.headers.get("location", ""))
    assert location.netloc in ("", BASE_HOST), f"redirect leaves the app origin: {location}"
    return {
        "status": resp.status_code,
        "path": location.path,
        "query": location.query,
        "cookies": resp.headers.get_list("set-cookie"),
    }


def _agora_ref_lines(cookies: list[str]) -> list[str]:
    return [c for c in cookies if c.startswith(f"{COOKIE}=")]


def _goto(world: World, route: str) -> None:
    goto_streamed(world.page, f"{BASE}{route}")


def _browser_cookie(world: World) -> list[dict]:
    return [c for c in world.context.cookies() if c["name"] == COOKIE]


def _set_browser_cookie(world: World, value: str) -> None:
    world.context.add_cookies([{"name": COOKIE, "value": value, "url": BASE}])


def _record_set_cookies(world: World) -> list[str]:
    sink: list[str] = []
    _x(world)["ri_set_cookies"] = sink

    def on_response(resp: Response) -> None:
        try:
            raw = resp.all_headers().get("set-cookie", "")
        except PlaywrightError:
            return
        sink.extend(line for line in raw.split("\n") if line)

    world.page.on("response", on_response)
    return sink


def _submit_register(world: World, route: str, username: str | None = None) -> str:
    _goto(world, route)
    name = username or fake.unique_username("ri_user")
    RegisterPage(world.page).register(name, SETTINGS.seed_password)
    _x(world)["ri_username"] = name
    return name


def _wait_path(world: World, path: str) -> None:
    world.page.wait_for_url(lambda url: urlsplit(url).path == path, timeout=timeouts.NAVIGATION)
    world.page.wait_for_load_state("networkidle")


def _rises(world: World, label: str) -> None:
    inviter = _inviter(world, label)
    _until(
        lambda: _invited(inviter["token"]) == inviter["base"] + 1,
        timeout_s=ATTRIBUTION_WAIT_S,
        what=f"{label} invited count to rise by 1 from {inviter['base']}",
    )


def _stays(world: World, label: str) -> None:
    inviter = _inviter(world, label)
    time.sleep(SETTLE_S)
    assert _invited(inviter["token"]) == inviter["base"], f"{label} invited count changed"


def _sign_in(world: World, prefix: str = "ri_signed") -> User:
    user = User(
        username=fake.unique_username(prefix), password=SETTINGS.seed_password, role="buyer"
    )
    login_via_api(world, user)
    return user


def _grant_clipboard(world: World) -> None:
    world.context.grant_permissions(["clipboard-read", "clipboard-write"], origin=BASE)


def _open_referral(world: World) -> None:
    _goto(world, "/account/referral")
    expect(world.page.get_by_test_id("referral-code")).to_be_visible(timeout=timeouts.NAVIGATION)


def _toast(world: World, text: str) -> Locator:
    return world.page.get_by_role("status").filter(has_text=text)


def _rewards(token: str) -> list[dict[str, Any]]:
    return ReferralService(token=token).list_referral_rewards(50).get("rewards", [])


def _status(reward: dict[str, Any]) -> str:
    raw = str(reward.get("status", ""))
    for name in ("PENDING", "RELEASED", "VOID"):
        if name in raw:
            return name
    return raw


def _shop(world: World) -> dict[str, Any]:
    ex = _x(world)
    if "ri_shop" not in ex:
        ex["ri_shop"] = d2.seed_shop()
    return ex["ri_shop"]


def _invitee_order(
    world: World,
    inviter: dict[str, Any],
    *,
    ret: bool,
    before_return: Callable[[], None] | None = None,
) -> str:
    before = _invited(inviter["token"])
    username = fake.unique_username("ri_invitee")
    token = _register_api(username, inviter["code"])
    _until(
        lambda: _invited(inviter["token"]) > before,
        timeout_s=ATTRIBUTION_WAIT_S,
        what="invitee to be attributed to the inviter",
    )
    AddressService(token=token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Hà Nội",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    shop = _shop(world)
    CartService(token=token).add_to_cart(shop["listing_id"], 1)
    orders = OrderService(token=token)
    res = orders.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    found = res.get("orders", [])
    order_id = found[0]["id"] if found else res["order"]["id"]
    PaymentService(token=token).mock_pay(order_id, 120_000, True)
    _until(
        lambda: (
            {o["id"]: o.get("status") for o in orders.list_buyer_orders().get("orders", [])}.get(
                order_id
            )
            == "ORDER_STATUS_PAID"
        ),
        timeout_s=20,
        what="invitee order to be PAID",
        interval_s=0.5,
    )
    d2.ship_order(shop, order_id, complete=True)
    if ret:
        if before_return:
            before_return()
        orders.create_return_request(order_id, "defective", 120_000)
    return order_id


def _wait_reward_count(inviter: dict[str, Any], n: int) -> None:
    _until(
        lambda: len(_rewards(inviter["token"])) >= n,
        timeout_s=ATTRIBUTION_WAIT_S,
        what=f"{n} reward(s) for the inviter",
    )


def _sum(rewards: list[dict[str, Any]], status: str) -> int:
    return sum(_int(r.get("amount")) for r in rewards if _status(r) == status)


# ── shared givens ─────────────────────────────────────────────────────────────
@given("an inviter with a referral code")
def inviter_with_code(world: World) -> None:
    _make_inviter(world, "ri_inviter")


@given("a second inviter with a referral code")
def second_inviter(world: World) -> None:
    _make_inviter(world, "ri_inviter2")


@given("a signed-in inviter with a referral code and clipboard access")
def signed_in_inviter(world: World) -> None:
    user = _sign_in(world, "ri_inviter")
    _x(world)["ri_inviter"] = {
        "username": user.username,
        "token": user.token,
        "code": _code_of(user.token),
        "base": 0,
    }
    _grant_clipboard(world)


@given(
    parsers.parse(
        'the inviter has a referral share link with utm_source "{source}" and utm_medium "{medium}"'
    )
)
def inviter_referral_link(world: World, source: str, medium: str) -> None:
    inviter = _inviter(world)
    res = SharingService(token=inviter["token"]).create_share_link(
        "referral", inviter["code"], {"utm_source": source, "utm_medium": medium}
    )
    assert res.get("shortCode"), res
    _short(world, res["shortCode"], "referral")


@given("a seller with a published listing")
def seller_with_listing(world: World) -> None:
    _shop(world)


@given(parsers.parse('a {kind} share link with utm_source "{source}"'))
def kind_share_link(world: World, kind: str, source: str) -> None:
    shop = _shop(world)
    target = shop["listing_id"] if kind == "listing" else shop["seller_id"]
    res = SharingService(token=shop["token"]).create_share_link(
        kind, target, {"utm_source": source}
    )
    assert res.get("shortCode"), res
    _short(world, res["shortCode"], kind)
    _x(world)[f"ri_target_{kind}"] = target


@given(
    parsers.parse(
        'a listing share link carrying utm_source "{source}" plus the key "{key}" valued "{value}"'
    )
)
def listing_link_with_extra_key(world: World, source: str, key: str, value: str) -> None:
    shop = _shop(world)
    res = SharingService(token=shop["token"]).create_share_link(
        "listing", shop["listing_id"], {"utm_source": source, key: value}
    )
    assert res.get("shortCode"), res
    _short(world, res["shortCode"], "listing")
    _x(world)["ri_target_listing"] = shop["listing_id"]


@given(parsers.parse('a share link of target type "{target_type}" with utm_source "{source}"'))
def odd_type_link(world: World, target_type: str, source: str) -> None:
    _, token = _new_buyer()
    res = SharingService(token=token).create_share_link(
        target_type, f"t-{uuid.uuid4().hex[:8]}", {"utm_source": source}
    )
    assert res.get("shortCode"), res
    _short(world, res["shortCode"])


@given(parsers.parse('a referral share link whose target id is "{target_id}"'))
def unsafe_referral_link(world: World, target_id: str) -> None:
    _, token = _new_buyer()
    res = SharingService(token=token).create_share_link(
        "referral", target_id, {"utm_source": "referral", "utm_medium": "invite"}
    )
    assert res.get("shortCode"), res
    _short(world, res["shortCode"])


@given("an existing account")
def existing_account(world: World) -> None:
    username, _ = _new_buyer()
    _x(world)["ri_existing"] = username


@given("the browser holds the agora_ref cookie of the second inviter")
def cookie_second(world: World) -> None:
    _set_browser_cookie(world, _inviter(world, "ri_inviter2")["code"])


@given("the browser holds the agora_ref cookie of the inviter")
def cookie_inviter(world: World) -> None:
    _set_browser_cookie(world, _inviter(world)["code"])


# ── S1 / S4 ────────────────────────────────────────────────────────────────────
@when('the buyer opens the referral page and taps "Chia sẻ link mời"')
def tap_share(world: World) -> None:
    _open_referral(world)
    button = world.page.get_by_role("button", name="Chia sẻ link mời")
    RegisterPage(world.page).wait_until_interactive(button)
    button.click()


@then(parsers.parse('the toast "{text}" is shown'))
def toast_shown(world: World, text: str) -> None:
    expect(_toast(world, text).first).to_be_visible(timeout=timeouts.DEFAULT)


@then('the clipboard holds the site origin followed by "/s/" and a short code')
def clipboard_link(world: World) -> None:
    text = world.page.evaluate("navigator.clipboard.readText()")
    match = re.fullmatch(re.escape(BASE) + r"/s/([A-Za-z0-9_-]+)", text)
    assert match, f"clipboard is {text!r}"
    _x(world)["ri_clip_short"] = match.group(1)


@then(
    'that short code resolves to a referral link for the buyer\'s code with utm_source "referral" and utm_medium "invite"'
)
def clipboard_resolves(world: World) -> None:
    inviter = _inviter(world)
    res = SharingService(token=inviter["token"]).resolve_share_link(_x(world)["ri_clip_short"])
    assert res.get("targetType") == "referral", res
    assert res.get("targetId") == inviter["code"], res
    assert res.get("utm") == {"utm_source": "referral", "utm_medium": "invite"}, res


@when("an anonymous visitor opens the referral page")
def anonymous_referral(world: World) -> None:
    world.page.goto(f"{BASE}/account/referral", wait_until="domcontentloaded")
    world.page.wait_for_url(lambda url: urlsplit(url).path == "/login", timeout=timeouts.NAVIGATION)


@then(parsers.parse('the visitor is on the login page with returnUrl "{expected}"'))
def on_login_with_return(world: World, expected: str) -> None:
    query = parse_qs(urlsplit(world.page.url).query)
    assert query.get("returnUrl") == [expected], world.page.url


@then("no referral code, invite button or xu total is rendered")
def nothing_rendered(world: World) -> None:
    page = world.page
    assert page.get_by_test_id("referral-code").count() == 0
    assert page.get_by_role("button", name="Chia sẻ link mời").count() == 0
    assert page.get_by_text("Xu đang chờ").count() == 0
    assert page.get_by_text(re.compile(r"\d\s*xu$")).count() == 0


# ── /s/<code> ─────────────────────────────────────────────────────────────────
@when("the short link is requested without following redirects")
def request_short(world: World) -> None:
    _x(world)["ri_resp"] = _get_short(_x(world)["ri_short"])


@when("the short link is requested without following redirects behind an https proxy")
def request_short_https(world: World) -> None:
    _x(world)["ri_resp_https"] = _get_short(_x(world)["ri_short"], {"x-forwarded-proto": "https"})


@when("each short link is requested without following redirects")
def request_each(world: World) -> None:
    _x(world)["ri_resps"] = {
        kind: _get_short(short) for kind, short in _x(world)["ri_links"].items()
    }


@when("a short link that does not exist is requested without following redirects")
def request_unknown(world: World) -> None:
    _x(world)["ri_resp"] = _get_short(f"zz{uuid.uuid4().hex[:10]}")


@then(
    'the answer is 307 to the register page with ref the inviter\'s code and utm_medium "invite" and utm_source "referral" in that order'
)
def answer_register(world: World) -> None:
    resp = _x(world)["ri_resp"]
    assert resp["status"] == 307, resp
    assert resp["path"] == "/register", resp
    assert resp["query"] == f"ref={_inviter(world)['code']}&utm_medium=invite&utm_source=referral"


def _attributes(line: str) -> list[str]:
    return [part.strip().lower() for part in line.split(";")[1:]]


@then(
    "the agora_ref cookie holds the inviter's code with HttpOnly, SameSite=Lax, Path=/ and Max-Age=604800 and no Secure attribute"
)
def cookie_attrs(world: World) -> None:
    lines = _agora_ref_lines(_x(world)["ri_resp"]["cookies"])
    assert len(lines) == 1, _x(world)["ri_resp"]["cookies"]
    assert lines[0].split(";")[0] == f"{COOKIE}={_inviter(world)['code']}", lines[0]
    attrs = _attributes(lines[0])
    for needed in ("httponly", "samesite=lax", "path=/", "max-age=604800"):
        assert needed in attrs, (needed, lines[0])
    assert "secure" not in attrs, lines[0]


@then("the agora_ref cookie also carries the Secure attribute")
def cookie_secure(world: World) -> None:
    lines = _agora_ref_lines(_x(world)["ri_resp_https"]["cookies"])
    assert len(lines) == 1, _x(world)["ri_resp_https"]["cookies"]
    assert "secure" in _attributes(lines[0]), lines[0]


@then('the listing link answers 307 to the listing page with utm_source "share"')
def listing_answer(world: World) -> None:
    resp = _x(world)["ri_resps"]["listing"]
    assert resp["status"] == 307, resp
    assert resp["path"] == f"/listing/{_x(world)['ri_target_listing']}", resp
    assert resp["query"] == "utm_source=share", resp


@then('the shop link and the seller link answer 307 to the shop page with utm_source "share"')
def shop_answers(world: World) -> None:
    for kind in ("shop", "seller"):
        resp = _x(world)["ri_resps"][kind]
        assert resp["status"] == 307, (kind, resp)
        assert resp["path"] == f"/shop/{_x(world)[f'ri_target_{kind}']}", (kind, resp)
        assert resp["query"] == "utm_source=share", (kind, resp)


@then("none of the three responses sets agora_ref")
def none_set_cookie(world: World) -> None:
    for kind, resp in _x(world)["ri_resps"].items():
        assert not _agora_ref_lines(resp["cookies"]), (kind, resp["cookies"])


@then(parsers.parse('the answer is 307 to "{path}" with no query'))
def answer_plain(world: World, path: str) -> None:
    resp = _x(world)["ri_resp"]
    assert resp["status"] == 307, resp
    assert resp["path"] == path, resp
    assert resp["query"] == "", resp


@then("the response sets no agora_ref cookie")
def response_no_cookie(world: World) -> None:
    assert not _agora_ref_lines(_x(world)["ri_resp"]["cookies"]), _x(world)["ri_resp"]["cookies"]


@then('the answer is 307 to the listing page with utm_source "share" only')
def listing_only_utm(world: World) -> None:
    resp = _x(world)["ri_resp"]
    assert resp["status"] == 307, resp
    assert resp["path"] == f"/listing/{_x(world)['ri_target_listing']}", resp
    assert resp["query"] == "utm_source=share", resp


# ── register ──────────────────────────────────────────────────────────────────
@when("a visitor registers on the register page opened with the inviter's code as ref")
def register_with_ref(world: World) -> None:
    _submit_register(world, f"/register?ref={quote(_inviter(world)['code'])}")


@when("a visitor registers on the register page opened with the first inviter's code as ref")
def register_with_first_ref(world: World) -> None:
    _submit_register(world, f"/register?ref={quote(_inviter(world)['code'])}")


@when(
    parsers.parse('a visitor registers on the register page opened with the malformed ref "{ref}"')
)
def register_malformed(world: World, ref: str) -> None:
    _submit_register(world, f"/register?ref={quote(ref, safe='')}")


@when(
    "a visitor follows the short link in the browser and then registers on the plain register page"
)
def follow_then_register(world: World) -> None:
    _goto(world, f"/s/{_x(world)['ri_short']}")
    world.page.wait_for_url(
        lambda url: urlsplit(url).path == "/register", timeout=timeouts.NAVIGATION
    )
    assert _browser_cookie(world), "agora_ref was not set by the short link"
    _record_set_cookies(world)
    _submit_register(world, "/register")


@when("a visitor registers on the plain register page recording Set-Cookie headers")
def register_plain_recording(world: World) -> None:
    _goto(world, "/register")
    _record_set_cookies(world)
    RegisterPage(world.page).register(fake.unique_username("ri_user"), SETTINGS.seed_password)


@when(
    parsers.parse(
        'a visitor registers on the register page opened with returnUrl "{return_url}" and the inviter\'s code as ref'
    )
)
def register_with_return(world: World, return_url: str) -> None:
    code = quote(_inviter(world)["code"])
    _submit_register(world, f"/register?returnUrl={quote(return_url, safe='')}&ref={code}")


@when("a visitor submits the register form with the existing username")
def register_existing(world: World) -> None:
    _record_set_cookies(world)
    _submit_register(world, "/register", username=_x(world)["ri_existing"])


@then("the visitor lands on the home page")
def lands_home(world: World) -> None:
    _wait_path(world, "/")


@then(parsers.parse('the visitor lands on "{path}"'))
def lands_on(world: World, path: str) -> None:
    _wait_path(world, path)


@then("the inviter's invited count rises by 1")
def inviter_rises(world: World) -> None:
    _rises(world, "ri_inviter")


@then("the first inviter's invited count rises by 1")
def first_rises(world: World) -> None:
    _rises(world, "ri_inviter")


@then("the inviter's invited count stays 0")
def inviter_stays(world: World) -> None:
    _stays(world, "ri_inviter")


@then("the second inviter's invited count stays 0")
def second_stays(world: World) -> None:
    _stays(world, "ri_inviter2")


@then("the register response deleted agora_ref and the browser holds no agora_ref cookie")
def cookie_deleted(world: World) -> None:
    world.page.wait_for_load_state("networkidle")
    lines = _agora_ref_lines(_x(world)["ri_set_cookies"])
    assert any(
        "max-age=0" in _attributes(line)
        or any(a.startswith("expires=thu, 01 jan 1970") for a in _attributes(line))
        for line in lines
    ), lines
    assert _browser_cookie(world) == [], _browser_cookie(world)


@then("the new account can sign in through the gateway")
def new_account_signs_in(world: World) -> None:
    assert AuthService().login(_x(world)["ri_username"], SETTINGS.seed_password)


@then(parsers.parse('the form shows "{text}"'))
def form_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.DEFAULT)
    assert urlsplit(world.page.url).path == "/register", world.page.url


@then("the browser still holds the agora_ref cookie of the inviter")
def cookie_kept(world: World) -> None:
    cookies = _browser_cookie(world)
    assert [c["value"] for c in cookies] == [_inviter(world)["code"]], cookies


@then("no response set an agora_ref cookie")
def no_cookie_set(world: World) -> None:
    world.page.wait_for_load_state("networkidle")
    assert _agora_ref_lines(_x(world)["ri_set_cookies"]) == []
    assert _browser_cookie(world) == []


# ── referral page: totals and timeline ─────────────────────────────────────────
@given("an inviter whose invitee has a completed order with a pending reward")
def inviter_with_pending(world: World) -> None:
    inviter = _make_inviter(world, "ri_inviter")
    _invitee_order(world, inviter, ret=False)
    _until(
        lambda: _int(_referral(inviter["token"]).get("pendingXu")) > 0,
        timeout_s=ATTRIBUTION_WAIT_S,
        what="a pending reward for the inviter",
    )
    user = User(username=inviter["username"], password=SETTINGS.seed_password, role="buyer")
    login_via_api(world, user)


@given(
    "an inviter with two released rewards, one pending reward held by the daily cap "
    "and one voided reward"
)
def inviter_three_states(world: World) -> None:
    inviter = _make_inviter(world, "ri_inviter")
    for n in (1, 2, 3):
        _invitee_order(world, inviter, ret=False)
        _wait_reward_count(inviter, n)
    _until(
        lambda: (
            sorted(_status(r) for r in _rewards(inviter["token"]))
            == ["PENDING", "RELEASED", "RELEASED"]
        ),
        timeout_s=RELEASE_WAIT_S,
        what="two rewards released and one held by the daily cap (needs the referral-reward overlay)",
        interval_s=2.0,
    )
    _invitee_order(world, inviter, ret=True, before_return=lambda: _wait_reward_count(inviter, 4))
    _until(
        lambda: len(_rewards(inviter["token"])) == 3,
        timeout_s=ATTRIBUTION_WAIT_S,
        what="the returned order's reward to be voided and dropped from the list",
    )
    time.sleep(SETTLE_S)
    rewards = _rewards(inviter["token"])
    assert sorted(_status(r) for r in rewards) == ["PENDING", "RELEASED", "RELEASED"], rewards
    user = User(username=inviter["username"], password=SETTINGS.seed_password, role="buyer")
    login_via_api(world, user)


@given("another signed-in buyer with their own referral code")
def other_buyer(world: World) -> None:
    user = _sign_in(world, "ri_other")
    _x(world)["ri_other"] = {"token": user.token, "code": _code_of(user.token)}


@when("the inviter opens the referral page")
@when("the buyer opens the referral page")
@when("the other buyer opens the referral page")
def open_referral(world: World) -> None:
    _open_referral(world)


_STAT_JS = """(title) => {
  const all = Array.from(document.querySelectorAll('body *'));
  const el = all.find(e => e.children.length === 0 && e.textContent.trim() === title);
  if (!el) return null;
  let node = el;
  for (let i = 0; i < 4 && node; i += 1, node = node.parentElement) {
    if (node.textContent.includes(' xu')) return node.textContent.trim();
  }
  return null;
}"""


def _stat(world: World, title: str) -> str:
    expect(world.page.get_by_text(title, exact=True)).to_be_visible(timeout=timeouts.NAVIGATION)
    text = world.page.evaluate(_STAT_JS, title)
    assert text, f"no xu value next to {title!r}"
    return text


def _assert_stat(world: World, title: str, expected: int) -> None:
    text = _stat(world, title)
    assert _xu(expected) in text, f"{title}: expected {_xu(expected)!r} in {text!r}"


@then(
    '"Xu đang chờ" shows the pending xu from the gateway formatted with dots and the suffix " xu"'
)
def pending_formatted(world: World) -> None:
    ref = _referral(_inviter(world)["token"])
    assert _int(ref.get("pendingXu")) > 0, ref
    _assert_stat(world, "Xu đang chờ", _int(ref.get("pendingXu")))


@then(
    '"Xu đã nhận" shows the released xu from the gateway formatted with dots and the suffix " xu"'
)
def released_formatted(world: World) -> None:
    ref = _referral(_inviter(world)["token"])
    _assert_stat(world, "Xu đã nhận", _int(ref.get("releasedXu")))


@then("the page contains no đ currency amount")
def no_dong(world: World) -> None:
    content = world.page.locator('nav[aria-label="Menu tài khoản"] + div')
    expect(content.get_by_test_id("referral-code")).to_be_visible(timeout=timeouts.DEFAULT)
    body = content.inner_text()
    found = re.findall(r"\d[\d.,]*\s*[đ₫](?!\w)|[đ₫]\s*\d", body)
    assert not found, found
    assert "₫" not in body


@then('the timeline shows "Đang chờ" once, "Đã nhận" once per released reward and no "Đã hủy"')
def badges_listed(world: World) -> None:
    rewards = _rewards(_inviter(world)["token"])
    released = sum(1 for r in rewards if _status(r) == "RELEASED")
    expect(world.page.get_by_text("Đang chờ", exact=True)).to_have_count(
        1, timeout=timeouts.DEFAULT
    )
    expect(world.page.get_by_text("Đã nhận", exact=True)).to_have_count(
        released, timeout=timeouts.DEFAULT
    )
    expect(world.page.get_by_text("Đã hủy", exact=True)).to_have_count(0)


@then(
    'the pending reward shows "Dự kiến nhận:" with its release date as dd/MM/yyyy in Asia/Ho_Chi_Minh'
)
def pending_release_date(world: World) -> None:
    pending = [r for r in _rewards(_inviter(world)["token"]) if _status(r) == "PENDING"]
    assert len(pending) == 1, pending
    raw = re.sub(r"(\.\d{6})\d+", r"\1", str(pending[0]["releaseAt"]).replace("Z", "+00:00"))
    day = datetime.fromisoformat(raw).astimezone(HCM).strftime("%d/%m/%Y")
    expect(world.page.get_by_text(f"Dự kiến nhận: {day}")).to_be_visible(timeout=timeouts.DEFAULT)


_AMOUNT_JS = """() => {
  const amount = /^\\+?\\s*\\d[\\d.]*\\s*xu$/;
  const all = Array.from(document.querySelectorAll('body *'));
  const out = [];
  for (const el of all) {
    const badge = el.textContent.trim();
    if (el.children.length !== 0 || !['Đang chờ', 'Đã nhận', 'Đã hủy'].includes(badge)) continue;
    let item = el;
    let leaf = null;
    while (item && !leaf) {
      leaf = Array.from(item.querySelectorAll('*')).find(
        e => e.children.length === 0 && amount.test(e.textContent.trim()));
      if (!leaf) item = item.parentElement;
    }
    let struck = false;
    for (let n = leaf; n && item && n !== item.parentElement; n = n.parentElement) {
      if (getComputedStyle(n).textDecorationLine.includes('line-through')) struck = true;
    }
    out.push({ badge, amount: leaf ? leaf.textContent.trim() : null, struck });
  }
  return out;
}"""


@then("the listed rows are exactly the released and pending rewards, none struck through")
def rows_match_list(world: World) -> None:
    rewards = _rewards(_inviter(world)["token"])
    badge = {"PENDING": "Đang chờ", "RELEASED": "Đã nhận"}
    want = sorted((badge[_status(r)], _int(r.get("amount"))) for r in rewards)
    rows = world.page.evaluate(_AMOUNT_JS)
    got = sorted((row["badge"], int(re.sub(r"\D", "", row["amount"] or "0"))) for row in rows)
    assert got == want, (got, want)
    assert not any(row["struck"] for row in rows), rows


@then('"Xu đang chờ" counts only the pending reward and "Xu đã nhận" only the released rewards')
def totals_exclude_void(world: World) -> None:
    rewards = _rewards(_inviter(world)["token"])
    _assert_stat(world, "Xu đang chờ", _sum(rewards, "PENDING"))
    _assert_stat(world, "Xu đã nhận", _sum(rewards, "RELEASED"))


@then('"0 xu" is shown twice')
def zero_twice(world: World) -> None:
    expect(world.page.get_by_text("0 xu", exact=True)).to_have_count(2, timeout=timeouts.DEFAULT)


@then(parsers.parse('the text "{text}" is shown'))
def text_shown(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.DEFAULT)


@then("the page shows the other buyer's own code and not the inviter's code")
def own_code_only(world: World) -> None:
    shown = world.page.get_by_test_id("referral-code").inner_text().strip()
    assert shown == _x(world)["ri_other"]["code"], shown
    assert _inviter(world)["code"] not in world.page.inner_text("body")


@then('"Xu đang chờ" and "Xu đã nhận" both show "0 xu"')
def both_zero(world: World) -> None:
    for title in ("Xu đang chờ", "Xu đã nhận"):
        _assert_stat(world, title, 0)


@then(parsers.parse('no reward is listed and "{text}" is shown'))
def no_reward_listed(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.DEFAULT)
    for badge in ("Đang chờ", "Đã nhận", "Đã hủy"):
        assert world.page.get_by_text(badge, exact=True).count() == 0, badge


# ── redeem ────────────────────────────────────────────────────────────────────
@then(
    'the text "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành." is shown under the redeem field'
)
def rule_text(world: World) -> None:
    field = world.page.get_by_placeholder(CODE_PLACEHOLDER)
    expect(field).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(field.locator("xpath=ancestor::form")).to_contain_text(RULE_TEXT)


@given(parsers.parse("a signed-in buyer on a referral page with {situation}"))
def buyer_on_referral(world: World, situation: str) -> None:
    user = _sign_in(world, "ri_redeemer")
    ex = _x(world)
    ex["ri_redeemer"] = {"token": user.token, "base": 0}
    _until(
        lambda: _referral_knows(user.token),
        timeout_s=ATTRIBUTION_WAIT_S,
        what="team-referral to record the buyer's registration",
    )
    if situation == "a code already redeemed":
        friend = _make_inviter(world, "ri_friend1")
        _until(
            lambda: _redeem_when_registered(user.token, friend["code"]),
            timeout_s=ATTRIBUTION_WAIT_S,
            what="team-referral to record the buyer's registration and accept the redeem",
        )
    else:
        assert situation == "no earlier redeem", situation
    _open_referral(world)


def _referral_knows(token: str) -> bool:
    user_id = ue.user_id_of(token)
    proc = subprocess.run(
        ["docker", "exec", os.getenv("POSTGRES_CONTAINER", "agora-postgres-1"), "psql",
         "-U", "postgres", "-d", "referral_db", "-At", "-c",
         f"SELECT 1 FROM referral_accounts WHERE user_id = '{user_id}'"],
        capture_output=True, text=True, timeout=30,
    )  # fmt: skip
    return proc.stdout.strip() == "1"


def _redeem_when_registered(token: str, code: str) -> bool:
    try:
        ReferralService(token=token).redeem_referral(code)
    except GatewayError as err:
        if err.status == 400 and "failed_precondition" in err.body and "tài khoản mới" in err.body:
            return False
        raise
    return True


def _redeem(world: World, code: str) -> None:
    field = world.page.get_by_placeholder(CODE_PLACEHOLDER)
    RegisterPage(world.page).wait_until_interactive(field)
    field.fill(code)
    world.page.get_by_role("button", name="Nhập mã").click()


@when(parsers.parse("the buyer submits {code_kind} in the redeem field"))
def submit_code_kind(world: World, code_kind: str) -> None:
    ex = _x(world)
    if code_kind == "their own code":
        code = world.page.get_by_test_id("referral-code").inner_text().strip()
        target = {"token": ex["ri_redeemer"]["token"], "base": 0}
    else:
        assert code_kind == "another friend's code", code_kind
        friend = _make_inviter(world, "ri_friend2")
        code, target = friend["code"], friend
    ex["ri_attempt_target"] = target
    _redeem(world, code)


@then(
    "an error toast is shown and the redeem field is invalid with the same Vietnamese message as help"
)
def redeem_refused(world: World) -> None:
    field = world.page.get_by_placeholder(CODE_PLACEHOLDER)
    expect(field).to_have_attribute("aria-invalid", "true", timeout=timeouts.NAVIGATION)
    help_id = field.get_attribute("aria-describedby")
    assert help_id, "redeem field has no help text"
    help_text = world.page.locator(f'[id="{help_id}"]').inner_text().strip()
    assert help_text, "help text is empty"
    toast = world.page.get_by_role("alert").filter(has_text=help_text)
    expect(toast.first).to_be_visible(timeout=timeouts.DEFAULT)
    assert re.search(r"[À-ỹ]", help_text), f"message is not Vietnamese: {help_text!r}"


@then("no referral was attributed for that attempt")
def nothing_attributed(world: World) -> None:
    target = _x(world)["ri_attempt_target"]
    time.sleep(SETTLE_S)
    assert _invited(target["token"]) == target["base"]


@when("the buyer redeems the inviter's code on the referral page")
def redeem_inviter_code(world: World) -> None:
    _redeem(world, _inviter(world)["code"])


@then(parsers.parse('the redeem success toast "{text}" is shown and the redeem field is empty'))
def redeem_accepted(world: World, text: str) -> None:
    expect(_toast(world, text).first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(world.page.get_by_placeholder(CODE_PLACEHOLDER)).to_have_value(
        "", timeout=timeouts.DEFAULT
    )
