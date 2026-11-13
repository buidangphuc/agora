"""KYC documents (change verification-kyc-documents): upload, submit, review, queue, badges, retention, erasure.

Drives the gateway Connect API; storage, verification_db clock shifts and Kafka go through
support/kyc_documents_support. Sellers are named ("A", "B", ...) per scenario; the admin is the seeded one
("an admin is logged in" lives in common_steps).
"""

from __future__ import annotations

import threading
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services.verification_service import VerificationService
from tests.e2e.support import audit_support as aud
from tests.e2e.support import kyc_documents_support as kyc
from tests.e2e.support.world import World

TARGET = "kyc_submission"
PENDING = "VERIFICATION_STATUS_PENDING"


def _x(world: World) -> dict:
    return world.state.extra


def _people(world: World) -> dict:
    return _x(world).setdefault("kyc_people", {})


def _person(world: World, name: str) -> dict:
    people = _people(world)
    if name not in people:
        token, user_id = kyc.register(world, "seller")
        people[name] = {
            "token": token,
            "id": user_id,
            "svc": VerificationService(token=token),
            "refs": [],
            "subs": [],
            "sub_refs": [],
            "sub_bodies": [],
        }
    return people[name]


def _admin_svc(world: World) -> VerificationService:
    user = world.state.current_user
    token = user.token if user is not None and user.token else aud.admin_token(world)
    _x(world)["kyc_admin_token"] = token
    return VerificationService(token=token)


def _anon() -> VerificationService:
    return VerificationService(token=None)


def _set(world: World, resp: httpx.Response) -> httpx.Response:
    _x(world)["resp"] = resp
    return resp


def _json(resp: httpx.Response) -> dict:
    try:
        data = resp.json()
    except ValueError:
        return {}
    return data if isinstance(data, dict) else {}


def _body(world: World) -> dict:
    return _json(_x(world)["resp"])


def _last_sub(person: dict) -> str:
    return person["subs"][-1]


def _submit(person: dict, docs: int = 1) -> str:
    sub_id, refs, bodies = kyc.submit_with_uploads(person["svc"], docs)
    person["subs"].append(sub_id)
    person["sub_refs"].append(refs)
    person["sub_bodies"].append(bodies)
    return sub_id


def _review(world: World, sub_id: str, decision: str, reason: str | None = None) -> httpx.Response:
    return _admin_svc(world).review_kyc_response(sub_id, decision, reason)


def _admin_event_count(world: World, sub_id: str, action: str) -> list[dict]:
    found = aud.events_for(world, TARGET, sub_id, action=action)
    return [e for e in found if e.get("action") == action]


# ── Givens ────────────────────────────────────────────────────────────────


@given(parsers.parse('seller "{name}" is registered'))
def seller_registered(world: World, name: str) -> None:
    _person(world, name)


@given(parsers.parse('seller "{name}" has uploaded {n:d} documents'))
def seller_uploaded(world: World, name: str, n: int) -> None:
    p = _person(world, name)
    p["refs"] = []
    p["bodies"] = []
    for _ in range(n):
        ref, body = kyc.upload_doc(p["svc"])
        p["refs"].append(ref)
        p["bodies"].append(body)


@given(parsers.parse('seller "{name}" holds {n:d} unused unexpired upload grants'))
def seller_holds_grants(world: World, name: str, n: int) -> None:
    p = _person(world, name)
    p["refs"] = []
    for _ in range(n):
        resp = p["svc"].create_kyc_upload_url_response("image/jpeg", 1048576)
        assert resp.status_code == 200, resp.text
        p["refs"].append(resp.json()["docRef"])
    _x(world)["granted_size"] = 1048576


@given(parsers.parse("an object of {size:d} bytes is stored directly under the granted key"))
def object_under_granted_key(world: World, size: int) -> None:
    ref = _person(world, "A")["refs"][0]
    kyc.s3().put_object(Bucket=kyc.BUCKET, Key=ref, Body=b"x" * size, ContentType="image/jpeg")


@given(parsers.parse('seller "{name}" has submitted KYC with the uploaded documents'))
def seller_has_submitted(world: World, name: str) -> None:
    p = _person(world, name)
    resp = p["svc"].submit_kyc_response("national_id", p["refs"][0], p["refs"][1:])
    assert resp.status_code == 200, resp.text
    p["subs"].append(resp.json()["id"])
    p["sub_refs"].append(list(p["refs"]))


