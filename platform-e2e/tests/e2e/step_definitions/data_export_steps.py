"""identity-data-export: ExportMyData through the gateway (change identity-data-export)."""

from __future__ import annotations

import time

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import account_deletion_support as ad
from tests.e2e.support import data_export_support as de
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

SECONDS_SKEW = 30.0


def _x(world: World) -> dict:
    return world.state.extra


def _users(world: World) -> dict[str, dict]:
    return _x(world).setdefault("exp_users", {})


def _doc(world: World) -> dict:
    return _x(world)["exp_doc"]


def _seed(world: World, name: str, addresses: int, sessions: int, events: int, consents: int) -> dict:
    user = ue.register_user("buyer", "exp_b")
    _users(world)[name] = user
    _x(world)[f"exp_addr_{name}"] = de.add_addresses(user, addresses)
    ok = max(sessions - 2, 0)
    de.extra_logins(user, ok=ok, failed=max(events - 1 - ok, 0))
    have = de.consent_total(user)
    if consents > have:
        purposes = ["CONSENT_PURPOSE_ANALYTICS", "CONSENT_PURPOSE_MARKETING",
                    "CONSENT_PURPOSE_PERSONALIZATION"][: consents - have]  # fmt: skip
        de.record_consents(user, purposes)
    return user


@given(
    parsers.parse(
        'an export buyer "{name}" with {a:d} addresses, {s:d} sessions, {e:d} login events and {c:d} consent records'
    )
)
def seeded(world: World, name: str, a: int, s: int, e: int, c: int) -> None:
    _seed(world, name, a, s, e, c)


@given(parsers.parse('an export buyer "{name}" with 1 address'))
def one_address(world: World, name: str) -> None:
    user = ue.register_user("buyer", "exp_b")
    _users(world)[name] = user
    _x(world)[f"exp_addr_{name}"] = de.add_addresses(user, 1)


@given(parsers.parse('an export buyer "{name}" with {n:d} login events'))
def many_events(world: World, name: str, n: int) -> None:
    user = ue.register_user("buyer", "exp_b")
    _users(world)[name] = user
    de.many_logins(user, n - 1)


@given(
    parsers.parse(
        'an export buyer "{name}" with a live refresh token, an unused handoff code and a status reason'
    )
)
def credentials_user(world: World, name: str) -> None:
    user = ue.register_user("buyer", "exp_b")
    _users(world)[name] = user
    x = _x(world)
    x["exp_secrets"] = {
        "password hash": de.password_hash(user["id"]),
        "refresh token": user["refresh"],
        "handoff code": de.mint_handoff(user),
    }
    reason = f"exp_reason_{int(time.time())}"
    resp = ue.set_user_status(world, user["id"], "restricted", reason=reason, note=reason)
    assert resp.status_code == 200, f"SetUserStatus {resp.status_code}: {resp.text[:300]}"
    x["exp_secrets"]["status reason"] = reason


@given(
    parsers.parse(
        'an export buyer "{name}" who changed analytics consent after accepting terms and requested account deletion'
    )
)
def consent_deletion_user(world: World, name: str) -> None:
    user = ue.register_user("buyer", "exp_b")
    _users(world)[name] = user
    time.sleep(1.1)
    de.record_consents(user, ["CONSENT_PURPOSE_ANALYTICS"])
    time.sleep(1.1)
    before = time.time()
    resp = ad.request_deletion(user["token"], user["password"])
    assert resp.status_code == 200, f"RequestAccountDeletion {resp.status_code}: {resp.text[:300]}"
    _x(world)["exp_request_at"] = (before, time.time())


@when(parsers.parse('"{name}" exports their data through the gateway'))
def export_as(world: World, name: str) -> None:
    resp = de.export(_users(world)[name]["token"])
    _x(world)["exp_resp"] = resp
    if resp.status_code == 200:
        _x(world)["exp_doc"] = de.document(resp)


@when("a caller without a token exports data through the gateway")
def export_anonymous(world: World) -> None:
    _x(world)["exp_resp"] = de.export(None)


