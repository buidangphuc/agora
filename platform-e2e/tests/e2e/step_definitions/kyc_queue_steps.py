"""Admin KYC queue (change admin-kyc-queue): /admin/cases?tab=kyc in the browser.

Submissions are created through the real RPCs (support/kyc_documents_support); outcomes are read back
through ListKycSubmissions, GetVerificationStatus and QueryAuditLog. Sellers are named per scenario.
"""

from __future__ import annotations

import re
import time
from datetime import timedelta
from urllib.parse import parse_qs, urljoin, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services.verification_service import VerificationService
from src.constants import timeouts
from src.pages.admin_kyc_queue_page import EMPTY_TEXT, AdminKycQueuePage
from tests.e2e.flows.auth_flow import session_cookies, sign_in_admin_app
from tests.e2e.support import audit_support as aud
from tests.e2e.support import kyc_documents_support as kyc
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

TARGET = "kyc_submission"
VIEW_ACTION = "kyc.document_view"
PENDING = "VERIFICATION_STATUS_PENDING"
VERIFIED = "VERIFICATION_STATUS_VERIFIED"
REJECTED = "VERIFICATION_STATUS_REJECTED"
UTC7 = timedelta(hours=7)
TIME_RE = re.compile(r"\d{2}:\d{2}(?::\d{2})?")
PRESIGNED_MARKERS = ("X-Amz-Signature", "X-Amz-Credential", "X-Amz-Expires")


def _x(world: World) -> dict:
    return world.state.extra.setdefault("kq", {})


def _sellers(world: World) -> dict:
    return _x(world).setdefault("sellers", {})


def _seller(world: World, name: str) -> dict:
    sellers = _sellers(world)
    if name not in sellers:
        token, user_id = kyc.register(world, "seller")
        sellers[name] = {
            "token": token,
            "id": user_id,
            "svc": VerificationService(token=token),
            "subs": [],
        }
    return sellers[name]


def _submit(world: World, name: str, docs: int = 1) -> str:
    seller = _seller(world, name)
    sub_id, _, _ = kyc.submit_with_uploads(seller["svc"], docs)
    seller["subs"].append(sub_id)
    return sub_id


def _sub(world: World, name: str) -> str:
    return _seller(world, name)["subs"][-1]


def _admin_token(world: World) -> str:
    token = _x(world).get("admin_token")
    if not token:
        token = aud.admin_token(world)
        _x(world)["admin_token"] = token
    return token


def _admin_id(world: World) -> str:
    return kyc.jwt_sub(_admin_token(world))


def _page(world: World) -> AdminKycQueuePage:
    return AdminKycQueuePage(world.page)


def _fresh_cookies(world: World) -> None:
    world.context.clear_cookies()
    world.context.add_cookies(
        [
            {
                "name": "agora_consent",
                "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
                "url": world.settings.base_url,
            },
            {
                "name": "agora_consent",
                "value": f"v=1&a=1&m=0&p=0&ts={int(time.time())}",
                "url": world.settings.admin_base_url,
            },
        ]
    )


def _sign_in(world: World, token: str) -> None:
    _fresh_cookies(world)
    world.context.add_cookies(session_cookies(world.settings, token))
    sign_in_admin_app(world, token)


def _list_all(token: str, status: str) -> list[dict]:
    if not status.startswith("VERIFICATION_STATUS_"):
        status = f"VERIFICATION_STATUS_{status}"
    svc = VerificationService(token=token)
    out: list[dict] = []
    cursor: str | None = None
    for _ in range(500):
        resp = svc.list_kyc_submissions_response(status, 100, cursor)
        assert resp.status_code == 200, f"ListKycSubmissions failed: {resp.status_code} {resp.text}"
        data = resp.json()
        out.extend(data.get("submissions", []))
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            return out
    raise AssertionError("ListKycSubmissions paging did not terminate")


def _find(token: str, status: str, sub_id: str) -> dict | None:
    return next((s for s in _list_all(token, status) if s.get("id") == sub_id), None)


def _bkk(value: str) -> str:
    return (kyc.parse_ts(value) + UTC7).strftime("%Y-%m-%d %H:%M:%S")