@given(parsers.parse('seller "{name}" has a pending KYC submission'))
def seller_has_pending(world: World, name: str) -> None:
    _submit(_person(world, name))


@given(parsers.parse('seller "{name}" has a second KYC submission'))
def seller_has_second(world: World, name: str) -> None:
    _submit(_person(world, name))


@given(parsers.parse('seller "{name}" has a verified KYC submission'))
def seller_has_verified(world: World, name: str) -> None:
    p = _person(world, name)
    sub_id = _submit(p)
    resp = _review(world, sub_id, "approve")
    assert resp.status_code == 200, resp.text


@given(parsers.parse('seller "{name}" has a rejected KYC submission with reason_code "{reason}"'))
def seller_has_rejected(world: World, name: str, reason: str) -> None:
    p = _person(world, name)
    sub_id = _submit(p)
    resp = _review(world, sub_id, "reject", reason)
    assert resp.status_code == 200, resp.text


@given(
    parsers.parse(
        'seller "{name}" has a rejected second KYC submission with reason_code "{reason}"'
    )
)
def seller_has_rejected_second(world: World, name: str, reason: str) -> None:
    p = _person(world, name)
    sub_id = _submit(p)
    resp = _review(world, sub_id, "reject", reason)
    assert resp.status_code == 200, resp.text


@given("the admin has a pending KYC submission")
def admin_has_pending(world: World) -> None:
    svc = _admin_svc(world)
    sub_id, _, _ = kyc.submit_with_uploads(svc, 1)
    _x(world)["admin_sub"] = sub_id


@given(parsers.parse("{n:d} sellers each have a pending KYC submission"))
def many_pending(world: World, n: int) -> None:
    ids: list[str] = []
    for i in range(n):
        p = _person(world, f"Q{i}")
        ids.append(_submit(p))
    _x(world)["queue_ids"] = ids


@given(parsers.parse('the grants of seller "{name}" were issued {m:d} minutes ago'))
def grants_issued_ago(world: World, name: str, m: int) -> None:
    uid = _person(world, name)["id"]
    kyc.psql(
        f"UPDATE kyc_upload_grants SET issued_at = now() - interval '{m} minutes', "
        f"expires_at = now() - interval '{m - 15} minutes' WHERE user_id = {kyc.lit(uid)}"
    )


def _mark_sweep(world: World, name: str, sub_idx: int, days: int) -> None:
    p = _person(world, name)
    if days > 90:
        _x(world).setdefault("sweep_keys", []).extend(p["sub_refs"][sub_idx])


@given(parsers.parse('the rejection of seller "{name}" happened {days:d} days ago'))
def rejection_ago(world: World, name: str, days: int) -> None:
    p = _person(world, name)
    kyc.psql(
        f"UPDATE kyc_submissions SET reviewed_at = now() - interval '{days} days' "
        f"WHERE id = {kyc.lit(_last_sub(p))}"
    )
    _mark_sweep(world, name, -1, days)


@given(parsers.parse('the approval of seller "{name}" happened {days:d} days ago'))
def approval_ago(world: World, name: str, days: int) -> None:
    p = _person(world, name)
    kyc.psql(
        f"UPDATE kyc_submissions SET reviewed_at = now() - interval '{days} days' "
        f"WHERE id = {kyc.lit(_last_sub(p))}"
    )


@given("a rejected sweep marker submission rejected 91 days ago")
def sweep_marker(world: World) -> None:
    p = _person(world, "M")
    sub_id = _submit(p)
    assert _review(world, sub_id, "reject", "document_unreadable").status_code == 200
    kyc.psql(
        f"UPDATE kyc_submissions SET reviewed_at = now() - interval '91 days' WHERE id = {kyc.lit(sub_id)}"
    )
    _x(world).setdefault("sweep_keys", []).extend(p["sub_refs"][-1])


@given(parsers.parse('the first submission of seller "{name}" was superseded {days:d} days ago'))
def first_superseded(world: World, name: str, days: int) -> None:
    p = _person(world, name)
    kyc.psql(
        f"UPDATE kyc_submissions SET superseded_at = now() - interval '{days} days' "
        f"WHERE id = {kyc.lit(p['subs'][0])}"
    )
    _mark_sweep(world, name, 0, days)


