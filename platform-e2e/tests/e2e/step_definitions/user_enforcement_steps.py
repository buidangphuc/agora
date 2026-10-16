"""Steps for frontend/user_enforcement_ui.feature (frontend-user-enforcement, initiative q6-trust-enforcement).

Fresh accounts per scenario (never a shared seeded user): registered through the gateway
(user_enforcement_support, owned by identity-user-enforcement, imported only). Every assertion is
on the rendered page, plus the stored status read back through the gateway.
"""

from __future__ import annotations

import re
from datetime import datetime, timedelta, timezone
from urllib.parse import quote

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.pages.admin_users_page import FORBIDDEN_TITLE, AdminUsersPage
from src.pages.login_page import LoginPage
from tests.e2e.support import audit_support as aud
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

REFUSAL = "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ."
CREDENTIALS = "Tên đăng nhập hoặc mật khẩu không chính xác."
SUCCESS = re.compile(r"Đã cập nhật trạng thái; đã đăng xuất \d+ phiên\.")
VN = timezone(timedelta(hours=7))


def _x(world: World) -> dict:
    return world.state.extra.setdefault("ueui", {})


def _users(world: World) -> AdminUsersPage:
    return AdminUsersPage(world.page)


def _login(world: World) -> LoginPage:
    return LoginPage(world.page)


def _status_cell(row):
    """The 'Trạng thái' td (4th column). The row also embeds the status <select> whose options list
    every label, so whole-row text matching would pass before the update lands."""
    return row.locator("td").nth(3)


def _subject(world: World) -> dict:
    x = _x(world)
    return x.get("buyer") or x["seller"]


# ── Givens ───────────────────────────────────────────────────────────────
@given("a freshly registered buyer for the users page")
def fresh_buyer(world: World) -> None:
    _x(world)["buyer"] = ue.register_user("buyer", "ueui_buyer")


@given("a freshly registered seller for the users page")
def fresh_seller(world: World) -> None:
    _x(world)["seller"] = ue.register_user("seller", "ueui_seller")


@given(parsers.parse('an admin has set that {who} to "{status}"'))
def admin_sets(world: World, who: str, status: str) -> None:
    user = _x(world)[who]
    resp = ue.set_user_status(
        world,
        user["id"],
        status,
        reason="abuse",
        until=ue.in_future(3600) if status == "suspended" else None,
    )
    assert resp.status_code == 200, f"setup SetUserStatus failed: {resp.status_code} {resp.text[:200]}"
    row = ue.stored_user(world, user["id"])
    assert row.get("status") == f"USER_STATUS_{status.upper()}", row


@given("team-audit is down for this scenario")
def audit_down(world: World) -> None:
    aud.stop_audit(world)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the visitor opens the users page")
@when("the admin opens the users page")
def open_users(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/admin/users", wait_until="domcontentloaded")


@when("the admin searches for that buyer's username")
def search_buyer(world: World) -> None:
    _users(world).search(_x(world)["buyer"]["username"])
    world.page.wait_for_url(re.compile(r"query="), timeout=timeouts.NAVIGATION)


@when("the admin searches for their own username")
def search_self(world: World) -> None:
    me = world.state.current_user.username
    _x(world)["admin_username"] = me
    _users(world).search(me)
    world.page.wait_for_url(re.compile(r"query="), timeout=timeouts.NAVIGATION)


@when(parsers.parse('the admin submits "{status}" with reason "{reason}" and an end time one hour ahead'))
def submit_with_until(world: World, status: str, reason: str) -> None:
    until = (datetime.now(VN) + timedelta(hours=1)).replace(second=0, microsecond=0)
    _x(world)["until_text"] = until.strftime("%Y-%m-%d %H:%M")
    _users(world).submit_status(
        _x(world)["buyer"]["username"], status, reason, until.strftime("%Y-%m-%dT%H:%M")
    )


@when(parsers.parse('the admin submits "{status}" with reason "{reason}" and no end time'))
def submit_no_until(world: World, status: str, reason: str) -> None:
    _users(world).submit_status(_x(world)["buyer"]["username"], status, reason)


def _browser_login(world: World, password: str) -> None:
    user = _subject(world)
    world.context.clear_cookies()
    page = _login(world)
    page.navigate()
    page.login(user["username"], password)


@when("the buyer submits the correct password on the login page")
def login_correct(world: World) -> None:
    _browser_login(world, _x(world)["buyer"]["password"])


@when("the buyer submits a wrong password on the login page")
def login_wrong(world: World) -> None:
    _browser_login(world, "wrong-" + _x(world)["buyer"]["password"])


@when(parsers.parse('the seller logs in on the login page and opens "{path}"'))
def seller_login_open(world: World, path: str) -> None:
    _browser_login(world, _x(world)["seller"]["password"])
    # Wait until the login submit settles: either off /login or an error shown.
    world.page.wait_for_url(lambda u: "/login" not in u, timeout=timeouts.NAVIGATION)
    world.page.goto(f"{world.settings.base_url}{path}", wait_until="domcontentloaded")


# ── Thens ────────────────────────────────────────────────────────────────
@then("the browser lands on the login page with a returnUrl for the users page")
def login_with_return(world: World) -> None:
    world.page.wait_for_url(
        re.compile(r"/login\?.*returnUrl=" + re.escape(quote("/admin/users", safe="")) + r"(&|$)"),
        timeout=timeouts.NAVIGATION,
    )


@then(parsers.parse('the users page shows "{text}" and no user table'))
def users_forbidden(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_users(world).table).to_have_count(0)


@then("the users table is shown")
def users_table(world: World) -> None:
    expect(_users(world).heading).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(FORBIDDEN_TITLE)).to_have_count(0)


