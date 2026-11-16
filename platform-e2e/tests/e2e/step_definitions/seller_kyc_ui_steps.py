"""Seller KYC UI (change seller-kyc-ui): /seller/verification, wallet payout notice, shop badge.

KYC state is reached only through the real RPCs (`flows.kyc_flow`: SubmitKyc and an admin ReviewKyc);
submissions are read back through ListKycSubmissions and GetVerificationStatus. Faults are real
container stops (team-verification, the document storage), restored by world cleanups.
"""

from __future__ import annotations

import re
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services.verification_service import VerificationService
from src.constants import PageName, timeouts
from src.pages import SellerVerificationPage, SellerWalletPage
from tests.e2e.flows import kyc_flow as k
from tests.e2e.flows import login_via_api, scenario_buyer
from tests.e2e.support import kyc_documents_support as kyc
from tests.e2e.support import seller_kyc_ui_support as sup
from tests.e2e.support.world import World

VERIFICATION_PATH = "/seller/verification"
DESKTOP = {"width": 1280, "height": 720}
NOTICE = "Cần xác minh danh tính để rút tiền"
STATES = {
    "PENDING": k.PENDING,
    "REJECTED": k.REJECTED,
    "NOT_SUBMITTED": k.NOT_SUBMITTED,
}


def _x(world: World) -> dict:
    return world.state.extra.setdefault("skui", {"puts": [], "posts": [], "files": []})