# ── Whens ─────────────────────────────────────────────────────────────────


@when(parsers.parse('seller "{name}" requests an upload URL for "{ctype}" of {size:d} bytes'))
def seller_requests_url(world: World, name: str, ctype: str, size: int) -> None:
    p = _person(world, name)
    resp = _set(world, p["svc"].create_kyc_upload_url_response(ctype, size))
    if resp.status_code == 200:
        p["grant"] = resp.json()
        p["grant_size"] = size


@when(parsers.parse('an anonymous caller requests an upload URL for "{ctype}" of {size:d} bytes'))
def anon_requests_url(world: World, ctype: str, size: int) -> None:
    _set(world, _anon().create_kyc_upload_url_response(ctype, size))


@when(parsers.parse('seller "{name}" uploads {size:d} bytes of "{ctype}" to the upload URL'))
def seller_uploads(world: World, name: str, size: int, ctype: str) -> None:
    grant = _person(world, name).get("grant")
    assert grant, f"no upload grant was issued: {_x(world)['resp'].status_code} {_x(world)['resp'].text}"
    _x(world)["storage_resp"] = kyc.http_put(grant["uploadUrl"], b"x" * size, ctype)


@when(parsers.parse('seller "{name}" submits KYC with the uploaded documents'))
def seller_submits(world: World, name: str) -> None:
    p = _person(world, name)
    resp = _set(world, p["svc"].submit_kyc_response("national_id", p["refs"][0], p["refs"][1:]))
    if resp.status_code == 200:
        p["subs"].append(resp.json()["id"])
        p["sub_refs"].append(list(p["refs"]))


@when(parsers.parse('seller "{name}" submits KYC with the granted keys'))
def seller_submits_granted(world: World, name: str) -> None:
    p = _person(world, name)
    _set(world, p["svc"].submit_kyc_response("national_id", p["refs"][0], p["refs"][1:]))


@when(parsers.parse('seller "{name}" submits KYC with the documents of seller "{other}"'))
def seller_submits_foreign(world: World, name: str, other: str) -> None:
    owner = _person(world, other)
    _set(world, _person(world, name)["svc"].submit_kyc_response("national_id", owner["refs"][0]))
    unknown = f"kyc/{_person(world, name)['id']}/{uuid.uuid4()}"
    _x(world)["unknown_key_resp"] = _person(world, name)["svc"].submit_kyc_response(
        "national_id", unknown
    )


@when(parsers.parse('seller "{name}" submits KYC with the free-text doc_ref "{ref}"'))
def seller_submits_free_text(world: World, name: str, ref: str) -> None:
    _set(world, _person(world, name)["svc"].submit_kyc_response("national_id", ref))


@when(parsers.parse('an anonymous caller submits KYC with the free-text doc_ref "{ref}"'))
def anon_submits(world: World, ref: str) -> None:
    _set(world, _anon().submit_kyc_response("national_id", ref))


@when(parsers.parse('seller "{name}" submits KYC listing the first key twice'))
def seller_submits_dup(world: World, name: str) -> None:
    p = _person(world, name)
    _set(world, p["svc"].submit_kyc_response("national_id", p["refs"][0], [p["refs"][0]]))


