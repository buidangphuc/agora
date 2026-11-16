"""Steps for the frontend-legal-pages features (legal_pages, legal_signup, legal_reacceptance,
consent_banner_ledger). Black box: HTTP and the browser for the shell, the gateway for the ledger
(PrivacyService.ListMyConsents) and the team-gateway log for "calls the gateway received".

Shared with identity-legal-consent: ledger paging, the identity_db helper and the
"consent table is temporarily unavailable" fault and changed-versions steps are imported from
legal_consent_steps, not copied.
"""

from __future__ import annotations

import re
import time
from urllib.parse import unquote, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.constants import legal_versions as legal
from src.pages import RegisterPage
from src.utils import data as fake
from tests.e2e.flows.auth_flow import SESSION_COOKIE, session_cookies
from tests.e2e.step_definitions.legal_consent_steps import (  # noqa: F401
    PERSONALIZATION,
    PRIVACY,
    TERMS,
    _all_records,
    _psql,
)
from tests.e2e.support import legal_ui_support as lu
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

ANALYTICS = "CONSENT_PURPOSE_ANALYTICS"
MARKETING = "CONSENT_PURPOSE_MARKETING"
PURPOSE_ENUM = {
    "TERMS": TERMS,
    "PRIVACY_POLICY": PRIVACY,
    "ANALYTICS": ANALYTICS,
    "MARKETING": MARKETING,
    "PERSONALIZATION": PERSONALIZATION,
}
CONSENT_COOKIE = "agora_consent"
STALE_MESSAGE = "Điều khoản đã được cập nhật. Vui lòng tải lại trang và đồng ý lại."
ENGLISH_BANNER_LABELS = (
    "Cookie preferences",
    "Cookie settings",
    "Analytics",
    "Marketing",
    "Personalization",
    "Personalisation",
    "Reject all",
    "Accept all",
    "Save choices",
    "Save preferences",
)


def _x(world: World) -> dict:
    return world.state.extra.setdefault("legal_ui", {})


def _base(world: World) -> str:
    return world.settings.base_url


def _nav(world: World, path: str) -> None:
    world.page.goto(_base(world) + path, wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)


def _idle(world: World) -> None:
    world.page.wait_for_load_state("networkidle", timeout=timeouts.NAVIGATION)


def _count(world: World, procedure: str) -> int:
    lu.settle_gateway_log()
    return lu.gateway_calls(procedure, _x(world)["since"])


def _start_window(world: World) -> None:
    _x(world)["since"] = lu.mark()


def _sign_in(world: World, user: dict) -> None:
    world.context.add_cookies(session_cookies(world.settings, user["token"]))
    _x(world)["user"] = user
    _x(world)["baseline_rows"] = len(_all_records(user["token"]))


def _reaccept_dialog(world: World):
    return (
        world.page.get_by_role("dialog")
        .filter(has=world.page.get_by_role("button", name="Đồng ý", exact=True))
        .filter(has=world.page.get_by_role("button", name="Đăng xuất", exact=True))
    )


def _banner(world: World):
    return world.page.get_by_role("dialog").filter(has_text="Tùy chọn cookie")


def _consent_cookie(world: World) -> str:
    for c in world.context.cookies(_base(world)):
        if c["name"] == CONSENT_COOKIE:
            return unquote(c["value"])
    return ""


# ── legal pages ──────────────────────────────────────────────────────────────
@when(parsers.parse('an anonymous visitor opens the legal page "{slug}"'))
def open_legal(world: World, slug: str) -> None:
    _x(world)["resp"] = lu.get(_base(world), "/" + slug.lstrip("/"))


@when("an anonymous visitor requests each of the seven legal slugs")
def request_all(world: World) -> None:
    _x(world)["resps"] = {s: lu.get(_base(world), "/" + s) for s in lu.SLUGS}