def _token(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "scenario must be tagged @needsSeller"
    return seller.token


def _seller_id(world: World) -> str:
    return kyc.jwt_sub(_token(world))


def _page(world: World) -> SellerVerificationPage:
    return world.get_page(PageName.SELLER_VERIFICATION)  # type: ignore[return-value]


def _wallet(world: World) -> SellerWalletPage:
    return world.get_page(PageName.SELLER_WALLET)  # type: ignore[return-value]


def _record_requests(world: World) -> None:
    x = _x(world)
    if x.get("recording"):
        return
    x["recording"] = True
    seller_host = world.settings.seller_base_url.rstrip("/")

    def on_request(request) -> None:  # noqa: ANN001
        if request.method == "PUT":
            body = request.post_data_buffer or b""
            x["puts"].append(
                {
                    "url": request.url,
                    "type": request.headers.get("content-type", ""),
                    "length": request.headers.get("content-length"),
                    "size": len(body),
                }
            )
        elif request.method == "POST" and request.url.startswith(seller_host):
            x["posts"].append(request.post_data_buffer or b"")

    world.page.on("request", on_request)


def _open_verification(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    _record_requests(world)
    world.navigate_to(PageName.SELLER_VERIFICATION)


def _find_submission(user_id: str) -> dict | None:
    svc = VerificationService(token=k.admin_token())
    cursor: str | None = None
    for _ in range(500):
        resp = svc.list_kyc_submissions_response("PENDING", 100, cursor)
        assert resp.status_code == 200, f"ListKycSubmissions: {resp.status_code} {resp.text}"
        data = resp.json()
        for sub in data.get("submissions", []):
            if sub.get("userId") == user_id:
                return sub
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            return None
    raise AssertionError("ListKycSubmissions paging did not terminate")


# ── Givens: KYC state through the real RPCs ──────────────────────────────
@given("the seller has no KYC submission")
def seller_no_submission(world: World) -> None:
    assert k.status_of(_token(world)) == k.NOT_SUBMITTED


@given("the seller has a PENDING KYC submission")
def seller_pending(world: World) -> None:
    k.submit(_token(world))
    assert k.status_of(_token(world)) == k.PENDING


@given("the seller was approved by an admin through ReviewKyc")
def seller_approved(world: World) -> None:
    k.verify_seller(_token(world))


@given(
    parsers.parse(
        'the seller was rejected by an admin through ReviewKyc with reason code "{reason}"'
    )
)
def seller_rejected(world: World, reason: str) -> None:
    k.reject_seller(_token(world), reason)


@given(parsers.parse("the seller's KYC state is {state}"))
def seller_state(world: World, state: str) -> None:
    token = _token(world)
    if state == "PENDING":
        k.submit(token)
    elif state == "REJECTED":
        k.reject_seller(token)
    else:
        assert state == "NOT_SUBMITTED", state
    assert k.status_of(token) == STATES[state]


@given("another seller was approved by an admin through ReviewKyc")
def other_seller_approved(world: World) -> None:
    token, _ = kyc.register(world, "seller")
    k.verify_seller(token)


@given("team-verification is stopped for the seller UI")
@when("team-verification is stopped for the seller UI")
def verification_stopped(world: World) -> None:
    sup.stop_verification(world)


@given("the KYC document storage is stopped")
def storage_stopped(world: World) -> None:
    sup.stop_storage(world)


@when("the KYC document storage is started again")
def storage_started(world: World) -> None:
    sup.start_storage(world)


@given("a signed-in buyer without seller scope is on the seller app")
def buyer_on_seller_app(world: World) -> None:
    login_via_api(world, scenario_buyer(world))


# ── Whens ────────────────────────────────────────────────────────────────
@when("the seller opens the verification page")
def seller_opens_verification(world: World) -> None:
    _open_verification(world)


@when("an anonymous visitor opens the verification page")
def guest_opens_verification(world: World) -> None:
    world.page.goto(
        f"{world.settings.seller_base_url.rstrip('/')}{VERIFICATION_PATH}",
        wait_until="domcontentloaded",
    )


def _select(world: World, files: list[dict]) -> None:
    x = _x(world)
    x["files"] = files
    page = _page(world)
    expect(page.file_inputs.first).to_be_attached(timeout=timeouts.NAVIGATION)
    page.wait_until_interactive(page.file_inputs.first)
    page.file_inputs.first.set_input_files(files)


def _marker(world: World) -> bytes:
    x = _x(world)
    return x.setdefault("marker", sup.new_marker())


@when(parsers.parse("the seller selects {n:d} valid file"))
@when(parsers.parse("the seller selects {n:d} valid files"))
def select_valid(world: World, n: int) -> None:
    _select(world, sup.valid_files(n, _marker(world)))


@when(parsers.parse("the seller selects a PNG of {size:d} bytes"))
def select_png(world: World, size: int) -> None:
    _select(world, [sup.make_file("PNG", "cccd.png", size, _marker(world))])


@when(parsers.parse('the seller selects a {kind} file named "{name}" of {size:d} bytes'))
def select_named(world: World, kind: str, name: str, size: int) -> None:
    _select(world, [sup.make_file(kind, name, size, _marker(world))])


@when("the seller ticks the consent checkbox")
def tick_consent(world: World) -> None:
    _page(world).consent_checkbox.check()


@when(parsers.parse("the seller selects {n:d} valid file and leaves consent unticked"))
def select_without_consent(world: World, n: int) -> None:
    _select(world, sup.valid_files(n, _marker(world)))
    expect(_page(world).consent_checkbox).not_to_be_checked()


@when(parsers.parse("the seller selects {n:d} valid file and ticks the consent checkbox"))
def select_and_consent(world: World, n: int) -> None:
    _select(world, sup.valid_files(n, _marker(world)))
    _page(world).consent_checkbox.check()


@when(
    parsers.re(
        r"the seller selects (?P<n>\d+) valid files?, ticks the consent checkbox "
        r'and presses "(?P<label>[^"]+)"'
    )
)
def select_consent_submit(world: World, n: str, label: str) -> None:
    _select(world, sup.valid_files(int(n), _marker(world)))
    page = _page(world)
    page.consent_checkbox.check()
    expect(page.submit_button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.page.get_by_role("button", name=label).click()


@when(parsers.parse('the seller presses "{label}" again without re-selecting files'))
def press_again(world: World, label: str) -> None:
    button = _page(world).page.get_by_role("button", name=label)
    expect(button).to_be_enabled(timeout=timeouts.NAVIGATION)
    button.click()


@when(parsers.parse('the seller double-clicks "{label}"'))
def double_click(world: World, label: str) -> None:
    button = _page(world).page.get_by_role("button", name=label)
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    button.dblclick()


@when(parsers.parse('the seller visits "{path}"'))
def seller_visits(world: World, path: str) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.page.goto(
        f"{world.settings.seller_base_url.rstrip('/')}{path}", wait_until="domcontentloaded"
    )


# ── Thens: status page ───────────────────────────────────────────────────
@then(parsers.parse('the verification page shows "{label}" and the upload form'))
def shows_label_and_form(world: World, label: str) -> None:
    page = _page(world)
    expect(page.text(label).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.file_inputs.first).to_be_attached()
    expect(page.submit_button).to_be_visible()


@then("the verification page shows only the signed-in seller's own status")
def shows_own_status(world: World) -> None:
    page = _page(world)
    expect(page.text("Chưa gửi hồ sơ").first).to_be_visible(timeout=timeouts.NAVIGATION)
    for other in ("Đã xác minh", "Đang chờ duyệt", "Bị từ chối"):
        expect(page.page.get_by_role("main").get_by_text(other, exact=True)).to_have_count(0)


@then(parsers.parse('the verification page shows "{label}" and renders 0 file inputs'))
def shows_label_no_inputs(world: World, label: str) -> None:
    page = _page(world)
    expect(page.page.get_by_role("main").get_by_text(label).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.file_inputs).to_have_count(0)


@then(
    parsers.parse(
        'the verification page shows "{label}" with the text for that reason and the upload form again'
    )
)
def shows_rejected_with_reason(world: World, label: str) -> None:
    page = _page(world)
    main = page.page.get_by_role("main")
    expect(main.get_by_text(label).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.file_inputs.first).to_be_attached()
    reason_text = main.inner_text()
    assert len(reason_text.split(label, 1)[-1].strip()) > 0, "no reject reason text rendered"


@then(parsers.parse('the raw reason code "{code}" is not rendered'))
def raw_code_absent(world: World, code: str) -> None:
    assert code not in _page(world).page.content()


@then(
    parsers.parse(
        'the verification page shows an error alert with a "{link}" link and renders 0 upload forms'
    )
)
def shows_error_alert(world: World, link: str) -> None:
    page = _page(world)
    expect(page.error_alert.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name=link)).to_have_count(1)
    expect(page.file_inputs).to_have_count(0)


@then(parsers.parse('the verification page does not show "{text}"'))
def not_shown(world: World, text: str) -> None:
    expect(_page(world).text(text)).to_have_count(0)


@then(parsers.parse('the browser is sent to the login page with the return path "{path}"'))
def sent_to_login(world: World, path: str) -> None:
    world.page.wait_for_url(re.compile(r"/login"), timeout=timeouts.NAVIGATION)
    query = parse_qs(urlsplit(world.page.url).query)
    target = (query.get("returnUrl") or query.get("next") or [""])[0]
    assert urlsplit(target).path == path, world.page.url


@then("the seller access-denied view is shown and the verification page content is not")
def access_denied(world: World) -> None:
    page = _page(world)
    expect(page.page.get_by_text("Không có quyền").first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.file_inputs).to_have_count(0)
    expect(page.submit_button).to_have_count(0)


# ── Thens: upload form ───────────────────────────────────────────────────
@then("the browser sent 3 uploads with the matching Content-Type and Content-Length")
def three_uploads(world: World) -> None:
    x = _x(world)
    page = _page(world)
    expect(page.text("Đang chờ duyệt").first).to_be_visible(timeout=timeouts.NAVIGATION)
    puts = x["puts"]
    assert len(puts) == 3, puts
    expected = sorted((f["mimeType"], len(f["buffer"])) for f in x["files"])
    got = sorted((p["type"].split(";")[0], p["size"]) for p in puts)
    assert got == expected, (got, expected)
    for p in puts:
        assert p["length"] in (None, str(p["size"])), p


@then("the seller's KYC submission holds 3 documents")
def holds_three_documents(world: World) -> None:
    sub = kyc.poll(
        lambda: _find_submission(_seller_id(world)), 20, "the seller's PENDING submission", 1.0
    )
    assert sub.get("documentCount") == 3, sub


@then("the submit button is disabled")
def submit_disabled(world: World) -> None:
    expect(_page(world).submit_button).to_be_disabled(timeout=timeouts.DEFAULT)


@then(parsers.parse('the form shows "{message}" and the submit button is disabled'))
def message_and_disabled(world: World, message: str) -> None:
    page = _page(world)
    expect(page.text(message).first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.submit_button).to_be_disabled()


@then("the form shows no size error and the submit button is enabled")
def no_size_error(world: World) -> None:
    page = _page(world)
    expect(page.submit_button).to_be_enabled(timeout=timeouts.DEFAULT)
    expect(page.text("Tệp tối đa 5 MiB")).to_have_count(0)


@then(parsers.re(r'the form shows "(?P<message>[^"]+)"$'))
def form_shows(world: World, message: str) -> None:
    expect(_page(world).text(message).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the browser made 0 upload requests")
def no_uploads(world: World) -> None:
    assert _x(world)["puts"] == [], _x(world)["puts"]


@then("the browser made 0 upload requests and the seller has no KYC submission")
def no_uploads_no_submission(world: World) -> None:
    assert _x(world)["puts"] == [], _x(world)["puts"]
    assert kyc.count("kyc_submissions", _seller_id(world)) == 0


@then("the seller has no KYC submission")
def still_no_submission(world: World) -> None:
    assert kyc.count("kyc_submissions", _seller_id(world)) == 0
    assert k.status_of(_token(world)) == k.NOT_SUBMITTED


@then(parsers.parse("the seller has exactly {n:d} KYC submission"))
def exactly_submissions(world: World, n: int) -> None:
    assert kyc.count("kyc_submissions", _seller_id(world)) == n


@then("no request to the seller app carried the file bytes")
def no_bytes_in_posts(world: World) -> None:
    x = _x(world)
    assert x["files"], "no file was selected"
    marker = x["marker"]
    assert x["posts"], "the browser sent no request to the seller app"
    for body in x["posts"]:
        assert marker not in body, "file bytes reached a request to the seller app"


@then("the page content contains no presigned URL and no document reference")
def no_presigned_in_dom(world: World) -> None:
    html = _page(world).page.content()
    for marker in ("X-Amz-Signature", "X-Amz-Credential", "X-Amz-Expires", kyc.BUCKET):
        assert marker not in html, f"{marker} found in the rendered page"
    sub = _find_submission(_seller_id(world))
    assert sub is not None
    for ref in sub.get("docRefs", []) or []:
        assert ref not in html


@then(parsers.parse('the navigation contains exactly 1 entry "{label}" with href "{href}"'))
def nav_entry(world: World, label: str, href: str) -> None:
    link = _page(world).page.get_by_role("link", name=label, exact=True)
    expect(link).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(link).to_have_attribute("href", href)


# ── Wallet ───────────────────────────────────────────────────────────────
def _verification_link(scope) -> object:  # noqa: ANN001
    return scope.locator(f'a[href="{VERIFICATION_PATH}"]')


@then(parsers.parse('the wallet shows the notice "{text}" with a link to "{href}"'))
def wallet_notice(world: World, text: str, href: str) -> None:
    main = _wallet(world).page.get_by_role("main")
    expect(main.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_verification_link(main).first).to_be_visible()


@then("the wallet shows no verification notice")
def wallet_no_notice(world: World) -> None:
    page = _wallet(world)
    expect(page.balance_label).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_text(NOTICE)).to_have_count(0)


@when("the seller opens the payout dialog")
def open_payout_dialog(world: World) -> None:
    page = _wallet(world)
    expect(page.payout_button).to_be_enabled(timeout=timeouts.NAVIGATION)
    page.wait_until_interactive(page.payout_button)
    page.payout_button.click()
    expect(page.confirm_dialog).to_be_visible(timeout=timeouts.DEFAULT)


@when("the seller confirms a payout of the offered amount")
def confirm_payout(world: World) -> None:
    page = _wallet(world)
    seller = world.state.seeded_seller
    world.service_factory.set_token(seller.token)
    _x(world)["balance_before"] = int(world.service_factory.payment.wallet_balance())
    if not page.confirm_dialog.is_visible():
        open_payout_dialog(world)
    amount = page.confirm_dialog.get_by_label("Số tiền", exact=False)
    if not amount.input_value().strip():
        amount.fill(str(_x(world)["balance_before"]))
    page.confirm_payout.click()


@then(parsers.parse('the payout dialog shows "{text}" with a link to "{href}"'))
def dialog_message_link(world: World, text: str, href: str) -> None:
    dialog = _wallet(world).confirm_dialog
    expect(dialog.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_verification_link(dialog).first).to_be_visible()


@then(parsers.re(r'the payout dialog shows "(?P<text>[^"]+)"$'))
def dialog_message(world: World, text: str) -> None:
    expect(_wallet(world).confirm_dialog.get_by_text(text).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the payout dialog does not show "{text}"'))
def dialog_no_message(world: World, text: str) -> None:
    expect(_wallet(world).confirm_dialog.get_by_text(text)).to_have_count(0)


@then("the seller's ledger balance is unchanged")
def ledger_unchanged(world: World) -> None:
    world.service_factory.set_token(world.state.seeded_seller.token)
    assert int(world.service_factory.payment.wallet_balance()) == _x(world)["balance_before"]


@then("the payout confirm button is enabled again")
def confirm_enabled(world: World) -> None:
    expect(_wallet(world).confirm_payout).to_be_enabled(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the toast "{text}" is shown'))
def toast_shown(world: World, text: str) -> None:
    expect(_wallet(world).page.get_by_text(text, exact=True)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


# ── Buyer app and shop page ──────────────────────────────────────────────
@when(parsers.parse('the buyer requests "{path}" on the buyer app'))
def buyer_requests(world: World, path: str) -> None:
    resp = world.page.goto(
        f"{world.settings.base_url.rstrip('/')}{path}", wait_until="domcontentloaded"
    )
    _x(world)["status"] = resp.status if resp else 0


@then(parsers.parse("the response status is {code:d}"))
def response_status(world: World, code: int) -> None:
    assert _x(world)["status"] == code, _x(world)["status"]


@when(parsers.parse('the buyer opens the account pages "{a}", "{b}" and "{c}"'))
def buyer_opens_account_pages(world: World, a: str, b: str, c: str) -> None:
    results = {}
    for path in (a, b, c):
        resp = world.page.goto(
            f"{world.settings.base_url.rstrip('/')}{path}", wait_until="domcontentloaded"
        )
        assert resp is not None and resp.status == 200, f"{path}: {resp and resp.status}"
        expect(world.page.get_by_role("navigation").first).to_be_visible(
            timeout=timeouts.NAVIGATION
        )
        results[path] = world.page.locator('a[href="/account/verification"]').count()
    _x(world)["link_counts"] = results


@then(parsers.parse('every page has 0 links to "{href}" in the account nav and the top bar'))
def no_links(world: World, href: str) -> None:
    assert _x(world)["link_counts"] and all(n == 0 for n in _x(world)["link_counts"].values()), _x(
        world
    )["link_counts"]


@when("an anonymous visitor opens the shop page of the seller")
def anon_opens_shop(world: World) -> None:
    resp = world.page.goto(
        f"{world.settings.base_url.rstrip('/')}/shop/{_seller_id(world)}",
        wait_until="domcontentloaded",
    )
    _x(world)["status"] = resp.status if resp else 0
    expect(world.page.locator("h1").first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the shop page contains exactly 1 "{label}" badge'))
def one_badge(world: World, label: str) -> None:
    expect(world.page.get_by_text(label, exact=True)).to_have_count(1, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the shop page contains 0 "{label}" badges'))
def zero_badges(world: World, label: str) -> None:
    expect(world.page.get_by_text(label, exact=True)).to_have_count(0)


@then(parsers.parse('the shop page shows none of "{a}", "{b}" and "{c}"'))
def no_status_words(world: World, a: str, b: str, c: str) -> None:
    for word in (a, b, c):
        expect(world.page.get_by_text(word)).to_have_count(0)


@then(parsers.parse('the shop page answers with HTTP 200 and contains 0 "{label}" badges'))
def ok_without_badge(world: World, label: str) -> None:
    assert _x(world)["status"] == 200, _x(world)["status"]
    expect(world.page.get_by_text(label, exact=True)).to_have_count(0)
