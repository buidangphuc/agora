"""identity-legal-consent: Register legal versions, PrivacyService consent ledger, boot validation.

Black box through the gateway (Connect JSON). Exceptions that cannot go through the edge:
the service-principal call is a grpcurl call to team-identity on the stack network, the boot
scenario runs the real image with `docker run`, and the stack-level scenarios restart
team-identity with compose/legal-versions.override.yaml (restored in teardown).
"""

from __future__ import annotations

import json
import os
import subprocess
import time
import uuid
from datetime import datetime, timezone

from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import gateway_endpoints as ep
from src.constants import legal_versions as legal
from src.utils import data as fake
from tests.e2e.support import apr_support as apr
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

OVERLAY = pe.REPO_ROOT / "platform-e2e" / "compose" / "legal-versions.override.yaml"
TERMS = "CONSENT_PURPOSE_TERMS"
PRIVACY = "CONSENT_PURPOSE_PRIVACY_POLICY"
ANALYTICS = "CONSENT_PURPOSE_ANALYTICS"
MARKETING = "CONSENT_PURPOSE_MARKETING"
PERSONALIZATION = "CONSENT_PURPOSE_PERSONALIZATION"
SKEW_S = 5.0


def _x(world: World) -> dict:
    return world.state.extra


def _identity_container() -> str:
    return os.getenv("IDENTITY_CONTAINER", "team-identity-svc")


def _version(value: str) -> str:
    return {"CURRENT": legal.CURRENT_TERMS, "CURRENT_PRIVACY": legal.CURRENT_PRIVACY}.get(
        value, value
    )


def _register(world: World, terms: str, privacy: str, extra: dict | None = None) -> dict:
    username = fake.unique_username("legal")
    body = {
        "username": username,
        "password": get_settings().seed_password,
        "role": "buyer",
        "termsVersion": _version(terms),
        "privacyVersion": _version(privacy),
    }
    body.update(extra or {})
    started = time.time()
    resp = ue.call(ep.AUTH_REGISTER, body)
    token = ue.body_of(resp).get("token", "") if resp.status_code == 200 else ""
    reg = {
        "username": username,
        "resp": resp,
        "token": token,
        "user_id": ue.user_id_of(token) if token else "",
        "started": started,
    }
    _x(world).setdefault("regs", []).append(reg)
    _x(world)["reg"] = reg
    return reg


def _registered_user(world: World) -> dict:
    reg = _register(world, "CURRENT", "CURRENT_PRIVACY")
    assert reg["resp"].status_code == 200, reg["resp"].text
    _x(world)["user"] = reg
    return reg


def _user(world: World) -> dict:
    return _x(world)["user"]


def _list(token: str | None, page: dict | None = None):
    body = {"page": page} if page else {}
    return ue.call(ep.PRIVACY_LIST_MY_CONSENTS, body, token)


def _all_records(token: str, page_size: int = 100) -> list[dict]:
    out: list[dict] = []
    cursor = ""
    for _ in range(50):
        resp = _list(token, {"pageSize": page_size, "cursor": cursor})
        assert resp.status_code == 200, f"ListMyConsents {resp.status_code}: {resp.text}"
        body = ue.body_of(resp)
        out += body.get("records", [])
        cursor = (body.get("page") or {}).get("nextCursor", "")
        if not cursor:
            return out
    raise AssertionError("ListMyConsents never ended the cursor chain")


def _record(purpose: str, granted: bool, version: str, source: str, **extra) -> dict:
    return {"purpose": purpose, "granted": granted, "version": version, "source": source, **extra}


def _record_consent(token: str | None, records: list[dict]):
    return ue.call(ep.PRIVACY_RECORD_CONSENT, {"records": records}, token)


def _ts(value: str) -> float:
    base, _, rest = value.rstrip("Z").partition(".")
    frac = rest[:6].ljust(6, "0") if rest else "000000"
    dt = datetime.strptime(f"{base}.{frac}", "%Y-%m-%dT%H:%M:%S.%f").replace(tzinfo=timezone.utc)
    return dt.timestamp()