@then(
    'the response is 200 with a Vietnamese h1, a "Phiên bản" version of 1 to 32 characters and the legal placeholder body'
)
def page_content(world: World) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 200, resp.status_code
    h1 = lu.h1_of(resp.text)
    assert lu.is_vietnamese(h1), f"h1 is not Vietnamese: {h1!r}"
    version = lu.version_after_label(resp.text)
    assert 1 <= len(version) <= 32, f"version after 'Phiên bản': {version!r}"
    assert lu.PLACEHOLDER_BODY in lu.visible_text(resp.text)


@then("each response is 200")
def each_200(world: World) -> None:
    bad = {s: r.status_code for s, r in _x(world)["resps"].items() if r.status_code != 200}
    assert not bad, bad


@then("the sitemap contains exactly those 7 legal URLs")
def sitemap_exact(world: World) -> None:
    urls = lu.sitemap_legal_urls(_base(world))
    slugs = sorted(u.rstrip("/").rsplit("/", 1)[-1] for u in urls)
    assert slugs == sorted(lu.SLUGS), slugs


@then("the response is 404 and an HTML not-found page renders without a legal title")
def not_found(world: World) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 404, resp.status_code
    assert "<html" in resp.text.lower(), "404 without an HTML page"
    assert lu.PLACEHOLDER_BODY not in resp.text


@then(parsers.parse('the legal page "{slug}" still answers 200'))
def positive_control(world: World, slug: str) -> None:
    assert lu.get(_base(world), "/" + slug).status_code == 200


@then(parsers.parse('the response is 200 with no redirect to "{path}"'))
def no_redirect(world: World, path: str) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 200, (resp.status_code, resp.headers.get("location"))
    assert path not in resp.headers.get("location", "")


# ── footer ───────────────────────────────────────────────────────────────────
@when("an anonymous visitor opens the home page")
def open_home_anonymous(world: World) -> None:
    world.context.clear_cookies()
    _nav(world, "/")
    expect(world.page.locator("footer")).to_be_visible(timeout=timeouts.NAVIGATION)


def _footer_hrefs(world: World) -> list[str]:
    return [
        h
        for h in world.page.locator("footer a").evaluate_all(
            "els => els.map(e => e.getAttribute('href') || '')"
        )
        if h
    ]


def _path(href: str) -> str:
    return urlsplit(href).path.rstrip("/") or "/"


@then("the footer contains 7 links whose targets are the seven legal slugs")
def footer_links(world: World) -> None:
    targets = {_path(h) for h in _footer_hrefs(world)} & {"/" + s for s in lu.SLUGS}
    assert targets == {"/" + s for s in lu.SLUGS}, sorted(targets)


@then(parsers.parse('the footer RMA link target is "{path}"'))
def footer_rma(world: World, path: str) -> None:
    link = world.page.locator("footer").get_by_role("link", name=lu.RMA_LABEL)
    expect(link).to_have_count(1)
    assert _path(link.get_attribute("href") or "") == path


@then("the footer shows the four labelled operator fields each with the operator placeholder")
def footer_operator(world: World) -> None:
    text = lu.visible_text(world.page.locator("footer").inner_html())
    assert text.count(lu.OPERATOR_PLACEHOLDER) == 4, text.count(lu.OPERATOR_PLACEHOLDER)
    labels = {
        "company name": r"Tên (công ty|doanh nghiệp|đơn vị)|Công ty",
        "business registration number": r"(Mã số|Số) (doanh nghiệp|đăng ký kinh doanh|ĐKKD)|Giấy chứng nhận đăng ký",
        "address": r"Địa chỉ",
        "Ministry of Industry and Trade notice": r"Bộ Công Thương",
    }
    for name, pattern in labels.items():
        assert re.search(pattern, text, re.I), f"operator field label missing: {name}"


@then("every footer legal link resolves to a 200 page")
def footer_links_resolve(world: World) -> None:
    legal_paths = sorted({_path(h) for h in _footer_hrefs(world)} & {"/" + s for s in lu.SLUGS})
    assert legal_paths, "no footer legal link found"
    bad = {p: lu.get(_base(world), p).status_code for p in legal_paths}
    assert all(code == 200 for code in bad.values()), bad


