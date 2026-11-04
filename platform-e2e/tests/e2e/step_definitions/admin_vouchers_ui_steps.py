"""Steps for frontend/admin_vouchers_ui.feature (change promotion-admin-vouchers).

Vouchers that are only set-up are created through the gateway; everything the change promises is
asserted on the rendered admin pages (and the buyer checkout for the stopped-voucher text), plus the
stored state read back through the gateway. Fresh accounts per scenario.
"""

from __future__ import annotations

import re
from datetime import datetime, timedelta, timezone

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.models import User
from src.pages import CheckoutPage
from src.pages.admin_vouchers_page import (
    AdminVouchersPage,
)
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.promo_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.promotion_admin_vouchers_steps import *  # noqa: F401,F403
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support.world import World

STOPPED_TEXT = "Mã giảm giá đã ngừng áp dụng."
_IMMUTABLE = re.compile(
    r"cannot be (changed|edited|modified|updated)|can't be (changed|edited|modified)|"
    r"immutable|read.?only after|không thể (sửa|thay đổi|chỉnh sửa|cập nhật)",
    re.I,
)


def _st(world: World) -> dict:
    return av.state(world)


def _page(world: World) -> AdminVouchersPage:
    return AdminVouchersPage(world.page)


def _requests(world: World) -> list[str]:
    st = _st(world)
    if "update_requests" not in st:
        st["update_requests"] = []
        world.page.on(
            "request",
            lambda r: (
                st["update_requests"].append(r.url)
                if "VoucherService/UpdateVoucher" in r.url
                else None
            ),
        )
    return st["update_requests"]


def _date_forms(moment: datetime) -> list[str]:
    forms: list[str] = []
    for m in (moment, moment.astimezone()):
        forms += [
            m.strftime("%Y-%m-%d"),
            m.strftime("%d/%m/%Y"),
            f"{m.day}/{m.month}/{m.year}",
            m.strftime("%b %d, %Y"),
            f"{m.strftime('%b')} {m.day}, {m.year}",
            f"{m.day} {m.strftime('%b')} {m.year}",
            m.strftime("%d-%m-%Y"),
        ]
    return forms


# ── Givens ───────────────────────────────────────────────────────────────
@given("an admin is signed in the admin app")
def admin_signed_in(world: World) -> None:
    from src.utils import get_test_data_manager

    login_via_api(world, get_test_data_manager().get_user_by_role("admin"))


@given("the admin has the create form filled with the code of that voucher")
def create_form_taken(world: World) -> None:
    st = _st(world)
    page = _page(world)
    page.goto(f"{page.app_base_url()}/admin/vouchers/new")
    page.fill_create_form(st["code"], 5, av.iso(days=-1), av.iso(days=30))
    st["ui_mode"] = "create"
    st["typed_code"] = st["code"]


@given("the admin has the create form filled with a fresh code")
def create_form_fresh(world: World) -> None:
    st = _st(world)
    st["new_code"] = av.fresh_code("AVUI")
    page = _page(world)
    page.goto(f"{page.app_base_url()}/admin/vouchers/new")
    page.fill_create_form(st["new_code"], 5, av.iso(days=-1), av.iso(days=30))
    st["ui_mode"] = "create"
    st["typed_code"] = st["new_code"]


@given("the admin has the stop dialog open on that voucher")
def stop_dialog_open(world: World) -> None:
    st = _st(world)
    page = _page(world)
    page.navigate()
    row = page.find_row(st["code"])
    assert row is not None, f"voucher {st['code']} is not listed"
    page.press(row, re.compile(r"^(stop|ngừng voucher)$", re.I))
    expect(page.dialog.first).to_be_visible(timeout=timeouts.DEFAULT)
    st["ui_mode"] = "stop"


@given("the admin opens the detail screen of that voucher")
def open_detail_given(world: World) -> None:
    _open_detail(world)


@when("the admin opens the detail screen of that voucher")
def open_detail_when(world: World) -> None:
    _open_detail(world)


def _open_detail(world: World) -> None:
    page = _page(world)
    page.navigate()
    page.open_detail(_st(world)["code"])


@given("the admin sees the voucher list with the Stop control")
def list_with_stop(world: World) -> None:
    st = _st(world)
    page = _page(world)
    page.navigate()
    row = page.find_row(st["code"])
    assert row is not None, "control: the admin does not see the voucher"
    expect(row.get_by_role("button", name=re.compile(r"^(stop|ngừng voucher)$", re.I)).first).to_be_visible(
        timeout=timeouts.DEFAULT
    )


