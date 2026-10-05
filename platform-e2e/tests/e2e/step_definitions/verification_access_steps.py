"""KYC review access control (fix-verification-auth).

Drives SubmitKyc / ReviewKyc / GetVerificationStatus through the gateway Connect
API and asserts the status team-verification produced: 403 for a non-admin
reviewer, 200 for an admin. The admin is the seeded one (SEED_ADMIN_PASSWORD);
the "an admin is logged in" step lives in common_steps.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services.verification_service import VerificationService
from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"


def _register_and_submit(world: World, role: str) -> None:
    username = f"e2e_{role}_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role=role)
    submit = VerificationService(token=token).submit_kyc(
        doc_type="national_id", doc_ref=f"kyc-ref-{uuid.uuid4().hex[:8]}"
    )
    world.state.extra["kyc_owner_token"] = token
    world.state.extra["kyc_id"] = submit["id"]
    world.service_factory.set_token(token)


@given("a signed-in buyer with a pending KYC submission")
def buyer_with_pending_kyc(world: World) -> None:
    _register_and_submit(world, "buyer")


@given("a seller with a pending KYC submission")
def seller_with_pending_kyc(world: World) -> None:
    _register_and_submit(world, "seller")


@when("the buyer approves their own KYC submission")
def buyer_approves_own(world: World) -> None:
    world.state.extra["kyc_resp"] = VerificationService(
        token=world.state.extra["kyc_owner_token"]
    ).review_kyc_response(world.state.extra["kyc_id"], "approve")


@when("the admin approves the seller's KYC submission")
def admin_approves_seller(world: World) -> None:
    admin_token = world.state.current_user.token
    world.state.extra["kyc_resp"] = VerificationService(token=admin_token).review_kyc_response(
        world.state.extra["kyc_id"], "approve"
    )


@then(parsers.parse("the verification gateway call returns {status:d}"))
def verification_returns_status(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["kyc_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


def _owner_badge(world: World) -> bool:
    res = VerificationService(token=world.state.extra["kyc_owner_token"]).get_verification_status()
    return bool(res.get("badge"))


@then("the buyer's verification status is still not verified")
def buyer_not_verified(world: World) -> None:
    assert not _owner_badge(world), "a self-approved KYC must not verify the buyer"


@then("the seller's verification status is verified")
def seller_verified(world: World) -> None:
    assert _owner_badge(world), "admin approval should verify the seller"