@then(parsers.parse('the table shows that buyer with status "{label}"'))
def buyer_listed(world: World, label: str) -> None:
    row = _users(world).row_of(_x(world)["buyer"]["username"])
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(_status_cell(row)).to_have_text(label, timeout=timeouts.NAVIGATION)
    expect(row).to_contain_text(_x(world)["buyer"]["id"])


@then("the users page shows the success message")
def success_message(world: World) -> None:
    expect(_users(world).flash).to_contain_text(SUCCESS, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the buyer\'s row reads "{label}" with that end time'))
def row_reads_with_until(world: World, label: str) -> None:
    row = _users(world).row_of(_x(world)["buyer"]["username"])
    expect(_status_cell(row)).to_have_text(label, timeout=timeouts.NAVIGATION)
    expect(row).to_contain_text(_x(world)["until_text"], timeout=timeouts.NAVIGATION)


@then(parsers.parse('the buyer\'s row reads "{label}"'))
def row_reads(world: World, label: str) -> None:
    row = _users(world).row_of(_x(world)["buyer"]["username"])
    expect(_status_cell(row)).to_have_text(label, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the gateway reports the buyer as "{status}"'))
def gateway_status(world: World, status: str) -> None:
    row = ue.stored_user(world, _x(world)["buyer"]["id"])
    assert row.get("status") == f"USER_STATUS_{status.upper()}", row


@then("the buyer can still log in through the gateway")
def buyer_can_login(world: World) -> None:
    resp = ue.login_raw(_x(world)["buyer"]["username"], _x(world)["buyer"]["password"])
    assert resp.status_code == 200, f"login refused: {resp.status_code} {resp.text[:200]}"


@then(parsers.parse('the users page shows "{text}"'))
def users_page_shows(world: World, text: str) -> None:
    expect(_users(world).flash).to_contain_text(text, timeout=timeouts.NAVIGATION)


@then("the buyer's row has a status form")
def buyer_row_has_form(world: World) -> None:
    form = _users(world).status_form_of(_x(world)["buyer"]["username"])
    expect(form).to_have_count(1, timeout=timeouts.NAVIGATION)


@then("the admin's own row has no status form")
def own_row_no_form(world: World) -> None:
    me = _x(world)["admin_username"]
    row = _users(world).row_of(me)
    expect(row.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_users(world).status_form_of(me)).to_have_count(0)


@then("the login page shows the fixed refusal text")
def shows_refusal(world: World) -> None:
    expect(world.page.get_by_text(REFUSAL)).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the browser is still on the login page")
def still_login(world: World) -> None:
    assert "/login" in world.page.url, world.page.url


@then(parsers.parse('the login page shows "{text}" and not the refusal text'))
def shows_credentials_only(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(REFUSAL)).to_have_count(0)


@then("the browser leaves the login page")
def left_login(world: World) -> None:
    world.page.wait_for_url(lambda u: "/login" not in u, timeout=timeouts.NAVIGATION)


@then("the header shows the signed-in state")
def header_signed_in(world: World) -> None:
    expect(world.page.get_by_role("button", name="Đăng xuất")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the browser is still on "{path}"'))
def still_on(world: World, path: str) -> None:
    assert world.page.url == f"{get_settings().base_url.rstrip('/')}{path}", world.page.url


@then(parsers.parse('the seller notice shows "{title}" and "{line}"'))
def seller_notice(world: World, title: str, line: str) -> None:
    expect(world.page.get_by_text(title).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(line)).to_be_visible(timeout=timeouts.NAVIGATION)