@when(parsers.parse('seller "{name}" submits KYC with the uploaded documents twice concurrently'))
def seller_submits_concurrently(world: World, name: str) -> None:
    p = _person(world, name)
    barrier = threading.Barrier(2)
    results: list[httpx.Response] = []

    def call() -> None:
        svc = VerificationService(token=p["token"])
        barrier.wait()
        results.append(svc.submit_kyc_response("national_id", p["refs"][0]))

    threads = [threading.Thread(target=call) for _ in range(2)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    _x(world)["concurrent"] = results


@when(parsers.parse('seller "{name}" reads verification status "{status}" with an empty user_id'))
def seller_reads_own_empty(world: World, name: str, status: str) -> None:
    _x(world)["expected_status"] = status
    _set(world, _person(world, name)["svc"].get_verification_status_response())


@when(parsers.parse('seller "{name}" reads the verification status of seller "{other}"'))
def seller_reads_other(world: World, name: str, other: str) -> None:
    _set(
        world,
        _person(world, name)["svc"].get_verification_status_response(_person(world, other)["id"]),
    )


def _decide(world: World, name: str, decision: str, reason: str | None) -> None:
    _set(world, _review(world, _last_sub(_person(world, name)), decision, reason))


@when(
    parsers.parse('the admin rejects the submission of seller "{name}" with reason_code "{reason}"')
)
def admin_rejects(world: World, name: str, reason: str) -> None:
    _decide(world, name, "reject", reason)


@when(parsers.parse('the admin rejects the submission of seller "{name}" with reason_code ""'))
def admin_rejects_empty(world: World, name: str) -> None:
    _decide(world, name, "reject", "")


@when(
    parsers.parse(
        'the admin rejects the submission of seller "{name}" with a reason_code of {n:d} characters'
    )
)
def admin_rejects_long(world: World, name: str, n: int) -> None:
    _decide(world, name, "reject", "a" * n)


@when(
    parsers.parse(
        'the admin approves the submission of seller "{name}" with reason_code "{reason}"'
    )
)
def admin_approves_with_reason(world: World, name: str, reason: str) -> None:
    _decide(world, name, "approve", reason)


@when("the admin approves their own submission")
def admin_approves_own(world: World) -> None:
    _set(world, _review(world, _x(world)["admin_sub"], "approve"))


@when(
    parsers.parse("the admin walks the PENDING queue with page_size {size:d} following next_cursor")
)
def admin_walks_queue(world: World, size: int) -> None:
    svc = _admin_svc(world)
    pages: list[list[dict]] = []
    cursors: list[str] = []
    cursor: str | None = None
    for _ in range(5000):
        resp = svc.list_kyc_submissions_response(PENDING, size, cursor)
        assert resp.status_code == 200, f"ListKycSubmissions failed: {resp.status_code} {resp.text}"
        data = resp.json()
        pages.append(data.get("submissions", []))
        cursor = (data.get("page") or {}).get("nextCursor") or None
        cursors.append(cursor or "")
        if not cursor:
            break
    _x(world)["pages"] = pages
    _x(world)["page_cursors"] = cursors


@when(parsers.parse("the admin lists the queue with page_size {size:d}"))
def admin_lists_size(world: World, size: int) -> None:
    _set(world, _admin_svc(world).list_kyc_submissions_response(page_size=size))


@when(parsers.parse('the admin lists the queue with cursor "{cursor}"'))
def admin_lists_cursor(world: World, cursor: str) -> None:
    _set(world, _admin_svc(world).list_kyc_submissions_response(cursor=cursor))


@when(parsers.parse('the admin lists the queue with status "{status}"'))
def admin_lists_status(world: World, status: str) -> None:
    _set(world, _admin_svc(world).list_kyc_submissions_response(status=status))


@when(parsers.parse('seller "{name}" lists the queue'))
def seller_lists(world: World, name: str) -> None:
    _set(world, _person(world, name)["svc"].list_kyc_submissions_response())


@when("an anonymous caller lists the queue")
def anon_lists(world: World) -> None:
    _set(world, _anon().list_kyc_submissions_response())


@when(parsers.parse('the admin requests document {idx:d} of the submission of seller "{name}"'))
def admin_requests_doc(world: World, idx: int, name: str) -> None:
    resp = _set(
        world, _admin_svc(world).get_kyc_document_url_response(_last_sub(_person(world, name)), idx)
    )
    _x(world)["doc_asked_at"] = time.monotonic()
    if resp.status_code == 200:
        _x(world)["doc_url"] = resp.json().get("url")


@when(parsers.parse("the admin requests document {idx:d} of an unknown submission"))
def admin_requests_unknown(world: World, idx: int) -> None:
    _set(world, _admin_svc(world).get_kyc_document_url_response(str(uuid.uuid4()), idx))


@when(parsers.parse('seller "{name}" requests document {idx:d} of their own submission'))
def seller_requests_own(world: World, name: str, idx: int) -> None:
    p = _person(world, name)
    _set(world, p["svc"].get_kyc_document_url_response(_last_sub(p), idx))


@when(
    parsers.parse('seller "{name}" requests document {idx:d} of the submission of seller "{owner}"')
)
def seller_requests_other(world: World, name: str, idx: int, owner: str) -> None:
    _set(
        world,
        _person(world, name)["svc"].get_kyc_document_url_response(
            _last_sub(_person(world, owner)), idx
        ),
    )


@when(parsers.parse('seller "{name}" requests document {idx:d} of an unknown submission'))
def seller_requests_unknown(world: World, name: str, idx: int) -> None:
    _set(world, _person(world, name)["svc"].get_kyc_document_url_response(str(uuid.uuid4()), idx))


@when(
    parsers.parse(
        'an anonymous caller requests document {idx:d} of the submission of seller "{owner}"'
    )
)
def anon_requests_doc(world: World, idx: int, owner: str) -> None:
    _set(world, _anon().get_kyc_document_url_response(_last_sub(_person(world, owner)), idx))


@when("team-audit is stopped")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@when("team-audit is started again")
def audit_started(world: World) -> None:
    aud.start_audit(world)


@when("more than 5 minutes pass since the document link was issued")
def wait_five_minutes(world: World) -> None:
    started = _x(world)["doc_asked_at"]
    kyc.poll(
        lambda: time.monotonic() - started > 5 * 60 + 5, 6 * 60, "the 5 minute link expiry", 5.0
    )


@when(
    parsers.parse(
        'an anonymous client requests the first object of seller "{name}" without a signature'
    )
)
def anon_object(world: World, name: str) -> None:
    key = _person(world, name)["sub_refs"][0][0]
    _x(world)["storage_resp"] = httpx.get(f"{kyc.s3_endpoint()}/{kyc.BUCKET}/{key}", timeout=30)


@when(
    parsers.parse(
        'an anonymous caller asks for the public badges of sellers "{a}", "{b}", "{c}" and an unknown id'
    )
)
def anon_badges_named(world: World, a: str, b: str, c: str) -> None:
    ids = [_person(world, n)["id"] for n in (a, b, c)] + [str(uuid.uuid4())]
    _x(world)["badge_ids"] = ids
    _set(world, _anon().get_public_badges_response(ids))


@when(parsers.parse("an anonymous caller asks for the public badges of {n:d} ids"))
def anon_badges_n(world: World, n: int) -> None:
    ids = [str(uuid.uuid4()) for _ in range(n)]
    _x(world)["badge_ids"] = ids
    _set(world, _anon().get_public_badges_response(ids))


@when("the retention sweeper has run")
def sweeper_has_run(world: World) -> None:
    keys = _x(world)["sweep_keys"]
    kyc.poll(
        lambda: not any(kyc.object_exists(k) for k in keys),
        kyc.sweep_wait_s(),
        "the retention sweeper to delete the 91 day old objects",
    )


@when('UserErased for seller "A" is published on identity.events')
def publish_erased(world: World) -> None:
    _x(world)["erased_event"] = kyc.publish_user_erased(_person(world, "A")["id"])


@when('UserErased for seller "A" is published on identity.events twice with the same event id')
def publish_erased_twice(world: World) -> None:
    uid = _person(world, "A")["id"]
    eid = kyc.publish_user_erased(uid)
    kyc.publish_user_erased(uid, event_id=eid)


# ── Thens ─────────────────────────────────────────────────────────────────


@then("the call succeeds")
def call_succeeds(world: World) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text}"