def _row(world: World, name: str):
    return _page(world).row(_sub(world, name))


def _own_row(world: World):
    return _page(world).row(_x(world)["own_sub"])


# ── Givens ────────────────────────────────────────────────────────────────


@given(parsers.parse('seller "{name}" has a pending KYC submission'))
def seller_pending(world: World, name: str) -> None:
    _submit(world, name, 1)


@given(parsers.parse('seller "{name}" has a pending KYC submission with {n:d} documents'))
def seller_pending_docs(world: World, name: str, n: int) -> None:
    _submit(world, name, n)


@given("the PENDING KYC queue is empty")
def queue_empty(world: World) -> None:
    clearer = ue.register_second_admin()
    for _ in range(50):
        pending = _list_all(clearer["token"], "PENDING")
        if not pending:
            return
        for sub in pending:
            svc = VerificationService(token=clearer["token"])
            resp = svc.review_kyc_response(sub["id"], "reject", "other")
            assert resp.status_code == 200, f"draining the queue failed: {resp.text}"
    raise AssertionError("the PENDING KYC queue did not drain")


@given(parsers.parse('sellers "{a}", "{b}" and "{c}" each submit KYC one after another'))
def sellers_submit_in_order(world: World, a: str, b: str, c: str) -> None:
    for name in (a, b, c):
        _submit(world, name, 1)
        time.sleep(1.1)


@given(parsers.parse("{n:d} sellers each have a pending KYC submission"))
def many_pending(world: World, n: int) -> None:
    for i in range(n):
        _submit(world, f"Q{i}", 1)


@given(
    parsers.parse(
        'the seeded admin rejects the submission of seller "{name}" with reason code "{reason}"'
    )
)
def admin_rejects(world: World, name: str, reason: str) -> None:
    resp = VerificationService(token=_admin_token(world)).review_kyc_response(
        _sub(world, name), "reject", reason
    )
    assert resp.status_code == 200, resp.text


@given("no admin session is open in the browser")
def no_session(world: World) -> None:
    _fresh_cookies(world)


@given(parsers.parse('seller "{name}" is signed in the browser'))
def seller_signed_in(world: World, name: str) -> None:
    _sign_in(world, _seller(world, name)["token"])


@given("the seeded admin is signed in the browser")
@when("the seeded admin is signed in the browser")
def admin_signed_in(world: World) -> None:
    _sign_in(world, _admin_token(world))


@given("a second admin")
def second_admin(world: World) -> None:
    _x(world)["admin2"] = ue.register_second_admin()


@given("a second admin who has a pending KYC submission is signed in the browser")
def second_admin_with_pending(world: World) -> None:
    admin2 = ue.register_second_admin()
    _x(world)["admin2"] = admin2
    sub_id, _, _ = kyc.submit_with_uploads(VerificationService(token=admin2["token"]), 1)
    _x(world)["own_sub"] = sub_id
    _x(world)["own_id"] = admin2["id"]

    def reject_leftover() -> None:
        VerificationService(token=_admin_token(world)).review_kyc_response(
            sub_id, "reject", "other"
        )

    world.add_cleanup(reject_leftover)
    _sign_in(world, admin2["token"])


@given("team-audit is down for the KYC queue")
@when("team-audit is down for the KYC queue")
def audit_down(world: World) -> None:
    aud.stop_audit(world)


# ── Whens ─────────────────────────────────────────────────────────────────


@when("the visitor opens the KYC tab")
def open_tab(world: World) -> None:
    _page(world).open_kyc()


@when(parsers.parse('the visitor opens the KYC tab with status "{status}"'))
def open_tab_status(world: World, status: str) -> None:
    _page(world).open_kyc(status=status)


@when(parsers.parse('the visitor opens "{path}"'))
def open_path(world: World, path: str) -> None:
    world.page.goto(
        f"{world.settings.admin_base_url.rstrip('/')}{path}", wait_until="domcontentloaded"
    )


@when(parsers.parse('the visitor follows "{label}"'))
def follow_link(world: World, label: str) -> None:
    world.page.get_by_role("link", name=label).click()
    world.page.wait_for_load_state("domcontentloaded")