@then(parsers.parse('no footer link for the RMA entry points to "{path}"'))
def rma_not_orders(world: World, path: str) -> None:
    link = world.page.locator("footer").get_by_role("link", name=lu.RMA_LABEL)
    expect(link).to_have_count(1)
    assert _path(link.get_attribute("href") or "") != path


# ── sign-up ──────────────────────────────────────────────────────────────────
def _fill_register(
    world: World, *, terms: bool = True, privacy: bool = True, submit: bool = True
) -> str:
    username = fake.unique_username("legal_ui")
    _x(world)["username"] = username
    _start_window(world)
    page: RegisterPage = world.get_page(PageName.REGISTER)  # type: ignore[assignment]
    page.register(
        username,
        get_settings().seed_password,
        "buyer",
        accept_terms=terms,
        accept_privacy=privacy,
        submit=submit,
    )
    return username


@then("the Terms and Privacy checkboxes are unchecked and link to their pages in a new tab")
def boxes_unchecked(world: World) -> None:
    page: RegisterPage = world.get_page(PageName.REGISTER)  # type: ignore[assignment]
    page.wait_until_interactive(page.submit_button)
    expect(page.terms_checkbox).not_to_be_checked()
    expect(page.privacy_checkbox).not_to_be_checked()
    form = world.page.locator("form")
    for slug in ("dieu-khoan-su-dung", "chinh-sach-bao-mat"):
        link = form.locator(f'a[href$="/{slug}"]')
        expect(link).to_have_count(1)
        assert link.get_attribute("target") == "_blank", slug


@when("the visitor submits the register form with both boxes checked")
def submit_both(world: World) -> None:
    _fill_register(world)


@when("the visitor submits the register form with the Terms box unchecked")
def submit_terms_unchecked(world: World) -> None:
    _fill_register(world, terms=False)


@when(
    "the register form is submitted with both boxes checked but the acceptance fields stripped from the post"
)
def submit_forged(world: World) -> None:
    def handler(route, request) -> None:  # noqa: ANN001
        if request.method == "POST" and "next-action" in request.headers:
            body = request.post_data_buffer or b""
            route.continue_(
                post_data=lu.strip_multipart_fields(body, ("acceptTerms", "acceptPrivacy"))
            )
            _x(world)["forged"] = True
        else:
            route.continue_()

    world.page.route("**/register*", handler)
    _fill_register(world)


@then(
    parsers.parse(
        'ListMyConsents for the new user returns the TERMS and PRIVACY_POLICY records at the current versions with source "{source}"'
    )
)
def two_rows(world: World, source: str) -> None:
    user = ue.login({"username": _x(world)["username"], "password": get_settings().seed_password})
    rows = _all_records(user["token"])
    assert len(rows) == 2, rows
    by = {r["purpose"]: r for r in rows}
    assert set(by) == {TERMS, PRIVACY}, rows
    assert (
        by[TERMS]["version"] == legal.CURRENT_TERMS
        and by[PRIVACY]["version"] == legal.CURRENT_PRIVACY
    ), rows
    assert all(r["source"] == source and r.get("granted") is True for r in rows), rows


@then('a field error is shown on the register form and the browser stays on "/register"')
def field_error(world: World) -> None:
    box = world.page.locator('form input[type="checkbox"][aria-invalid="true"]').first
    expect(box).to_be_visible(timeout=timeouts.NAVIGATION)
    error = world.page.locator(f"#{box.get_attribute('aria-describedby')}")
    expect(error).to_be_visible()
    assert error.inner_text().strip(), "field error has no text"
    assert urlsplit(world.page.url).path == "/register", world.page.url


@then(parsers.parse("the gateway received {n:d} Register calls"))
def register_calls(world: World, n: int) -> None:
    assert _count(world, lu.REGISTER_PROC) == n


