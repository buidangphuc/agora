"""Binds identity/seller_onboarding.feature (change identity-seller-onboarding, initiative p10-backend-gaps).

Black box through the gateway; helpers in support/seller_onboarding_support.
"""

from __future__ import annotations

import math
import time
from concurrent.futures import ThreadPoolExecutor

from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import seller_onboarding_support as so
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

scenarios("identity/seller_onboarding.feature")

P95_BUDGET_S = 0.2
EVENT_WAIT_S = 10.0
QUIET_WAIT_S = 4.0


def _s(world: World) -> dict:
    return world.state.extra.setdefault("so", {})


def _p95(samples: list[float]) -> float:
    ordered = sorted(samples)
    return ordered[max(0, math.ceil(0.95 * len(ordered)) - 1)]


@given("a registered buyer who cannot create listings")
def buyer(world: World) -> None:
    s = _s(world)
    s["tap"] = so.RoleGrantedTap()
    s["buyer"] = so.new_buyer()
    s["first_token"] = s["buyer"]["token"]
    denied = so.create_listing(s["buyer"]["token"])
    assert so.code(denied) == "permission_denied", f"a buyer must not create listings: {denied.status_code} {denied.text[:200]}"


@given("a registered buyer who has become a seller")
def seller(world: World) -> None:
    buyer(world)
    s = _s(world)
    resp = so.become_seller(s["buyer"]["token"])
    assert resp.status_code == 200, f"BecomeSeller failed: {resp.status_code} {resp.text[:200]}"
    s["first_result"] = so.result_of(resp)
    s["first_row"] = so.terms_row(s["buyer"]["id"])
    s["first_call_at"] = time.time()


@given("an admin restricts the buyer")
@when("an admin restricts the buyer")
def restrict(world: World) -> None:
    s = _s(world)
    resp = ue.set_user_status(world, s["buyer"]["id"], "restricted")
    assert resp.status_code == 200, f"SetUserStatus failed: {resp.status_code} {resp.text[:200]}"


@given("the buyer's first session is revoked from a second session")
def revoke_first_session(world: World) -> None:
    s = _s(world)
    buyer_ = s["buyer"]
    second = ue.login(buyer_)
    sid = ue.claims(s["first_token"])["sid"]
    resp = ue.call(so.REVOKE, {"sessionId": sid}, second["token"])
    assert resp.status_code == 200, f"RevokeSession failed: {resp.status_code} {resp.text[:200]}"
    s["second"] = second
    time.sleep(3)


@when("the buyer calls BecomeSeller with the current terms version")
def call_current(world: World) -> None:
    s = _s(world)
    s["call_at"] = time.time()
    s["resp"] = so.become_seller(s["buyer"]["token"])


@when("the buyer calls BecomeSeller with the current terms version again")
def call_again(world: World) -> None:
    s = _s(world)
    s["resp"] = so.become_seller(s["buyer"]["token"])


@when("the buyer calls BecomeSeller with the revoked session's token")
def call_revoked(world: World) -> None:
    s = _s(world)
    s["resp"] = so.become_seller(s["first_token"])


@when("the buyer calls BecomeSeller with an empty terms version")
def call_empty(world: World) -> None:
    s = _s(world)
    s.setdefault("resps", []).append(so.become_seller(s["buyer"]["token"], ""))


@when(parsers.parse('the buyer calls BecomeSeller with the terms version "{version}"'))
def call_version(world: World, version: str) -> None:
    s = _s(world)
    s.setdefault("resps", []).append(so.become_seller(s["buyer"]["token"], version))


@when("the buyer calls BecomeSeller with the current terms version padded with spaces")
def call_padded(world: World) -> None:
    s = _s(world)
    s.setdefault("resps", []).append(so.become_seller(s["buyer"]["token"], f" {so.current_terms()} "))


@when(parsers.parse('the buyer calls BecomeSeller with the current terms version followed by "{suffix}"'))
def call_suffixed(world: World, suffix: str) -> None:
    s = _s(world)
    s.setdefault("resps", []).append(so.become_seller(s["buyer"]["token"], so.current_terms() + suffix))


@when("an anonymous caller calls BecomeSeller with the current terms version")
def call_anonymous(world: World) -> None:
    _s(world)["resp"] = so.become_seller(None)