@when(parsers.parse('the admin clicks "{label}" on the row of seller "{name}"'))
def click_on_row(world: World, label: str, name: str) -> None:
    x = _x(world)
    row = _row(world, name)
    button = row.get_by_role("button", name=label, exact=True)
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    x["clicked_at"] = time.time()
    button.click()
    if label.startswith("Xem tài liệu"):
        world.page.wait_for_load_state("domcontentloaded")


@when('the admin clicks "Duyệt" on the row of their own submission')
def click_own_approve(world: World) -> None:
    button = _page(world).approve_button(_own_row(world))
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    button.click()


@when(parsers.parse('the admin picks the reason "{label}" and confirms'))
def pick_and_confirm(world: World, label: str) -> None:
    page = _page(world)
    expect(page.dialog).to_be_visible(timeout=timeouts.DEFAULT)
    page.pick_reason(label)
    page.dialog.get_by_role("button", name=re.compile(r"^(Xác nhận|Từ chối|Gửi)$")).click()


@when("the admin confirms the reject dialog without a reason")
def confirm_without_reason(world: World) -> None:
    page = _page(world)
    expect(page.dialog).to_be_visible(timeout=timeouts.DEFAULT)
    page.dialog.get_by_role("button", name=re.compile(r"^(Xác nhận|Từ chối|Gửi)$")).click()


@when(
    parsers.parse('the second admin approves the submission of seller "{name}" through the gateway')
)
def second_admin_approves(world: World, name: str) -> None:
    svc = VerificationService(token=_x(world)["admin2"]["token"])
    resp = svc.review_kyc_response(_sub(world, name), "approve")
    assert resp.status_code == 200, resp.text


@when("team-audit is up again for the KYC queue")
def audit_up(world: World) -> None:
    aud.start_audit(world)


# ── Thens ─────────────────────────────────────────────────────────────────


@then("the browser is sent to login with a return URL for the KYC tab")
def sent_to_login(world: World) -> None:
    world.page.wait_for_url(re.compile(r"/login"), timeout=timeouts.NAVIGATION)
    query = parse_qs(urlsplit(world.page.url).query)
    target = (query.get("returnUrl") or query.get("next") or [""])[0]
    assert urlsplit(target).path == "/admin/cases", world.page.url


@then("the KYC table is not shown")
def table_not_shown(world: World) -> None:
    expect(_page(world).table).to_have_count(0)


