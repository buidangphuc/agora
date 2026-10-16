"""KYC review audit trail (change verification-kyc-audit, initiative q3-audit-log).

Drives ReviewKyc through the gateway and reads the trail back with QueryAuditLog as the
seeded admin via support/audit_support (import only). Reuses the Given/When/Then steps of
verification_access_steps for the submission, the admin login and the status assertions.
"""

from __future__ import annotations

import base64
import json

import httpx
from pytest_bdd import parsers, then, when

from src.api.services.verification_service import VerificationService
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

KYC_TARGET = "kyc_submission"


def _x(world: World) -> dict:
    return world.state.extra


def _sub(token: str) -> str:
    """The user id (JWT `sub`) of a gateway token."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


@when("the admin rejects the seller's KYC submission")
def admin_rejects_seller(world: World) -> None:
    admin_token = world.state.current_user.token
    _x(world)["kyc_admin_token"] = admin_token
    _x(world)["kyc_resp"] = VerificationService(token=admin_token).review_kyc_response(
        _x(world)["kyc_id"], "reject"
    )


@when("team-audit is stopped")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@when("team-audit is started again")
def audit_started(world: World) -> None:
    aud.start_audit(world)


@then(parsers.parse("the verification gateway call returns {status:d} with code {code}"))
def returns_status_code(world: World, status: int, code: str) -> None:
    resp: httpx.Response = _x(world)["kyc_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    assert resp.json().get("code") == code, resp.text


@then("the seller's verification status is still not verified")
def seller_still_not_verified(world: World) -> None:
    res = VerificationService(token=_x(world)["kyc_owner_token"]).get_verification_status()
    assert not res.get("badge"), f"no badge expected while the audit is down: {res}"
    assert res.get("status") == "VERIFICATION_STATUS_PENDING", res


@then("the seller's verification status is rejected and not verified")
def seller_rejected(world: World) -> None:
    res = VerificationService(token=_x(world)["kyc_owner_token"]).get_verification_status()
    assert not res.get("badge"), f"a rejected KYC must not verify the seller: {res}"
    assert res.get("status") == "VERIFICATION_STATUS_REJECTED", res


@then(parsers.parse('exactly one kyc.review audit event exists for the submission with decision "{decision}"'))
def one_event(world: World, decision: str) -> None:
    x = _x(world)
    admin_id = _sub(x.get("kyc_admin_token") or world.state.current_user.token)
    aud.assert_one_event(
        world,
        target_type=KYC_TARGET,
        target_id=x["kyc_id"],
        action="kyc.review",
        actor_id=admin_id,
        metadata={"decision": decision, "user_id": _sub(x["kyc_owner_token"])},
    )


@then("no audit event exists for the submission")
def no_event(world: World) -> None:
    aud.assert_no_event(world, target_type=KYC_TARGET, target_id=_x(world)["kyc_id"])