# ── Whens ────────────────────────────────────────────────────────────────
@when("the admin submits the form")
def submit_form(world: World) -> None:
    st = _st(world)
    page = _page(world)
    if st.get("ui_mode") == "stop":
        page.press(page.dialog.first, re.compile(r"^(confirm|stop|yes|xác nhận|ngừng)", re.I))
    else:
        page.submit_form()


@when("the admin opens the create confirmation dialog")
def open_confirm_dialog(world: World) -> None:
    page = _page(world)
    page.press(page.page, re.compile(r"^(create|review|submit)", re.I))
    expect(page.dialog.first).to_be_visible(timeout=timeouts.DEFAULT)


@given(parsers.parse("the admin has the edit form filled with quota {quota:d}"))
@when(parsers.parse("the admin has the edit form filled with quota {quota:d}"))
def edit_form_filled(world: World, quota: int) -> None:
    st = _st(world)
    page = _page(world)
    page.navigate()
    page.open_detail(st["code"])
    page.press(page.page, re.compile(r"^(edit|sửa voucher)$", re.I))
    st["sent_end"] = av.iso(days=60)
    page.field("quota").first.fill(str(quota))
    page.fill_date("end", st["sent_end"])
    st["ui_mode"] = "edit"
    st["ui_quota"] = quota


@when("the admin opens the edit form")
def open_edit_form(world: World) -> None:
    page = _page(world)
    page.press(page.page, re.compile(r"^(edit|sửa voucher)$", re.I))
    _st(world)["ui_mode"] = "edit"
    _requests(world)


def _attempt(world: World, label: str, value: str) -> None:
    st = _st(world)
    page = _page(world)
    box = page.field(label).first
    box.fill(value)
    page.press(page.page, re.compile(r"^(save|submit|update)", re.I))
    invalid = box.get_attribute("aria-invalid") == "true"
    alert = page.page.get_by_role("alert").filter(has_text=re.compile(r".+")).count() > 0
    st.setdefault("attempts", []).append(invalid or alert)


@when(parsers.parse("the admin enters a quota of {quota:d} and submits"))
def enter_quota(world: World, quota: int) -> None:
    _attempt(world, "quota", str(quota))


@when("the admin enters an end date before the start date and submits")
def enter_bad_end(world: World) -> None:
    page = _page(world)
    page.fill_date("end", av.iso(days=-3))
    page.press(page.page, re.compile(r"^(save|submit|update)", re.I))
    box = page.field("end").first
    _st(world).setdefault("attempts", []).append(
        box.get_attribute("aria-invalid") == "true" or page.page.get_by_role("alert").count() > 0
    )


@when("the buyer applies the stopped code at checkout")
def buyer_applies_stopped(world: World) -> None:
    _apply_code(world, _st(world)["code"])


@when("the buyer applies an unknown code at checkout")
def buyer_applies_unknown(world: World) -> None:
    _apply_code(world, av.fresh_code("NOPE"))


def _apply_code(world: World, code: str) -> None:
    world.navigate_to(PageName.CHECKOUT)
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.stepper).to_be_visible(timeout=timeouts.NAVIGATION)
    checkout.continue_to("payment")
    expect(checkout.voucher_input).to_be_visible(timeout=timeouts.DEFAULT)
    checkout.voucher_input.fill(code)
    checkout.apply_voucher_button.click()


@when("a signed-in seller opens the vouchers page")
def seller_opens(world: World) -> None:
    world.context.clear_cookies()
    seller = User(
        username=fake.unique_username("av_seller"),
        password=world.settings.seed_password,
        role="seller",
    )
    login_via_api(world, seller)
    _page(world).navigate()


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('the form shows "{message}" and keeps the typed code'))
def form_shows(world: World, message: str) -> None:
    page = _page(world)
    expect(page.text(message).first).to_be_visible(timeout=timeouts.DEFAULT)
    assert page.field(r"^(code|mã voucher)").first.input_value() == _st(world)["typed_code"]


