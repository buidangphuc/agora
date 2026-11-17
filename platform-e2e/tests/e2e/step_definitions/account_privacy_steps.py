"""Steps for frontend/account_privacy_ui.feature (change frontend-account-privacy).

Black box: the browser drives /account/privacy; state is read back through the gateway
(PrivacyService.ListMyConsents / GetAccountDeletionStatus) and "calls the gateway received" from the
team-gateway log. Accounts and the open order are seeded through real RPCs.
"""

from __future__ import annotations

import json
import re
import time
from urllib.parse import unquote, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages.privacy_page import PrivacyPage
from tests.e2e.flows.auth_flow import SESSION_COOKIE, session_cookies
from tests.e2e.step_definitions.account_deletion_steps import _buyer_with_open_order
from tests.e2e.step_definitions.frontend_legal_steps import ANALYTICS, MARKETING
from tests.e2e.step_definitions.legal_consent_steps import PERSONALIZATION, _all_records
from tests.e2e.support import account_deletion_support as ad
from tests.e2e.support import data_export_support as de
from tests.e2e.support import legal_ui_support as lu
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

CONSENT_COOKIE = "agora_consent"
PRIVACY_PATH = "/account/privacy"


def _x(world: World) -> dict:
    return world.state.extra.setdefault("priv", {})


def _page(world: World) -> PrivacyPage:
    return PrivacyPage(world.page)


def _base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def _user(world: World) -> dict:
    return _x(world)["user"]


def _sign_in(world: World, user: dict, cookie: str | None = None) -> None:
    world.context.add_cookies(session_cookies(world.settings, user["token"]))
    if cookie is not None:
        world.context.add_cookies(
            [
                {
                    "name": CONSENT_COOKIE,
                    "value": f"v=1&{cookie}&ts={int(time.time())}",
                    "url": _base(world),
                }
            ]
        )
    _x(world)["user"] = user
    _x(world)["baseline"] = len(_all_records(user["token"]))


def _consent_cookie(world: World) -> str:
    for c in world.context.cookies(_base(world)):
        if c["name"] == CONSENT_COOKIE:
            return unquote(c["value"])
    return ""


def _calls(procedure: str, since: str) -> int:
    lu.settle_gateway_log()
    return lu.gateway_calls(procedure, since)


def _record(user: dict, purpose: str, granted: bool) -> None:
    resp = ue.call(
        de.RECORD_CONSENT,
        {
            "records": [
                {"purpose": purpose, "granted": granted, "version": "1", "source": "banner"}
            ]
        },
        user["token"],
    )
    assert resp.status_code == 200, f"RecordConsent {resp.status_code}: {resp.text[:300]}"


# ── givens ───────────────────────────────────────────────────────────────────
@given("a signed-in privacy buyer")
def buyer(world: World) -> None:
    _sign_in(world, ue.register_user("buyer", "priv"))