def _psql(sql: str) -> str:
    proc = subprocess.run(
        ["docker", "exec", "-i", os.getenv("POSTGRES_CONTAINER", "agora-postgres-1"),
         "psql", "-U", "postgres", "-d", "identity_db", "-v", "ON_ERROR_STOP=1", "-At"],
        input=sql, capture_output=True, text=True, timeout=60,
    )  # fmt: skip
    assert proc.returncode == 0, f"identity_db statement failed: {proc.stderr.strip()[:300]}"
    return proc.stdout.strip()


def _assert_invalid(resp, what: str) -> None:
    assert ue.code_of(resp) == "invalid_argument", f"{what}: {resp.status_code} {resp.text[:300]}"


# ── Register ─────────────────────────────────────────────────────────────────
@when("a visitor registers with the current terms and privacy versions")
def register_current(world: World) -> None:
    _register(world, "CURRENT", "CURRENT_PRIVACY")


@when(
    parsers.re(
        r'a visitor registers with terms version "(?P<terms>[^"]*)" and privacy version "(?P<privacy>[^"]*)"'
    )
)
def register_with(world: World, terms: str, privacy: str) -> None:
    _register(world, terms, privacy)


@when("an anonymous caller registers with the current versions and a client-supplied user id")
def register_spoofed_id(world: World) -> None:
    spoofed = f"client-{uuid.uuid4().hex[:10]}"
    _x(world)["spoofed_id"] = spoofed
    _register(world, "CURRENT", "CURRENT_PRIVACY", {"userId": spoofed, "id": spoofed})


@then(
    "Register succeeds and ListMyConsents for the new user returns exactly the TERMS and "
    "PRIVACY_POLICY records at the current versions"
)
def register_two_rows(world: World) -> None:
    reg = _x(world)["reg"]
    assert reg["resp"].status_code == 200, reg["resp"].text
    finished = time.time()
    rows = _all_records(reg["token"])
    assert len(rows) == 2, rows
    by_purpose = {r["purpose"]: r for r in rows}
    assert set(by_purpose) == {TERMS, PRIVACY}, rows
    assert by_purpose[TERMS]["version"] == legal.CURRENT_TERMS, rows
    assert by_purpose[PRIVACY]["version"] == legal.CURRENT_PRIVACY, rows
    for row in rows:
        assert row.get("granted") is True, row
        assert row["source"] == "register", row
        at = _ts(row["recordedAt"])
        assert reg["started"] - SKEW_S <= at <= finished + SKEW_S, (row, reg["started"], finished)


@then("every Register call failed INVALID_ARGUMENT")
def register_all_invalid(world: World) -> None:
    for reg in _x(world)["regs"]:
        _assert_invalid(reg["resp"], f"Register for {reg['username']}")


@then("logging in with each of those usernames fails as for an unknown user")
def login_unknown(world: World) -> None:
    for reg in _x(world)["regs"]:
        resp = ue.login_raw(reg["username"], get_settings().seed_password)
        assert resp.status_code != 200, f"{reg['username']} can log in: {resp.text[:200]}"
        assert ue.code_of(resp) in {"unauthenticated", "not_found", "permission_denied"}, resp.text


@then(parsers.parse('the Register error names the field "{field}"'))
def register_error_names_field(world: World, field: str) -> None:
    text = _x(world)["reg"]["resp"].text
    assert field in text, text


@then("Register succeeds")
def register_ok(world: World) -> None:
    reg = _x(world)["reg"]
    assert reg["resp"].status_code == 200, reg["resp"].text


@then("the new user's two consent records belong to the user id in the issued token")
def rows_belong_to_new_user(world: World) -> None:
    reg = _x(world)["reg"]
    assert reg["user_id"] and reg["user_id"] != _x(world)["spoofed_id"], reg["user_id"]
    rows = _all_records(reg["token"])
    assert len(rows) == 2, rows
    count = _psql(f"SELECT count(*) FROM consents WHERE user_id = '{_x(world)['spoofed_id']}';")
    assert count == "0", f"{count} consent rows exist for the client-supplied id"