@then(parsers.parse('the call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code >= 400, f"expected failure {code}, got {resp.status_code}: {resp.text}"
    got = _json(resp).get("code")
    assert got == code, f"expected code {code}, got {got}: {resp.status_code} {resp.text}"


@then(parsers.parse("the gateway answers {status:d}"))
def gateway_answers(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then('the doc_ref matches "kyc/<A id>/<uuid>"')
def doc_ref_shape(world: World) -> None:
    ref = _body(world).get("docRef", "")
    uid = _person(world, "A")["id"]
    prefix = f"kyc/{uid}/"
    assert ref.startswith(prefix), f"doc_ref {ref!r} does not start with {prefix!r}"
    uuid.UUID(ref[len(prefix) :])


@then("the upload expires in 15 minutes")
def upload_expiry(world: World) -> None:
    ahead = kyc.seconds_ahead(_body(world)["expiresAt"])
    assert abs(ahead - 15 * 60) <= 30, f"expires_at is {ahead:.0f}s ahead, want 900 +-30"


@then(
    parsers.parse(
        'seller "{name}" uploading {size:d} bytes of "{ctype}" to the upload URL returns 2xx'
    )
)
def upload_returns_2xx(world: World, name: str, size: int, ctype: str) -> None:
    grant = _person(world, name)["grant"]
    resp = kyc.http_put(grant["uploadUrl"], b"x" * size, ctype)
    assert 200 <= resp.status_code < 300, f"PUT answered {resp.status_code}: {resp.text}"


@then(parsers.parse("the storage endpoint answers {status:d}"))
def storage_answers(world: World, status: int) -> None:
    resp: httpx.Response = _x(world)["storage_resp"]
    assert resp.status_code == status, f"storage answered {resp.status_code}: {resp.text[:300]}"


@then("no object exists under the doc_ref")
def no_object_under_doc_ref(world: World) -> None:
    ref = _person(world, "A")["grant"]["docRef"]
    assert not kyc.object_exists(ref), f"object {ref} must not exist"


@then("no object exists under the granted key")
def no_object_under_granted(world: World) -> None:
    ref = _person(world, "A")["refs"][0]
    assert not kyc.object_exists(ref), f"object {ref} must have been removed"


@then(parsers.parse('seller "{name}" has {n:d} upload grants'))
def grants_count(world: World, name: str, n: int) -> None:
    assert kyc.count("kyc_upload_grants", _person(world, name)["id"]) == n


@then(parsers.parse('seller "{name}" has {n:d} used upload grants'))
def used_grants_count(world: World, name: str, n: int) -> None:
    uid = kyc.lit(_person(world, name)["id"])
    got = int(
        kyc.psql(
            f"SELECT count(*) FROM kyc_upload_grants WHERE user_id = {uid} AND used_at IS NOT NULL"
        )
    )
    assert got == n, f"used grants {got}, want {n}"


@then(parsers.parse('seller "{name}" has {n:d} submissions'))
def submissions_count(world: World, name: str, n: int) -> None:
    assert kyc.count("kyc_submissions", _person(world, name)["id"]) == n


@then(parsers.parse('the submission status is "{status}"'))
def submission_status(world: World, status: str) -> None:
    assert _body(world).get("status") == status, _body(world)


@then(parsers.parse('seller "{name}" reads verification status "{status}"'))
def seller_reads_status(world: World, name: str, status: str) -> None:
    res = _person(world, name)["svc"].get_verification_status()
    assert res.get("status") == status, f"status {res}, want {status}"
    _x(world)["status_res"] = res


@then(parsers.parse('the admin reads verification status "{status}"'))
def admin_reads_status(world: World, status: str) -> None:
    res = _admin_svc(world).get_verification_status()
    assert res.get("status") == status, f"status {res}, want {status}"


@then(parsers.parse('the reject_reason_code is "{code}"'))
def reject_reason(world: World, code: str) -> None:
    assert _x(world)["status_res"].get("rejectReasonCode") == code, _x(world)["status_res"]


@then("the status response has badge false and an empty reject_reason_code")
def status_no_badge(world: World) -> None:
    resp: httpx.Response = _x(world)["resp"]
    assert resp.status_code == 200, resp.text
    body = _json(resp)
    assert body.get("status") == _x(world)["expected_status"], body
    assert not body.get("badge"), body
    assert not body.get("rejectReasonCode"), body


@then("the error message equals the one for an unknown key")
def same_error_message(world: World) -> None:
    foreign = _json(_x(world)["resp"]).get("message")
    unknown = _json(_x(world)["unknown_key_resp"]).get("message")
    assert (
        foreign and foreign == unknown
    ), f"foreign key message {foreign!r} != unknown key message {unknown!r}"


@then(parsers.parse('exactly 1 call succeeds and the other fails with code "{code}"'))
def one_wins(world: World, code: str) -> None:
    results: list[httpx.Response] = _x(world)["concurrent"]
    ok = [r for r in results if r.status_code == 200]
    bad = [r for r in results if r.status_code != 200]
    assert len(ok) == 1 and len(bad) == 1, [(r.status_code, r.text) for r in results]
    assert _json(bad[0]).get("code") == code, bad[0].text


@then(parsers.parse('the admin queue lists seller "{name}" submission with document_count {n:d}'))
def queue_document_count(world: World, name: str, n: int) -> None:
    p = _person(world, name)
    svc = _admin_svc(world)
    cursor: str | None = None
    for _ in range(5000):
        resp = svc.list_kyc_submissions_response(PENDING, 100, cursor)
        assert resp.status_code == 200, resp.text
        data = resp.json()
        for sub in data.get("submissions", []):
            if sub.get("id") == _last_sub(p):
                assert int(sub.get("documentCount", 0)) == n, sub
                return
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            break
    raise AssertionError(f"submission {_last_sub(p)} not found in the PENDING queue")


@then(
    parsers.parse(
        'exactly one kyc.review audit event exists for the submission of seller "{name}" with reason_code "{reason}"'
    )
)
def review_event_reason(world: World, name: str, reason: str) -> None:
    p = _person(world, name)
    aud.assert_one_event(
        world,
        target_type=TARGET,
        target_id=_last_sub(p),
        action="kyc.review",
        actor_id=kyc.jwt_sub(_x(world)["kyc_admin_token"]),
        metadata={"reason_code": reason},
    )


@then(
    "every page but the last holds 10 submissions in ascending created_at order without duplicates"
)
def pages_shape(world: World) -> None:
    pages = _x(world)["pages"]
    assert (
        len(pages) >= 3
    ), f"expected at least 3 pages for 25 pending submissions, got {len(pages)}"
    assert all(len(p) == 10 for p in pages[:-1]), [len(p) for p in pages]
    assert 1 <= len(pages[-1]) <= 10, [len(p) for p in pages]
    flat = [s for page in pages for s in page]
    ids = [s["id"] for s in flat]
    assert len(ids) == len(set(ids)), "duplicate submission across pages"
    stamps = [kyc.parse_ts(s["createdAt"]) for s in flat]
    assert stamps == sorted(stamps), "submissions are not in ascending created_at order"


@then("the last page has an empty next_cursor")
def last_cursor_empty(world: World) -> None:
    assert _x(world)["page_cursors"][-1] == ""


@then("the 25 submissions appear in submission order at the end of the queue")
def queue_tail_order(world: World) -> None:
    flat = [s["id"] for page in _x(world)["pages"] for s in page]
    want = _x(world)["queue_ids"]
    assert (
        flat[-len(want) :] == want
    ), "the seeded submissions are not the last entries in submission order"


@then("no listed submission carries a document key")
def no_keys_listed(world: World) -> None:
    blob = str(_x(world)["pages"])
    assert "kyc/" not in blob, "ListKycSubmissions must not return document keys"


@then("at most 100 submissions are returned")
def at_most_100(world: World) -> None:
    assert len(_body(world).get("submissions", [])) <= 100


@then("the document link expires in 5 minutes")
def doc_link_expiry(world: World) -> None:
    ahead = kyc.seconds_ahead(_body(world)["expiresAt"])
    assert abs(ahead - 5 * 60) <= 30, f"expires_at is {ahead:.0f}s ahead, want 300 +-30"


@then("a GET of the document link returns the uploaded bytes")
def doc_link_bytes(world: World) -> None:
    p = _person(world, "A")
    got = kyc.http_get(_x(world)["doc_url"])
    assert got.status_code == 200, f"GET answered {got.status_code}: {got.text[:200]}"
    assert got.content == p["sub_bodies"][-1][0], "the document link must serve the uploaded bytes"


@then(
    parsers.parse(
        'exactly one kyc.document_view audit event exists for the submission of seller "{name}" by the admin'
    )
)
def one_view_event(world: World, name: str) -> None:
    event = aud.assert_one_event(
        world,
        target_type=TARGET,
        target_id=_last_sub(_person(world, name)),
        action="kyc.document_view",
        actor_id=kyc.jwt_sub(_x(world)["kyc_admin_token"]),
    )
    assert "kyc/" not in str(
        event.get("metadata")
    ), f"audit metadata must not hold the key: {event}"


@then(
    parsers.parse('no kyc.document_view audit event exists for the submission of seller "{name}"')
)
def no_view_event(world: World, name: str) -> None:
    aud.assert_no_event(
        world,
        target_type=TARGET,
        target_id=_last_sub(_person(world, name)),
        action="kyc.document_view",
    )


@then("the response contains no url")
def no_url(world: World) -> None:
    assert not _body(world).get("url"), _body(world)


@then("a GET of the document link answers 403")
def old_link_403(world: World) -> None:
    got = kyc.http_get(_x(world)["doc_url"])
    assert got.status_code == 403, f"expired link answered {got.status_code}"


@then("the kyc-documents bucket has default server-side encryption")
def bucket_encrypted(world: World) -> None:
    conf = kyc.s3().get_bucket_encryption(Bucket=kyc.BUCKET)
    rules = conf["ServerSideEncryptionConfiguration"]["Rules"]
    assert rules and rules[0]["ApplyServerSideEncryptionByDefault"]["SSEAlgorithm"], rules


@then(parsers.parse('{n:d} badge entries are returned with verified true only for seller "{name}"'))
def badges_only_verified(world: World, n: int, name: str) -> None:
    badges = _body(world).get("badges", [])
    assert len(badges) == n, badges
    verified = {b["userId"] for b in badges if b.get("verified")}
    assert verified == {_person(world, name)["id"]}, badges


@then(parsers.parse("{n:d} badge entries are returned"))
def badges_count(world: World, n: int) -> None:
    assert len(_body(world).get("badges", [])) == n


@then(parsers.parse('no object of the submission of seller "{name}" exists'))
def no_objects_of_sub(world: World, name: str) -> None:
    for key in _person(world, name)["sub_refs"][-1]:
        assert not kyc.object_exists(key), f"{key} should have been swept"


@then(parsers.parse('no object of the first submission of seller "{name}" exists'))
def no_objects_of_first(world: World, name: str) -> None:
    for key in _person(world, name)["sub_refs"][0]:
        assert not kyc.object_exists(key), f"{key} should have been swept"


@then(parsers.parse('all objects of the submission of seller "{name}" exist'))
def objects_of_sub_exist(world: World, name: str) -> None:
    for key in _person(world, name)["sub_refs"][-1]:
        assert kyc.object_exists(key), f"{key} must still exist"


@then(parsers.parse('all objects of the second submission of seller "{name}" exist'))
def objects_of_second_exist(world: World, name: str) -> None:
    for key in _person(world, name)["sub_refs"][1]:
        assert kyc.object_exists(key), f"{key} must still exist"


@then(
    parsers.parse(
        'the admin requesting document {idx:d} of the submission of seller "{name}" fails with code "{code}"'
    )
)
def admin_doc_fails(world: World, idx: int, name: str, code: str) -> None:
    resp = _admin_svc(world).get_kyc_document_url_response(_last_sub(_person(world, name)), idx)
    assert (
        resp.status_code >= 400 and _json(resp).get("code") == code
    ), f"{resp.status_code} {resp.text}"


@then('within the erasure window no object exists under the key prefix of seller "A"')
def erased_objects(world: World) -> None:
    prefix = f"kyc/{_person(world, 'A')['id']}/"
    kyc.poll(
        lambda: not kyc.keys_under(prefix),
        kyc.erasure_wait_s(),
        f"objects under {prefix} to be erased",
    )


@then('the admin queue holds no submission of seller "A"')
def erased_queue(world: World) -> None:
    uid = _person(world, "A")["id"]
    svc = _admin_svc(world)
    for status in ("VERIFICATION_STATUS_VERIFIED", "VERIFICATION_STATUS_REJECTED", PENDING):
        cursor: str | None = None
        for _ in range(5000):
            resp = svc.list_kyc_submissions_response(status, 100, cursor)
            assert resp.status_code == 200, resp.text
            data = resp.json()
            assert all(
                s.get("userId") != uid for s in data.get("submissions", [])
            ), f"{status} queue still lists {uid}"
            cursor = (data.get("page") or {}).get("nextCursor") or None
            if not cursor:
                break


@then(parsers.parse('the admin reads the verification status of seller "A" as "{status}"'))
def erased_status(world: World, status: str) -> None:
    resp = _admin_svc(world).get_verification_status_response(_person(world, "A")["id"])
    assert resp.status_code == 200, resp.text
    assert resp.json().get("status") == status, resp.text


@then('the team-verification DLQ holds no message for seller "A"')
def dlq_empty(world: World) -> None:
    assert not kyc.dlq_mentions(
        _person(world, "A")["id"]
    ), "the DLQ must hold no message for the erased user"


@then(
    parsers.parse(
        'seller "B" still has {n:d} submissions and all objects of the submission of seller "B" exist'
    )
)
def other_untouched(world: World, n: int) -> None:
    p = _person(world, "B")
    assert kyc.count("kyc_submissions", p["id"]) == n
    for key in p["sub_refs"][-1]:
        assert kyc.object_exists(key), f"{key} must be untouched"