@then(
    parsers.parse(
        "the export is OK and holds {a:d} addresses, {s:d} sessions, {e:d} login events and {c:d} consents"
    )
)
def holds_counts(world: World, a: int, s: int, e: int, c: int) -> None:
    doc = _doc(world)
    got = (len(doc["addresses"]), len(doc["sessions"]), len(doc["login_history"]), len(doc["consents"]))
    assert got == (a, s, e, c), f"addresses/sessions/login_history/consents {got}, want {(a, s, e, c)}"


@then(parsers.parse('the export account user id is the id of "{name}"'))
def account_id(world: World, name: str) -> None:
    assert _doc(world)["account"]["user_id"] == _users(world)[name]["id"]


@then("the export bytes contain none of the password hash, the refresh token, the handoff code or the status reason")
def no_secrets(world: World) -> None:
    raw = de.raw_bytes(_x(world)["exp_resp"]).decode("utf-8", errors="replace")
    for label, secret in _x(world)["exp_secrets"].items():
        assert secret, f"empty {label} fixture"
        assert secret not in raw, f"export contains the {label}"


@then("the export top-level keys are exactly the contracted nine")
def exact_keys(world: World) -> None:
    assert set(_doc(world)) == de.TOP_KEYS, sorted(_doc(world))


@then("the first export consent is the analytics record and the second is older")
def consent_order(world: World) -> None:
    consents = _doc(world)["consents"]
    assert len(consents) >= 2, consents
    assert "analytics" in str(consents[0].get("purpose", "")).lower(), consents[0]
    stamps = [c.get("recorded_at") for c in consents]
    assert all(stamps) and stamps == sorted(stamps, reverse=True), f"not newest first: {stamps}"
    assert stamps[0] > stamps[1], stamps
    assert "terms" in " ".join(str(c.get("purpose", "")).lower() for c in consents[1:])


@then("the export deletion is pending with erase_after 48 hours after the request")
def deletion_state(world: World) -> None:
    deletion = _doc(world)["deletion"]
    assert deletion["pending"] is True, deletion
    before, after = _x(world)["exp_request_at"]
    erase = ad.parse_ts(deletion["erase_after"])
    window = ad.configured_window_s() or ad.DEFAULT_WINDOW_S
    assert before + window - SECONDS_SKEW <= erase <= after + window + SECONDS_SKEW, deletion


@then("the export call fails unauthenticated")
def unauthenticated(world: World) -> None:
    resp = _x(world)["exp_resp"]
    assert resp.status_code == 401, f"{resp.status_code}: {resp.text[:200]}"
    assert ue.code_of(resp).lower() == "unauthenticated", resp.text[:200]


@then(parsers.parse('the export holds only the address ids of "{name}"'))
def only_addresses(world: World, name: str) -> None:
    got = sorted(a["id"] for a in _doc(world)["addresses"])
    assert got == sorted(_x(world)[f"exp_addr_{name}"]), got


@then("the export holds the newest 1000 login events and truncated.login_history is true")
def truncated(world: World) -> None:
    doc = _doc(world)
    assert len(doc["login_history"]) == de.LOGIN_CAP, len(doc["login_history"])
    assert doc["truncated"]["login_history"] is True, doc["truncated"]
    stamps = [e.get("at") or e.get("created_at") or e.get("occurred_at") for e in doc["login_history"]]
    assert all(stamps), f"login_history events carry no timestamp: {doc['login_history'][0]}"
    assert stamps == sorted(stamps, reverse=True), "login_history is not newest first"


@then(parsers.parse("the export data is at most {n:d} bytes"))
def size_cap(world: World, n: int) -> None:
    assert len(de.raw_bytes(_x(world)["exp_resp"])) <= n


@then(parsers.parse("the export response header Cache-Control equals {value}"))
def cache_header(world: World, value: str) -> None:
    got = _x(world)["exp_resp"].headers.get("cache-control")
    assert got == value, f"Cache-Control {got!r}"