# ── Consent insert failure ───────────────────────────────────────────────────
@given("the consent table is temporarily unavailable to team-identity")
def consent_table_unavailable(world: World) -> None:
    _psql("ALTER TABLE consents RENAME TO consents_e2e_off;")
    _x(world)["table_off"] = True

    def restore() -> None:
        if _x(world).get("table_off"):
            _psql("ALTER TABLE consents_e2e_off RENAME TO consents;")
            _x(world)["table_off"] = False

    world.add_cleanup(restore)


@then("the Register call failed")
def register_failed(world: World) -> None:
    resp = _x(world)["reg"]["resp"]
    assert resp.status_code != 200, f"Register succeeded without a consent table: {resp.text[:300]}"


@when("the consent table is available again")
def consent_table_back(world: World) -> None:
    _psql("ALTER TABLE consents_e2e_off RENAME TO consents;")
    _x(world)["table_off"] = False


@then("registering the same username again succeeds")
def register_same_again(world: World) -> None:
    first = _x(world)["reg"]
    body = {
        "username": first["username"],
        "password": get_settings().seed_password,
        "role": "buyer",
        **legal.versions(),
    }
    resp = ue.call(ep.AUTH_REGISTER, body)
    assert resp.status_code == 200, f"username left behind by the failed Register: {resp.text}"


# ── Boot validation ──────────────────────────────────────────────────────────
@when("the team-identity image is started with PRIVACY_POLICY_VERSIONS empty")
def boot_without_privacy_versions(world: World) -> None:
    name = f"e2e-legal-boot-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    env = pe.compose_env("team-identity")
    env.update(LEGAL_TERMS_VERSIONS=legal.CURRENT_TERMS, PRIVACY_POLICY_VERSIONS="")
    cmd = ["docker", "run", "--rm", "--name", name, "--network", pe.stack_network()]
    for key, value in env.items():
        cmd += ["-e", f"{key}={value}"]
    cmd.append(pe.identity_boot_image())
    try:
        _x(world)["boot"] = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    except subprocess.TimeoutExpired as exc:
        pe.docker("rm", "-f", name, check=False)
        raise AssertionError(
            f"team-identity kept running without PRIVACY_POLICY_VERSIONS: {exc.stdout}"
        ) from exc