@then("no account exists for the submitted username")
def no_account(world: World) -> None:
    resp = ue.login_raw(_x(world)["username"], get_settings().seed_password)
    assert resp.status_code != 200, f"account was created: {resp.text[:200]}"


@then(parsers.parse('the form shows "{message}"'))
def form_shows(world: World, message: str) -> None:
    expect(world.page.get_by_text(message)).to_be_visible(timeout=timeouts.NAVIGATION)
    assert urlsplit(world.page.url).path == "/register", world.page.url


# ── re-acceptance ────────────────────────────────────────────────────────────
@given("a legacy user without TERMS or PRIVACY_POLICY consent rows is signed in")
def legacy_user(world: World) -> None:
    user = ue.register_user("buyer", "legal_legacy")
    _psql(f"DELETE FROM consents WHERE user_id = '{user['id']}';")
    assert _all_records(user["token"]) == [], "legacy seed left consent rows behind"
    _sign_in(world, user)


@given("a user registered at the current versions is signed in")
def current_user(world: World) -> None:
    _sign_in(world, ue.register_user("buyer", "legal_current"))


@given("a user registered at the identity current terms version is signed in")
def identity_current_user(world: World) -> None:
    username = fake.unique_username("legal_newer")
    password = get_settings().seed_password
    body = {
        "username": username,
        "password": password,
        "role": "buyer",
        "termsVersion": world.state.extra["new_terms"],
        "privacyVersion": legal.CURRENT_PRIVACY,
    }
    resp = ue.call(ue.REGISTER, body)
    assert resp.status_code == 200, f"Register at the identity current version: {resp.text}"
    user = {"username": username, "password": password, "role": "buyer"}
    user.update(ue.login(user))
    rows = {r["purpose"]: r for r in _all_records(user["token"])}
    assert (
        rows[TERMS]["version"] != legal.CURRENT_TERMS
    ), "user is not at a version differing from the frontend's"
    _sign_in(world, user)


@when("the user loads the home page")
def user_home(world: World) -> None:
    _start_window(world)
    _nav(world, "/")


@when(parsers.parse("the user loads {n:d} different pages in one tab"))
def user_pages(world: World, n: int) -> None:
    _start_window(world)
    for path in ["/", "/cart", "/search"][:n]:
        _nav(world, path)
        _idle(world)