@given("a signed-in privacy buyer on the account security page")
def buyer_on_security(world: World) -> None:
    _sign_in(world, ue.register_user("buyer", "priv"))
    _page(world).open("/account/security")
    expect(_page(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)


@given("a privacy visitor with no session")
def visitor(world: World) -> None:
    world.context.clear_cookies()


@given(parsers.parse('a signed-in privacy buyer with the consent cookie "{value}"'))
def buyer_with_cookie(world: World, value: str) -> None:
    _sign_in(world, ue.register_user("buyer", "priv"), cookie=value)


@given(
    parsers.parse(
        'a signed-in privacy buyer with analytics granted and the consent cookie "{value}"'
    )
)
def buyer_analytics_granted(world: World, value: str) -> None:
    user = ue.register_user("buyer", "priv")
    _record(user, ANALYTICS, True)
    _sign_in(world, user, cookie=value)


@given(
    parsers.parse(
        "a signed-in privacy buyer with {n:d} consent records whose newest is PERSONALIZATION"
    )
)
def buyer_with_records(world: World, n: int) -> None:
    user = ue.register_user("buyer", "priv")
    have = len(_all_records(user["token"]))
    cycle = [MARKETING, PERSONALIZATION, ANALYTICS]
    missing = n - have
    assert missing >= 1, f"registration already left {have} consent rows"
    order = [cycle[(missing - 1 - i) % 3] for i in range(missing)][::-1]
    order[-1] = PERSONALIZATION
    for i, purpose in enumerate(order):
        _record(user, purpose, i % 2 == 0)
    rows = _all_records(user["token"])
    assert len(rows) == n and rows[0]["purpose"] == PERSONALIZATION, rows[:2]
    _sign_in(world, user)


@given("the privacy stack runs the default 48 hour cancel window")
def default_window(world: World) -> None:
    window = ad.configured_window_s()
    assert window in (None, ad.DEFAULT_WINDOW_S), (
        f"team-identity runs with ACCOUNT_DELETION_CANCEL_WINDOW={window}s; "
        "use the default stack for this scenario"
    )


@given("a signed-in privacy buyer with no obligations")
def buyer_no_obligations(world: World) -> None:
    _sign_in(world, ue.register_user("buyer", "priv_del"))


@given("a signed-in privacy buyer with an open order")
def buyer_open_order(world: World) -> None:
    _sign_in(world, _buyer_with_open_order(world, "priv_open"))


@given("a signed-in privacy buyer with a pending deletion")
def buyer_pending(world: World) -> None:
    user = ue.register_user("buyer", "priv_pend")
    resp = ad.request_deletion(user["token"], user["password"])
    assert resp.status_code == 200, f"setup request failed: {resp.status_code} {resp.text[:300]}"
    assert ad.is_pending(user["token"]), "setup deletion is not pending"
    _sign_in(world, user)


# ── navigation ───────────────────────────────────────────────────────────────
@when(parsers.parse('the buyer opens "{label}" from the privacy account menu'))
def open_from_menu(world: World, label: str) -> None:
    _page(world).menu_link(label).click()
    world.page.wait_for_url(f"**{PRIVACY_PATH}**", timeout=timeouts.NAVIGATION)


@when("the buyer opens the privacy page")
def open_page(world: World) -> None:
    _page(world).open(PRIVACY_PATH)
    expect(_page(world).heading).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the visitor opens the privacy page")
def visitor_opens(world: World) -> None:
    _x(world)["since"] = lu.mark()
    world.page.goto(_base(world) + PRIVACY_PATH, wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)


@when("the buyer opens page 2 of the privacy history")
def open_page_two(world: World) -> None:
    _page(world).page_two_link.first.click()
    world.page.wait_for_url("**page=2**", timeout=timeouts.NAVIGATION)


@then(parsers.parse('the browser is on "{path}" with the heading "{heading}"'))
def on_page(world: World, path: str, heading: str) -> None:
    world.page.wait_for_url(f"**{path}**", timeout=timeouts.NAVIGATION)
    assert urlsplit(world.page.url).path == path, world.page.url
    expect(_page(world).heading).to_have_text(heading, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the privacy account menu marks "{label}" as the current page'))
def menu_current(world: World, label: str) -> None:
    expect(_page(world).menu_link(label)).to_have_attribute(
        "aria-current", "page", timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the browser is redirected to "{path}" with the return URL "{ret}"'))
def redirected(world: World, path: str, ret: str) -> None:
    world.page.wait_for_url(f"**{path}**", timeout=timeouts.NAVIGATION)
    url = unquote(world.page.url)
    assert urlsplit(world.page.url).path == path, world.page.url
    assert ret in url.split("?", 1)[-1], url


@then(parsers.parse("the gateway received {n:d} PrivacyService calls from the privacy visitor"))
def visitor_calls(world: World, n: int) -> None:
    assert _calls(lu.PRIVACY_PROC, _x(world)["since"]) == n


# ── consent ──────────────────────────────────────────────────────────────────
@then(parsers.parse("the consent history shows {n:d} rows and the first row is the newest record"))
def history_first_page(world: World, n: int) -> None:
    page = _page(world)
    expect(page.history_rows).to_have_count(n, timeout=timeouts.NAVIGATION)
    expect(page.history_rows.first).to_contain_text("Cá nhân hóa")


@then("a pagination link to page 2 is shown")
def page_two_link(world: World) -> None:
    expect(_page(world).page_two_link.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse("the consent history shows {n:d} rows"))
def history_rows(world: World, n: int) -> None:
    expect(_page(world).history_rows).to_have_count(n, timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer switches "{off}" off and "{on}" on and saves'))
def switch_and_save(world: World, off: str, on: str) -> None:
    page = _page(world)
    page.toggle(off).uncheck()
    page.toggle(on).check()
    _x(world)["since"] = lu.mark()
    page.save_button.click()


@when(parsers.parse('the buyer switches "{label}" on and saves'))
def switch_on_and_save(world: World, label: str) -> None:
    page = _page(world)
    page.toggle(label).check()
    _x(world)["since"] = lu.mark()
    page.save_button.click()


@then(parsers.parse('the toast "{text}" is shown'))
def toast(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the privacy consent cookie holds "{fragment}"'))
def cookie_holds(world: World, fragment: str) -> None:
    deadline = time.monotonic() + 10
    while fragment not in _consent_cookie(world) and time.monotonic() < deadline:
        time.sleep(0.25)
    assert fragment in _consent_cookie(world), _consent_cookie(world)


@then(parsers.parse("the gateway received {n:d} RecordConsent call from the privacy buyer"))
def record_calls(world: World, n: int) -> None:
    assert _calls(lu.RECORD_PROC, _x(world)["since"]) == n


@then(
    parsers.parse(
        "the ledger gained exactly 2 account rows for ANALYTICS withdrawn and MARKETING granted "
        'at version "{version}"'
    )
)
def ledger_two_rows(world: World, version: str) -> None:
    deadline = time.monotonic() + 15
    new: list[dict] = []
    while time.monotonic() < deadline:
        rows = _all_records(_user(world)["token"])
        new = rows[: len(rows) - _x(world)["baseline"]]
        if len(new) >= 2:
            break
        time.sleep(0.5)
    assert len(new) == 2, new
    by = {r["purpose"]: r for r in new}
    assert set(by) == {ANALYTICS, MARKETING}, new
    assert not by[ANALYTICS].get("granted") and by[MARKETING].get("granted") is True, new
    assert all(r["source"] == "account" and r["version"] == version for r in new), new


@then("the consent history shows the 2 new records")
def history_gained(world: World) -> None:
    expect(_page(world).history_rows).to_have_count(
        _x(world)["baseline"] + 2, timeout=timeouts.NAVIGATION
    )


# ── export ───────────────────────────────────────────────────────────────────
@when(parsers.parse('the buyer clicks "{label}"'))
def click_export(world: World, label: str) -> None:
    with world.page.expect_download(timeout=timeouts.NAVIGATION) as info:
        world.page.get_by_role("button", name=label).click()
    _x(world)["download"] = info.value


@then('the browser receives one file named "agora-du-lieu-<date>.json" that is valid JSON')
def download_name(world: World) -> None:
    download = _x(world)["download"]
    assert re.fullmatch(
        r"agora-du-lieu-\d{4}-\d{2}-\d{2}\.json", download.suggested_filename
    ), download.suggested_filename
    with open(download.path(), "rb") as fh:
        _x(world)["doc"] = json.loads(fh.read())


@then("the downloaded JSON has account.user_id equal to the buyer id")
def download_user(world: World) -> None:
    account = _x(world)["doc"]["account"]
    assert (account.get("user_id") or account.get("userId")) == _user(world)["id"], account


# ── deletion ─────────────────────────────────────────────────────────────────
def _confirm_deletion(world: World, password: str) -> None:
    page = _page(world)
    page.delete_offer.click()
    expect(page.dialog).to_be_visible(timeout=timeouts.NAVIGATION)
    page.password_field.fill(password)
    _x(world)["before"] = time.time()
    page.dialog_confirm.click()


@when("the buyer opens the deletion dialog and confirms with the correct password")
def confirm_correct(world: World) -> None:
    _confirm_deletion(world, _user(world)["password"])
    _x(world)["after"] = time.time()


@when(
    parsers.parse('the buyer opens the deletion dialog and confirms with the password "{password}"')
)
def confirm_password(world: World, password: str) -> None:
    _confirm_deletion(world, password)


@then(parsers.parse('the pending deletion panel shows with the "{label}" button'))
def pending_panel(world: World, label: str) -> None:
    expect(world.page.get_by_role("button", name=label)).to_be_visible(timeout=timeouts.NAVIGATION)
    _x(world)["after"] = time.time()


@then("the stored erase time is the deletion window after the request within 5 seconds")
def erase_time(world: World) -> None:
    erase = ad.erase_after_of(ad.status_body(_user(world)["token"]))
    assert erase is not None, "no erase_after stored"
    x = _x(world)
    low = x["before"] + ad.DEFAULT_WINDOW_S - ad.TIMESTAMP_SKEW_S
    high = x["after"] + ad.DEFAULT_WINDOW_S + ad.TIMESTAMP_SKEW_S
    assert low <= erase <= high, (erase - x["before"], ad.DEFAULT_WINDOW_S)


@then(parsers.parse('the deletion dialog stays open with "{message}" and an empty password field'))
def dialog_wrong(world: World, message: str) -> None:
    page = _page(world)
    expect(page.dialog.get_by_text(message)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.password_field).to_have_value("")


@then(parsers.parse('the deletion dialog shows "{prefix}" followed by a reason naming orders'))
def dialog_blocker(world: World, prefix: str) -> None:
    page = _page(world)
    expect(page.dialog.get_by_text(prefix.strip(), exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    text = page.dialog.inner_text()
    reason = text.split(prefix.strip(), 1)[1]
    assert ad.names_category(reason, "open orders"), text


@then("no pending deletion panel is shown")
def no_panel(world: World) -> None:
    expect(_page(world).cancel_button).to_have_count(0)


@then("no deletion is pending for the buyer")
def not_pending(world: World) -> None:
    assert not ad.is_pending(_user(world)["token"])


@then("the buyer account is still usable")
def still_usable(world: World) -> None:
    assert ue.login_raw(_user(world)["username"], _user(world)["password"]).status_code == 200


@then(parsers.parse('the deletion offer "{label}" is not shown'))
def offer_hidden(world: World, label: str) -> None:
    expect(_page(world).delete_offer).to_have_count(0)


@then(parsers.parse('the deletion offer "{label}" is shown again'))
def offer_shown(world: World, label: str) -> None:
    expect(_page(world).delete_offer).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer cancels the deletion request and confirms")
def cancel_deletion(world: World) -> None:
    page = _page(world)
    _x(world)["since"] = lu.mark()
    page.cancel_button.click()
    if page.dialog.count():
        page.dialog.get_by_role("button", name=re.compile(r"^(Xác nhận|Hủy yêu cầu xóa)")).click()


@then(parsers.parse("the gateway received {n:d} CancelAccountDeletion call from the privacy buyer"))
def cancel_calls(world: World, n: int) -> None:
    assert _calls("PrivacyService/CancelAccountDeletion", _x(world)["since"]) == n


# ── expired session ──────────────────────────────────────────────────────────
@when("the buyer's session token becomes invalid in the browser")
def invalidate_session(world: World) -> None:
    token = _user(world)["token"]
    broken = token[:-6] + ("AAAAAA" if not token.endswith("AAAAAA") else "BBBBBB")
    world.context.add_cookies([{"name": SESSION_COOKIE, "value": broken, "url": _base(world)}])


@then(parsers.parse('the privacy page shows "{text}"'))
def page_shows(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