@then(parsers.parse('the 403 result "{title}" is shown'))
def forbidden_shown(world: World, title: str) -> None:
    expect(world.page.get_by_text(title)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the submission of seller "{name}" is listed'))
def listed(world: World, name: str) -> None:
    expect(_row(world, name)).to_have_count(1, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the submission of seller "{name}" is not listed'))
def not_listed(world: World, name: str) -> None:
    expect(_page(world).table.or_(world.page.get_by_text("Không có hồ sơ")).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(_row(world, name)).to_have_count(0)


@then(parsers.parse('the submission of seller "{name}" is no longer listed as pending'))
def no_longer_pending(world: World, name: str) -> None:
    sub_id = _sub(world, name)
    world.page.reload(wait_until="domcontentloaded")
    expect(_page(world).table.or_(world.page.get_by_text("Không có hồ sơ")).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(_page(world).row(sub_id)).to_have_count(0, timeout=timeouts.DEFAULT)


@then(parsers.parse('the tab strip shows "{a}", "{b}" and "{c}"'))
def tab_strip(world: World, a: str, b: str, c: str) -> None:
    for label in (a, b, c):
        expect(world.page.get_by_role("link", name=label).first).to_be_visible(
            timeout=timeouts.NAVIGATION
        )


@then(parsers.parse('the tab "{label}" links to "{href}"'))
def tab_href(world: World, label: str, href: str) -> None:
    expect(world.page.get_by_role("link", name=label).first).to_have_attribute(
        "href", re.compile(re.escape(href) + "$")
    )


@then(parsers.parse('the table shows 3 rows in the order of sellers "{a}", "{b}", "{c}"'))
def rows_in_order(world: World, a: str, b: str, c: str) -> None:
    page = _page(world)
    expect(page.body_rows).to_have_count(3, timeout=timeouts.NAVIGATION)
    texts = page.body_rows.all_inner_texts()
    ids = [_sub(world, n) for n in (a, b, c)]
    positions = [next(i for i, t in enumerate(texts) if sid in t) for sid in ids]
    assert positions == sorted(positions) and len(set(positions)) == 3, texts


@then("each row shows the seller user id, its document count and its creation time in UTC+07:00")
def row_columns(world: World) -> None:
    token = _admin_token(world)
    for name in _sellers(world):
        row = _row(world, name)
        text = row.inner_text()
        found = _find(token, "PENDING", _sub(world, name))
        assert found is not None, name
        assert _seller(world, name)["id"] in text, text
        assert found["userId"] == _seller(world, name)["id"]
        assert str(found.get("documentCount", 0)) in text, text
        assert _bkk(found["createdAt"]) in text, (_bkk(found["createdAt"]), text)


@then(parsers.parse('the table shows {n:d} rows and a "{label}" link'))
def rows_and_next(world: World, n: int, label: str) -> None:
    page = _page(world)
    expect(page.body_rows).to_have_count(n, timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name=label)).to_have_count(1)


@then(parsers.parse('the table shows {n:d} rows and no "{label}" link'))
def rows_and_no_next(world: World, n: int, label: str) -> None:
    page = _page(world)
    expect(page.body_rows).to_have_count(n, timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name=label)).to_have_count(0)


@then(parsers.parse("the table shows {n:d} rows"))
def rows_count(world: World, n: int) -> None:
    expect(_page(world).body_rows).to_have_count(n, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the empty message "{text}" is shown'))
def empty_shown(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the row of seller "{name}" shows "{a}" and "{b}"'))
def row_shows(world: World, name: str, a: str, b: str) -> None:
    page = _page(world)
    row = _row(world, name)
    page.table.or_(page.page.get_by_text(EMPTY_TEXT)).first.wait_for(timeout=timeouts.NAVIGATION)
    for _ in range(50):
        if row.count() or not page.next_link.count():
            break
        href = page.next_link.first.get_attribute("href") or ""
        page.page.goto(urljoin(page.page.url, href), wait_until="domcontentloaded")
        page.table.first.wait_for(timeout=timeouts.NAVIGATION)
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(row).to_contain_text(a)
    expect(row).to_contain_text(b)


@then(parsers.parse('the row of seller "{name}" has no "{a}" or "{b}" button'))
def row_no_buttons(world: World, name: str, a: str, b: str) -> None:
    row = _row(world, name)
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    for label in (a, b):
        expect(row.get_by_role("button", name=label, exact=True)).to_have_count(0)


@then(
    parsers.parse(
        'the row of seller "{name}" shows a "{label}" link that opens in a new tab with rel "{rel}"'
    )
)
def link_revealed(world: World, name: str, label: str, rel: str) -> None:
    link = _page(world).open_link(_row(world, name))
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(link).to_have_attribute("target", "_blank")
    expect(link).to_have_attribute("rel", rel)
    assert (link.get_attribute("href") or "").startswith("http")
    _x(world)["link_name"] = name


@then("the link expires at most 5 minutes after the click")
def link_expiry(world: World) -> None:
    x = _x(world)
    row = _row(world, x["link_name"])
    text = row.inner_text()
    sub = _find(_admin_token(world), "PENDING", _sub(world, x["link_name"]))
    created = _bkk(sub["createdAt"]) if sub else ""
    stripped = text.replace(created, "")
    shown = TIME_RE.findall(stripped)
    assert shown, f"no expiry time shown next to the link: {text!r}"
    hh, mm = (int(p) for p in shown[-1].split(":")[:2])
    clicked = time.gmtime(x["clicked_at"] + UTC7.total_seconds())
    delta = ((hh * 60 + mm) - (clicked.tm_hour * 60 + clicked.tm_min)) % (24 * 60)
    assert delta <= 6, f"expiry {shown[-1]} is {delta} min after the click: {text!r}"


@then(
    parsers.parse(
        'team-audit holds {n:d} kyc.document_view events for the submission of seller "{name}" by the seeded admin'
    )
)
def audit_views(world: World, n: int, name: str) -> None:
    sub_id = _sub(world, name)
    admin_id = _admin_id(world)
    deadline = time.monotonic() + 20
    events: list[dict] = []
    while time.monotonic() < deadline:
        events = [
            e
            for e in aud.events_for(world, TARGET, sub_id, action=VIEW_ACTION)
            if e.get("action") == VIEW_ACTION and e.get("actorId") == admin_id
        ]
        if len(events) == n and n > 0:
            return
        time.sleep(1)
    assert len(events) == n, f"expected {n} {VIEW_ACTION} events for {sub_id}, got {events}"


@then("the rendered page contains no presigned document URL")
def no_presigned(world: World) -> None:
    html = world.page.content()
    for marker in PRESIGNED_MARKERS:
        assert marker not in html, f"{marker} found in the rendered page"
    assert kyc.BUCKET not in re.sub(
        r"[^A-Za-z0-9/_.:-]", "", "".join(re.findall(r'href="[^"]*"', html))
    )


@then(parsers.parse('the row of seller "{name}" shows the alert "{text}"'))
def row_alert(world: World, name: str, text: str) -> None:
    expect(_row(world, name)).to_contain_text(text, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the row of seller "{name}" has no "{label}" link'))
def row_no_link(world: World, name: str, label: str) -> None:
    expect(_row(world, name).get_by_role("link", name=label)).to_have_count(0)


@then(
    parsers.parse(
        'the row of seller "{name}" has exactly {n:d} document buttons named "{first}" to "{last}"'
    )
)
def doc_buttons(world: World, name: str, n: int, first: str, last: str) -> None:
    page = _page(world)
    row = _row(world, name)
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(page.view_buttons(row)).to_have_count(n)
    for i in range(1, n + 1):
        expect(page.view_button(row, i)).to_have_count(1)
    assert first == "Xem tài liệu 1" and last == f"Xem tài liệu {n}"


@then(parsers.parse('the row of seller "{name}" has no button "{label}"'))
def row_no_button(world: World, name: str, label: str) -> None:
    expect(_row(world, name).get_by_role("button", name=label, exact=True)).to_have_count(0)


@then(parsers.parse('the toast "{text}" is shown'))
def toast_shown(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the dialog shows "{text}"'))
def dialog_shows(world: World, text: str) -> None:
    expect(_page(world).dialog.get_by_text(text)).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the verification status of seller "{name}" is "{status}"'))
def seller_status(world: World, name: str, status: str) -> None:
    seller = _seller(world, name)
    deadline = time.monotonic() + 20
    body: dict = {}
    while time.monotonic() < deadline:
        resp = seller["svc"].get_verification_status_response()
        assert resp.status_code == 200, resp.text
        body = resp.json()
        if body.get("status") == status:
            return
        time.sleep(1)
    assert body.get("status") == status, body


@then(
    parsers.parse(
        'the gateway lists the submission of seller "{name}" as rejected with reason code "{reason}"'
    )
)
def gateway_rejected(world: World, name: str, reason: str) -> None:
    found = _find(_admin_token(world), "REJECTED", _sub(world, name))
    assert found is not None, "submission is not in the REJECTED list"
    assert found.get("reasonCode") == reason, found
    assert found.get("status") == REJECTED, found


@then(parsers.parse('the gateway lists the submission of seller "{name}" as pending'))
def gateway_pending(world: World, name: str) -> None:
    assert _find(_admin_token(world), "PENDING", _sub(world, name)) is not None


@then(parsers.parse('the gateway lists the submission of seller "{name}" as verified'))
def gateway_verified(world: World, name: str) -> None:
    found = _find(_admin_token(world), "VERIFIED", _sub(world, name))
    assert found is not None and found.get("status") == VERIFIED, found


@then("the gateway lists the submission of the admin as pending")
def gateway_own_pending(world: World) -> None:
    assert _find(_admin_token(world), "PENDING", _x(world)["own_sub"]) is not None


@then(parsers.parse('the row of seller "{name}" still shows the "{label}" button after reload'))
def still_pending_row(world: World, name: str, label: str) -> None:
    world.page.reload(wait_until="domcontentloaded")
    expect(_row(world, name).get_by_role("button", name=label, exact=True)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