@when("the user reloads the page")
def reload(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")
    _idle(world)


@when(parsers.parse('the user opens the legal page "{path}" in the browser'))
def user_opens_legal(world: World, path: str) -> None:
    _nav(world, path)
    _idle(world)


@when("an anonymous visitor opens the home page in the browser")
def anon_home_browser(world: World) -> None:
    world.context.clear_cookies()
    _start_window(world)
    _nav(world, "/")
    _idle(world)


@then('the re-acceptance dialog with "Đồng ý" and "Đăng xuất" is visible')
def dialog_visible(world: World) -> None:
    expect(_reaccept_dialog(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@when('the user clicks "Đồng ý" in the re-acceptance dialog')
def click_accept(world: World) -> None:
    _x(world)["accept_at"] = time.monotonic()
    _reaccept_dialog(world).get_by_role("button", name="Đồng ý", exact=True).click()


@when('the user clicks "Đăng xuất" in the re-acceptance dialog')
def click_decline(world: World) -> None:
    _reaccept_dialog(world).get_by_role("button", name="Đăng xuất", exact=True).click()


@then("the re-acceptance dialog closes within 1 second")
def dialog_closes(world: World) -> None:
    expect(_reaccept_dialog(world)).to_be_hidden(timeout=1000)


@then("no re-acceptance dialog shows")
def no_dialog(world: World) -> None:
    _idle(world)
    expect(_reaccept_dialog(world)).to_have_count(0)


@then("the legal page renders and no re-acceptance dialog shows")
def legal_page_no_dialog(world: World) -> None:
    expect(world.page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_reaccept_dialog(world)).to_have_count(0)


@then(parsers.parse("the gateway received {n:d} RecordConsent call"))
def record_calls(world: World, n: int) -> None:
    assert _count(world, lu.RECORD_PROC) == n


@then(parsers.parse("the gateway received at most {n:d} ListMyConsents call"))
def list_calls_max(world: World, n: int) -> None:
    assert _count(world, lu.LIST_PROC) <= n


@then(parsers.parse("the gateway received {n:d} PrivacyService calls"))
def privacy_calls(world: World, n: int) -> None:
    assert _count(world, lu.PRIVACY_PROC) == n


@then(
    parsers.parse(
        'the ledger holds TERMS and PRIVACY_POLICY records at the current versions with source "{source}"'
    )
)
def ledger_accepted(world: World, source: str) -> None:
    token = _x(world)["user"]["token"]
    rows = _all_records(token)
    newest = {}
    for r in rows:
        newest.setdefault(r["purpose"], r)
    assert (
        newest[TERMS]["version"] == legal.CURRENT_TERMS
        and newest[PRIVACY]["version"] == legal.CURRENT_PRIVACY
    ), rows
    assert newest[TERMS]["source"] == source and newest[PRIVACY]["source"] == source, rows
    assert newest[TERMS].get("granted") is True and newest[PRIVACY].get("granted") is True, rows


@then("the page renders normally with no error shown")
def page_normal(world: World) -> None:
    _idle(world)
    expect(world.page.locator("header").first).to_be_visible()
    expect(world.page.locator("footer")).to_be_visible()
    expect(lu.page_alerts(world.page)).to_have_count(0)


@then('the session cookies are cleared and the user is on "/" as anonymous')
def signed_out(world: World) -> None:
    deadline = time.monotonic() + timeouts.NAVIGATION / 1000
    names = {c["name"] for c in world.context.cookies(_base(world))}
    while SESSION_COOKIE in names and time.monotonic() < deadline:
        time.sleep(0.25)
        names = {c["name"] for c in world.context.cookies(_base(world))}
    world.page.wait_for_url(f"{_base(world)}/", timeout=timeouts.NAVIGATION)
    expect(_reaccept_dialog(world)).to_have_count(0, timeout=timeouts.NAVIGATION)
    assert SESSION_COOKIE not in names, names
    assert not any(n.endswith("refresh") for n in names), names


@then("the legacy user has no TERMS or PRIVACY_POLICY consent row")
def legacy_no_rows(world: World) -> None:
    rows = [r for r in _all_records(_x(world)["user"]["token"]) if r["purpose"] in {TERMS, PRIVACY}]
    assert rows == [], rows


# ── banner ───────────────────────────────────────────────────────────────────
@when("a visitor with no consent cookie opens the home page in the browser")
def fresh_home(world: World) -> None:
    world.context.clear_cookies()
    _start_window(world)
    _nav(world, "/")
    expect(_banner(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the banner dialog title is "{title}"'))
def banner_title(world: World, title: str) -> None:
    expect(_banner(world).get_by_text(title).first).to_be_visible()


@then(parsers.parse('the banner groups are labelled "{a}", "{b}" and "{c}"'))
def banner_groups(world: World, a: str, b: str, c: str) -> None:
    for label in (a, b, c):
        expect(_banner(world).get_by_text(label).first).to_be_visible()


@then("no English label of the previous banner remains")
def banner_no_english(world: World) -> None:
    text = _banner(world).inner_text()
    left = [w for w in ENGLISH_BANNER_LABELS if w.lower() in text.lower()]
    assert not left, f"English labels left in the banner: {left}"


@given(
    parsers.parse(
        'a user registered at the current versions is signed in with the consent cookie "{value}"'
    )
)
def user_with_cookie(world: World, value: str) -> None:
    user = ue.register_user("buyer", "legal_banner")
    world.context.add_cookies(
        [
            {
                "name": CONSENT_COOKIE,
                "value": f"v=1&{value}&ts={int(time.time())}",
                "url": _base(world),
            }
        ]
    )
    _sign_in(world, user)


@given("a user registered at the current versions is signed in with no consent cookie")
def user_no_cookie(world: World) -> None:
    _sign_in(world, ue.register_user("buyer", "legal_banner"))


@when(
    'the user opens the banner from "Cài đặt cookie", turns on "Cá nhân hóa" and clicks "Lưu lựa chọn"'
)
def open_and_save(world: World) -> None:
    _start_window(world)
    _nav(world, "/")
    link = world.page.get_by_role("button", name="Cài đặt cookie").or_(
        world.page.get_by_role("link", name="Cài đặt cookie")
    )
    link.first.click()
    expect(_banner(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    box = _banner(world).get_by_label("Cá nhân hóa")
    if not box.is_checked():
        box.check()
    _banner(world).get_by_role("button", name="Lưu lựa chọn").click()


@when(parsers.parse('the user clicks "{label}" on the banner'))
@when(parsers.parse('the visitor clicks "{label}" on the banner'))
def click_banner(world: World, label: str) -> None:
    if _banner(world).count() == 0:
        _nav(world, "/")
    expect(_banner(world)).to_be_visible(timeout=timeouts.NAVIGATION)
    _start_window(world)
    _banner(world).get_by_role("button", name=label).click()


@then(parsers.parse('the consent cookie holds "{fragment}"'))
def cookie_holds(world: World, fragment: str) -> None:
    deadline = time.monotonic() + 10
    while fragment not in _consent_cookie(world) and time.monotonic() < deadline:
        time.sleep(0.25)
    assert fragment in _consent_cookie(world), _consent_cookie(world)


@then('the consent cookie matches "v=1&a=1&m=1&p=1&ts=<unix>"')
def cookie_matches(world: World) -> None:
    expect(_banner(world)).to_have_count(0)
    assert re.fullmatch(r"v=1&a=1&m=1&p=1&ts=\d{9,11}", _consent_cookie(world)), _consent_cookie(
        world
    )


@then(
    parsers.parse(
        'the newest ledger row is PERSONALIZATION granted with source "{source}" and version "{version}"'
    )
)
def newest_row(world: World, source: str, version: str) -> None:
    deadline = time.monotonic() + 15
    rows: list[dict] = []
    while time.monotonic() < deadline:
        rows = _all_records(_x(world)["user"]["token"])
        if rows and rows[0]["purpose"] == PERSONALIZATION:
            break
        time.sleep(0.5)
    row = rows[0]
    assert row["purpose"] == PERSONALIZATION and row.get("granted") is True, rows
    assert row["source"] == source and row["version"] == version, row


@then(parsers.parse("the ledger gained exactly {n:d} row after the Register rows"))
def ledger_gained(world: World, n: int) -> None:
    rows = _all_records(_x(world)["user"]["token"])
    assert len(rows) - _x(world)["baseline_rows"] == n, rows


@then("the ledger gained 3 banner rows for ANALYTICS, MARKETING and PERSONALIZATION, all denied")
def ledger_three(world: World) -> None:
    deadline = time.monotonic() + 15
    new: list[dict] = []
    while time.monotonic() < deadline:
        rows = _all_records(_x(world)["user"]["token"])
        new = rows[: len(rows) - _x(world)["baseline_rows"]]
        if len(new) >= 3:
            break
        time.sleep(0.5)
    assert len(new) == 3, new
    assert {r["purpose"] for r in new} == {ANALYTICS, MARKETING, PERSONALIZATION}, new
    assert all(r["source"] == "banner" and not r.get("granted") for r in new), new


@then("the banner closes and no error dialog is shown")
def banner_closed(world: World) -> None:
    expect(_banner(world)).to_have_count(0, timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("alertdialog")).to_have_count(0)
    expect(lu.page_alerts(world.page)).to_have_count(0)