@when("the buyer sends two BecomeSeller calls concurrently")
def call_concurrent(world: World) -> None:
    s = _s(world)
    with ThreadPoolExecutor(max_workers=2) as pool:
        s["resps"] = list(pool.map(lambda _: so.become_seller(s["buyer"]["token"]), range(2)))


@when("the buyer refreshes the session with the refresh token issued at login")
def refresh(world: World) -> None:
    s = _s(world)
    assert s["buyer"]["refresh"], "login returned no refresh token"
    s["refresh_resp"] = ue.call(ue.REFRESH, {"refreshToken": s["buyer"]["refresh"]})


@when(parsers.parse("{n:d} distinct buyers each call BecomeSeller once with the current terms version"))
def many_current(world: World, n: int) -> None:
    _s(world)["timed"] = _timed(n, None)


@when(parsers.parse("{n:d} distinct buyers each call BecomeSeller once with an unknown terms version"))
def many_unknown(world: World, n: int) -> None:
    _s(world)["timed"] = _timed(n, "1999-01-01")


def _timed(n: int, version: str | None) -> list[tuple[object, float]]:
    out = []
    for _ in range(n):
        user = so.new_buyer()
        started = time.perf_counter()
        resp = so.become_seller(user["token"], version)
        out.append((resp, time.perf_counter() - started))
    return out


@then("the call succeeds with a token carrying listing.write")
def ok_with_scope(world: World) -> None:
    resp = _s(world)["resp"]
    assert resp.status_code == 200, f"BecomeSeller: {resp.status_code} {resp.text[:300]}"
    token = so.result_of(resp)["token"]
    assert "listing.write" in so.scopes_of(token), f"scopes: {so.scopes_of(token)}"
    _s(world)["new_token"] = token


@then("creating a listing with that token no longer fails permission denied")
def listing_allowed(world: World) -> None:
    resp = so.create_listing(_s(world)["new_token"])
    assert so.code(resp) != "permission_denied", f"still denied: {resp.status_code} {resp.text[:300]}"


@then("the new token keeps the session id of the caller and carries listing.write")
def token_sid(world: World) -> None:
    s = _s(world)
    resp = s["resp"]
    assert resp.status_code == 200, f"BecomeSeller: {resp.status_code} {resp.text[:300]}"
    token = so.result_of(resp)["token"]
    assert ue.claims(token)["sid"] == ue.claims(s["first_token"])["sid"]
    assert "listing.write" in so.scopes_of(token)


@then("the buyer's roles are buyer and seller")
def roles_both(world: World) -> None:
    assert so.roles_of(world, _s(world)["buyer"]["id"]) == ["buyer", "seller"]


@then("the buyer's roles are buyer only")
def roles_buyer(world: World) -> None:
    assert so.roles_of(world, _s(world)["buyer"]["id"]) == ["buyer"]


@then("the user row holds the current terms version and an acceptance time within 5 seconds of the call")
def row_recorded(world: World) -> None:
    s = _s(world)
    assert s["resp"].status_code == 200, f"BecomeSeller: {s['resp'].status_code} {s['resp'].text[:300]}"
    version, epoch = so.terms_row(s["buyer"]["id"])
    assert version == so.current_terms(), f"stored version {version!r}"
    assert epoch and abs(int(epoch) - s["call_at"]) <= 5, f"stored acceptance {epoch}, call at {s['call_at']}"


@then("the BecomeSeller response carries no refresh token")
def no_refresh(world: World) -> None:
    resp = _s(world)["resp"]
    assert resp.status_code == 200, f"BecomeSeller: {resp.status_code} {resp.text[:300]}"
    assert not so.result_of(resp).get("refreshToken"), "BecomeSeller returned a refresh token"


@then("the refreshed token has the same session id and listing.write")
def refreshed_ok(world: World) -> None:
    s = _s(world)
    resp = s["refresh_resp"]
    assert resp.status_code == 200, f"RefreshSession: {resp.status_code} {resp.text[:300]}"
    token = ue.body_of(resp)["token"]
    assert ue.claims(token)["sid"] == ue.claims(s["first_token"])["sid"]
    assert "listing.write" in so.scopes_of(token)


@then(parsers.parse("both calls fail with {code}"))
def both_fail(world: World, code: str) -> None:
    for resp in _s(world)["resps"]:
        assert so.code(resp) == code, f"expected {code}, got {resp.status_code} {resp.text[:300]}"


@then(parsers.parse("the call fails with {code}"))
def one_fails(world: World, code: str) -> None:
    resp = _s(world)["resp"]
    assert so.code(resp) == code, f"expected {code}, got {resp.status_code} {resp.text[:300]}"