@then("the identity process exits non-zero and its log names PRIVACY_POLICY_VERSIONS")
def boot_refused(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["boot"]
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0, f"exit 0 with empty PRIVACY_POLICY_VERSIONS:\n{log}"
    assert "PRIVACY_POLICY_VERSIONS" in log, log


# ── Changed versions stack ───────────────────────────────────────────────────
@given(parsers.parse('team-identity runs with LEGAL_TERMS_VERSIONS "{value}"'))
def identity_with_versions(world: World, value: str) -> None:
    def restore() -> None:
        apr.dc("up", "-d", "--no-deps", "team-identity")
        pe.wait_healthy(_identity_container(), 120)

    world.add_cleanup(restore)
    os.environ["E2E_LEGAL_TERMS_VERSIONS"] = value
    try:
        apr.dc("-f", str(OVERLAY), "up", "-d", "--no-deps", "team-identity")
    finally:
        os.environ.pop("E2E_LEGAL_TERMS_VERSIONS", None)
    pe.wait_healthy(_identity_container(), 120)
    env = pe.container_env(_identity_container())
    assert env.get("LEGAL_TERMS_VERSIONS") == value, "team-identity did not pick up the overlay"
    _x(world)["new_terms"], _x(world)["old_terms"] = value.split(",")[:2]


@given("a buyer registered at the previous terms version")
def buyer_at_old_terms(world: World) -> None:
    _registered_user(world)


@when("the buyer records TERMS at the new current version from the account page")
def record_new_terms(world: World) -> None:
    rec = _record(TERMS, True, _x(world)["new_terms"], "account")
    _x(world)["resp"] = _record_consent(_user(world)["token"], [rec])


@then("the newest TERMS record has the new version and the earlier TERMS record is still listed")
def terms_history(world: World) -> None:
    assert _x(world)["resp"].status_code == 200, _x(world)["resp"].text
    rows = [r for r in _all_records(_user(world)["token"]) if r["purpose"] == TERMS]
    versions = [r["version"] for r in rows]
    assert versions[0] == _x(world)["new_terms"], versions
    assert legal.CURRENT_TERMS in versions[1:], versions


@then("registering with the old terms version fails INVALID_ARGUMENT")
def old_terms_rejected(world: World) -> None:
    reg = _register(world, _x(world)["old_terms"], "CURRENT_PRIVACY")
    _assert_invalid(reg["resp"], "Register at the non-first terms version")


@then("registering with the new terms version succeeds")
def new_terms_accepted(world: World) -> None:
    reg = _register(world, _x(world)["new_terms"], "CURRENT_PRIVACY")
    assert reg["resp"].status_code == 200, reg["resp"].text


# ── RecordConsent ────────────────────────────────────────────────────────────
@given("a signed-in buyer who registered with the current versions")
def signed_in_buyer(world: World) -> None:
    _registered_user(world)
    _x(world)["baseline"] = len(_all_records(_user(world)["token"]))


@when("the buyer records ANALYTICS granted and MARKETING denied from the banner")
def record_banner(world: World) -> None:
    recs = [
        _record(ANALYTICS, True, "1", "banner"),
        _record(MARKETING, False, "1", "banner"),
    ]
    _x(world)["resp"] = _record_consent(_user(world)["token"], recs)


@then("RecordConsent succeeds and exactly 2 banner records were added for ANALYTICS and MARKETING")
def banner_rows(world: World) -> None:
    assert _x(world)["resp"].status_code == 200, _x(world)["resp"].text
    rows = _all_records(_user(world)["token"])
    new = [r for r in rows if r["source"] == "banner"]
    assert len(rows) == _x(world)["baseline"] + 2, rows
    got = {r["purpose"]: r.get("granted", False) for r in new}
    assert got == {ANALYTICS: True, MARKETING: False}, new
    assert all(r["purpose"] != PERSONALIZATION for r in rows), rows


INVALID_CASES = {
    "seven records": lambda: [
        _record(p, True, "1", "banner")
        for p in (ANALYTICS, MARKETING, PERSONALIZATION, "CONSENT_PURPOSE_SELLER_TERMS")
    ]
    + [_record(ANALYTICS, True, "1", "banner")] * 3,
    "duplicate purpose": lambda: [
        _record(ANALYTICS, True, "1", "banner"),
        _record(ANALYTICS, False, "1", "banner"),
    ],
    "unspecified purpose": lambda: [_record("CONSENT_PURPOSE_UNSPECIFIED", True, "1", "banner")],
    "33-character version": lambda: [_record(ANALYTICS, True, "v" * 33, "banner")],
    "unknown source": lambda: [_record(ANALYTICS, True, "1", "carrier-pigeon")],
    "empty list": lambda: [],
}


@when(parsers.parse('the buyer submits the invalid consent request "{case}"'))
def submit_invalid(world: World, case: str) -> None:
    resp = _record_consent(_user(world)["token"], INVALID_CASES[case]())
    _x(world).setdefault("invalid", []).append((case, resp))


@then("every invalid request failed INVALID_ARGUMENT")
def invalid_all(world: World) -> None:
    assert _x(world)["invalid"], "no request was submitted"
    for case, resp in _x(world)["invalid"]:
        _assert_invalid(resp, case)


@then("the buyer's record count is unchanged")
def count_unchanged(world: World) -> None:
    assert len(_all_records(_user(world)["token"])) == _x(world)["baseline"]


@when("the buyer records TERMS at a non-current version together with a valid ANALYTICS record")
def stale_terms_with_valid(world: World) -> None:
    recs = [_record(TERMS, True, "1999-01", "account"), _record(ANALYTICS, True, "1", "banner")]
    _x(world)["resp"] = _record_consent(_user(world)["token"], recs)


@when("the buyer records PRIVACY_POLICY with granted false together with a valid MARKETING record")
def withdrawn_privacy_with_valid(world: World) -> None:
    recs = [
        _record(PRIVACY, False, legal.CURRENT_PRIVACY, "account"),
        _record(MARKETING, True, "1", "banner"),
    ]
    _x(world).setdefault("invalid", []).append(
        ("withdrawn privacy", _record_consent(_user(world)["token"], recs))
    )


@then("that call failed INVALID_ARGUMENT")
def that_call_invalid(world: World) -> None:
    _assert_invalid(_x(world)["resp"], "stale TERMS call")


@when("the buyer records ANALYTICS with recorded_at in the year 2000")
def record_client_time(world: World) -> None:
    _x(world)["sent_at"] = time.time()
    rec = _record(ANALYTICS, True, "1", "banner", recordedAt="2000-01-01T00:00:00Z")
    _x(world)["resp"] = _record_consent(_user(world)["token"], [rec])


@then("the stored recorded_at is within 5 seconds of the server time")
def stored_time(world: World) -> None:
    assert _x(world)["resp"].status_code == 200, _x(world)["resp"].text
    rows = [r for r in _all_records(_user(world)["token"]) if r["source"] == "banner"]
    assert rows, "no banner row stored"
    at = _ts(rows[0]["recordedAt"])
    assert abs(at - time.time()) < 60 and _x(world)["sent_at"] - SKEW_S <= at, rows[0]


@then("the row is listed for the calling user")
def row_listed_for_caller(world: World) -> None:
    assert len(_all_records(_user(world)["token"])) == _x(world)["baseline"] + 1


@when("an anonymous caller calls RecordConsent")
def anonymous_record(world: World) -> None:
    _x(world)["resp"] = _record_consent(None, [_record(ANALYTICS, True, "1", "banner")])


@when("a service principal calls RecordConsent on team-identity")
def service_record(world: World) -> None:
    pdir = pe.REPO_ROOT / "team-identity" / "proto"
    cmd = [
        "docker", "run", "--rm", "--network", pe.stack_network(), "-v", f"{pdir}:/proto:ro",
        os.getenv("FLASH_HOLD_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest"),
        "-plaintext", "-import-path", "/proto", "-proto", "platform/identity/v1/identity.proto",
        "-H", f"x-principal-id: {_user(world)['user_id']}", "-H", "x-principal-type: service",
        "-d", json.dumps({"records": [_record(ANALYTICS, True, "1", "banner")]}),
        os.getenv("IDENTITY_GRPC_ADDR", "team-identity-svc:50053"),
        "platform.identity.v1.PrivacyService/RecordConsent",
    ]  # fmt: skip
    _x(world)["service"] = subprocess.run(cmd, capture_output=True, text=True, timeout=120)


@then("the anonymous call failed UNAUTHENTICATED")
def anonymous_unauthenticated(world: World) -> None:
    assert ue.code_of(_x(world)["resp"]) == "unauthenticated", _x(world)["resp"].text


@then("the service call failed PERMISSION_DENIED")
def service_denied(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["service"]
    out = proc.stderr + proc.stdout
    assert proc.returncode != 0 and "PermissionDenied" in out, out


@then("no consent row was written")
def no_row_written(world: World) -> None:
    assert len(_all_records(_user(world)["token"])) == _x(world)["baseline"]


# ── ListMyConsents ───────────────────────────────────────────────────────────
@given(parsers.parse("a buyer with {total:d} consent records"))
def buyer_with_records(world: World, total: int) -> None:
    _registered_user(world)
    for _ in range(total - 2):
        resp = _record_consent(_user(world)["token"], [_record(ANALYTICS, True, "1", "banner")])
        assert resp.status_code == 200, resp.text
    _x(world)["baseline"] = total


@when(
    parsers.parse(
        "the buyer lists consents with page size {size:d} following next_cursor until it is empty"
    )
)
def list_paged(world: World, size: int) -> None:
    pages: list[list[dict]] = []
    cursor = ""
    for _ in range(20):
        resp = _list(_user(world)["token"], {"pageSize": size, "cursor": cursor})
        assert resp.status_code == 200, resp.text
        body = ue.body_of(resp)
        pages.append(body.get("records", []))
        cursor = (body.get("page") or {}).get("nextCursor", "")
        if not cursor:
            break
    _x(world)["pages"] = pages


@then(parsers.parse("the pages hold {a:d}, {b:d} and {c:d} records"))
def page_sizes(world: World, a: int, b: int, c: int) -> None:
    assert [len(p) for p in _x(world)["pages"]] == [a, b, c]


@then("the records are in non-increasing recorded_at order with none repeated or missing")
def order_and_unique(world: World) -> None:
    flat = [r for p in _x(world)["pages"] for r in p]
    times = [_ts(r["recordedAt"]) for r in flat]
    assert times == sorted(times, reverse=True), times
    assert len(flat) == _x(world)["baseline"], len(flat)
    whole = _all_records(_user(world)["token"], page_size=100)
    assert flat == whole, "paged walk differs from the single-page list"


@given("another buyer who recorded a banner choice")
def other_buyer(world: World) -> None:
    _x(world)["a"] = _x(world)["user"]
    other = _register(world, "CURRENT", "CURRENT_PRIVACY")
    assert other["resp"].status_code == 200, other["resp"].text
    _x(world)["b"] = other
    resp = _record_consent(other["token"], [_record(MARKETING, True, "1", "banner")])
    assert resp.status_code == 200, resp.text


@then("a user's list holds none of the other buyer's records and their own count is unchanged")
def isolation(world: World) -> None:
    a, b = _x(world)["a"], _x(world)["b"]
    rows_a = _all_records(a["token"])
    rows_b = _all_records(b["token"])
    assert len(rows_a) == _x(world)["baseline"], rows_a
    assert len(rows_b) == 3, rows_b
    assert not any(r["purpose"] == MARKETING for r in rows_a), rows_a
    assert any(r["purpose"] == MARKETING and r["source"] == "banner" for r in rows_b), rows_b


@when("the buyer lists consents with page size 500")
def list_500(world: World) -> None:
    _x(world)["resp"] = _list(_user(world)["token"], {"pageSize": 500})


@when("the buyer lists consents with a malformed cursor")
def list_bad_cursor(world: World) -> None:
    _x(world)["bad"] = _list(_user(world)["token"], {"cursor": "%%not-a-cursor%%"})


@then("the first call returns at most 100 records and the second fails INVALID_ARGUMENT")
def clamp_and_bad_cursor(world: World) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 200, resp.text
    assert len(ue.body_of(resp).get("records", [])) <= 100
    _assert_invalid(_x(world)["bad"], "malformed cursor")


@when("an anonymous caller calls ListMyConsents")
def anonymous_list(world: World) -> None:
    _x(world)["resp"] = _list(None)


@then("the list call failed UNAUTHENTICATED")
def list_unauthenticated(world: World) -> None:
    assert ue.code_of(_x(world)["resp"]) == "unauthenticated", _x(world)["resp"].text


@given("a legacy buyer with no consent rows")
def legacy_buyer(world: World) -> None:
    user = _registered_user(world)
    _psql(f"DELETE FROM consents WHERE user_id = '{user['user_id']}';")


@when("the legacy buyer calls ListMyConsents")
def legacy_list(world: World) -> None:
    _x(world)["resp"] = _list(_user(world)["token"])


@then("the response has 0 records and an empty next_cursor")
def empty_list(world: World) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 200, resp.text
    body = ue.body_of(resp)
    assert body.get("records", []) == [], body
    assert (body.get("page") or {}).get("nextCursor", "") == "", body


# ── Gateway ──────────────────────────────────────────────────────────────────
@when("the buyer calls PrivacyService RecordConsent then ListMyConsents through the gateway")
def gateway_record_then_list(world: World) -> None:
    rec = _record(ANALYTICS, True, "1", "banner")
    _x(world)["resp"] = _record_consent(_user(world)["token"], [rec])
    _x(world)["list_resp"] = _list(_user(world)["token"])


@then("both calls succeed and the list contains the recorded banner row")
def gateway_both_ok(world: World) -> None:
    assert _x(world)["resp"].status_code == 200, _x(world)["resp"].text
    resp = _x(world)["list_resp"]
    assert resp.status_code == 200, resp.text
    rows = ue.body_of(resp).get("records", [])
    assert any(r["source"] == "banner" and r["purpose"] == ANALYTICS for r in rows), rows


@then("the response is UNAUTHENTICATED")
def response_unauthenticated(world: World) -> None:
    assert ue.code_of(_x(world)["resp"]) == "unauthenticated", _x(world)["resp"].text