@then(parsers.parse('the admin app shows "{message}"'))
def app_shows(world: World, message: str) -> None:
    expect(_page(world).text(message).first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the admin app does not show "{message}"'))
def app_not_shows(world: World, message: str) -> None:
    page = _page(world)
    expect(page.text(message)).to_have_count(0)


@then("the voucher list holds exactly one row for that code")
def one_row(world: World) -> None:
    st = _st(world)
    page = _page(world)
    page.navigate()
    assert page.count_rows(st["code"]) == 1, "control: the original row must be listed exactly once"


@then(parsers.parse('after a refresh the voucher row shows status "{status}"'))
def row_status(world: World, status: str) -> None:
    st = _st(world)
    page = _page(world)
    page.navigate()
    row = page.find_row(st["code"])
    assert row is not None, f"voucher {st['code']} is not listed"
    expect(row).to_contain_text(re.compile(status, re.I), timeout=timeouts.DEFAULT)


@then(parsers.parse('the checkout voucher alert reads "{text}" and no raw reason'))
def alert_reads(world: World, text: str) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.voucher_error).to_be_visible(timeout=timeouts.DEFAULT)
    got = checkout.voucher_error.inner_text().strip()
    assert text in got, f"alert {got!r} does not read {text!r}"
    assert "inactive" not in got.lower(), f"raw reason leaked: {got!r}"


@then('the checkout voucher alert is shown, is not the stopped text and holds no raw "inactive"')
def alert_other(world: World) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.voucher_error).to_be_visible(timeout=timeouts.DEFAULT)
    got = checkout.voucher_error.inner_text().strip()
    assert got and STOPPED_TEXT not in got and "inactive" not in got.lower(), got


@then("the detail screen shows quota 8 and the new end date")
def detail_shows(world: World) -> None:
    st = _st(world)
    page = _page(world)
    stored = av.get_voucher(st["admin"], st["code"]) or {}
    assert str(stored.get("quota")) == "8", f"stored voucher not updated: {stored}"
    body = page.page.locator("body")
    expect(body).to_contain_text(re.compile(r"\b8\b"), timeout=timeouts.DEFAULT)
    ends = datetime.now(timezone.utc) + timedelta(days=60)
    text = body.inner_text()
    assert any(
        f in text for f in _date_forms(ends)
    ), f"new end date not shown; forms {_date_forms(ends)[:3]}"


@then("the discount, min spend, max discount and scope are read-only")
def money_readonly(world: World) -> None:
    page = _page(world)
    shown = 0
    for label in ("discount", "min.?spend", "max.?discount", "scope"):
        boxes = page.field(label)
        for i in range(boxes.count()):
            assert not boxes.nth(i).is_editable(), f"{label} field is editable"
        shown += 1 if page.page.get_by_text(re.compile(label, re.I)).count() else 0
    assert shown == 4, "the money fields are not displayed on the edit screen"


@then("the end date and quota fields are editable")
def date_quota_editable(world: World) -> None:
    page = _page(world)
    expect(page.field("quota").first).to_be_editable(timeout=timeouts.DEFAULT)
    expect(page.field("end").first).to_be_editable(timeout=timeouts.DEFAULT)


@then("the end date field is editable")
def date_editable(world: World) -> None:
    expect(_page(world).field("end").first).to_be_editable(timeout=timeouts.DEFAULT)


@then("every attempt shows an error on its field")
def attempts_error(world: World) -> None:
    attempts = _st(world).get("attempts", [])
    assert len(attempts) == 3 and all(attempts), attempts


@then("no UpdateVoucher request was sent")
def no_update_request(world: World) -> None:
    assert _requests(world) == [], _requests(world)


@then("the form renders no quota field")
def no_quota_field(world: World) -> None:
    page = _page(world)
    expect(page.field("end").first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.field("quota")).to_have_count(0)


@then(parsers.parse('the voucher shows status "{status}"'))
def shows_status(world: World, status: str) -> None:
    pattern = {"stopped": r"stopped|Đã ngừng"}.get(status, status)
    expect(_page(world).page.locator("body")).to_contain_text(
        re.compile(pattern, re.I), timeout=timeouts.DEFAULT
    )


@then("the Stop and Edit controls are absent")
def no_controls(world: World) -> None:
    page = _page(world)
    expect(page.page.get_by_text(_st(world)["code"]).first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.stop_controls).to_have_count(0)
    expect(page.edit_controls).to_have_count(0)


@then("the page shows the forbidden screen with no Stop or Edit control")
def forbidden_screen(world: World) -> None:
    page = _page(world)
    expect(page.forbidden.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.stop_controls).to_have_count(0)
    expect(page.edit_controls).to_have_count(0)


@then("the dialog has text and does not state that a voucher cannot be changed")
def dialog_text(world: World) -> None:
    page = _page(world)
    text = page.dialog.first.inner_text()
    assert text.strip(), "the confirmation dialog is empty"
    assert not _IMMUTABLE.search(text), f"dialog claims immutability: {text!r}"