@then("the response carries no token")
def no_token(world: World) -> None:
    body = _s(world)["resp"].json()
    assert not body.get("token") and not body.get("result", {}).get("token"), "a token was returned"


@then(parsers.parse("the response status is {status:d}"))
def status_is(world: World, status: int) -> None:
    resp = _s(world)["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code} {resp.text[:300]}"


@then("no UserRoleGranted event is published for the buyer")
def no_event(world: World) -> None:
    s = _s(world)
    assert s["tap"].events(s["buyer"]["id"], QUIET_WAIT_S) == []


@then("the buyer can still become a seller with the current terms version")
def control_buyer(world: World) -> None:
    s = _s(world)
    token = s["second"]["token"] if "second" in s else s["buyer"]["token"]
    resp = so.become_seller(token)
    assert resp.status_code == 200, f"positive control failed: {resp.status_code} {resp.text[:300]}"
    assert "listing.write" in so.scopes_of(so.result_of(resp)["token"])


@then("another active buyer can become a seller with the current terms version")
def control_other(world: World) -> None:
    other = so.new_buyer()
    resp = so.become_seller(other["token"])
    assert resp.status_code == 200, f"positive control failed: {resp.status_code} {resp.text[:300]}"


@then(parsers.parse("exactly one UserRoleGranted event with role seller reaches identity.events within {n:d} seconds"))
def one_event(world: World, n: int) -> None:
    s = _s(world)
    events = s["tap"].events(s["buyer"]["id"], float(n), stop_at=1)
    time.sleep(2)
    events = s["tap"].events(s["buyer"]["id"], 1.0)
    assert [e["role"] for e in events] == ["seller"], f"events: {events}"


@then("exactly one UserRoleGranted event with role seller was published for the buyer")
def one_event_total(world: World) -> None:
    s = _s(world)
    events = s["tap"].events(s["buyer"]["id"], EVENT_WAIT_S, stop_at=1)
    events = s["tap"].events(s["buyer"]["id"], QUIET_WAIT_S)
    assert [e["role"] for e in events] == ["seller"], f"events: {events}"


@then("the call succeeds with a new token on the same session id")
def again_ok(world: World) -> None:
    s = _s(world)
    resp = s["resp"]
    assert resp.status_code == 200, f"BecomeSeller again: {resp.status_code} {resp.text[:300]}"
    token = so.result_of(resp)["token"]
    assert token != s["first_result"]["token"], "no fresh token"
    assert ue.claims(token)["sid"] == ue.claims(s["first_token"])["sid"]


@then("the stored terms version and acceptance time equal the first call's values")
def row_unchanged(world: World) -> None:
    s = _s(world)
    assert so.terms_row(s["buyer"]["id"]) == s["first_row"]
    assert s["first_row"][0] == so.current_terms()


@then("both calls succeed")
def both_ok(world: World) -> None:
    for resp in _s(world)["resps"]:
        assert resp.status_code == 200, f"BecomeSeller: {resp.status_code} {resp.text[:300]}"


@then("the refreshed token has no listing.write and carries status restricted")
def refreshed_restricted(world: World) -> None:
    s = _s(world)
    resp = s["refresh_resp"]
    assert resp.status_code == 200, f"RefreshSession: {resp.status_code} {resp.text[:300]}"
    token = ue.body_of(resp)["token"]
    assert "listing.write" not in so.scopes_of(token)
    assert ue.claims(token).get("status") == "restricted", ue.claims(token)


@then("every call succeeds and the 95th percentile is below 200 ms")
def latency_ok(world: World) -> None:
    timed = _s(world)["timed"]
    bad = [r.status_code for r, _ in timed if r.status_code != 200]
    assert not bad, f"calls failed: {bad[:5]}"
    p95 = _p95([t for _, t in timed])
    assert p95 < P95_BUDGET_S, f"p95 {p95 * 1000:.0f} ms"


@then("every call fails with invalid_argument and the 95th percentile is below 200 ms")
def rejected_fast(world: World) -> None:
    timed = _s(world)["timed"]
    bad = [so.code(r) for r, _ in timed if so.code(r) != "invalid_argument"]
    assert not bad, f"unexpected codes: {bad[:5]}"
    p95 = _p95([t for _, t in timed])
    assert p95 < P95_BUDGET_S, f"p95 {p95 * 1000:.0f} ms"
