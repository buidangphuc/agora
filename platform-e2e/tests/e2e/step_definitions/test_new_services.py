"""API-level pytest-bdd tests for the four new thin/no-UI backend services.

Exercised directly through the team-gateway Connect/JSON API (no browser). Each
scenario is a happy-path round-trip: write via one RPC, read it back via another.

Self-contained: local fixtures + step defs, no edits to conftest / service_factory /
gateway_endpoints. Auth reuses the existing AuthService (register-or-login).
"""

from __future__ import annotations

import uuid

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from src.api.services.audit_service import AuditService
from src.api.services.auth_service import AuthService
from src.api.services.referral_service import ReferralService
from src.api.services.sharing_service import SharingService
from src.api.services.verification_service import VerificationService

SEED_PASSWORD = "pass123"

scenarios("services/new_services.feature")


# ── Auth helpers / fixtures ───────────────────────────────────────────────
def _token_for(username: str, role: str) -> str:
    """Register-or-login the seeded account and return its JWT."""
    return AuthService().register(username, SEED_PASSWORD, role)


@pytest.fixture(scope="module")
def buyer_token() -> str:
    return _token_for("buyer1", "buyer")


@pytest.fixture(scope="module")
def seller_token() -> str:
    return _token_for("seller1", "seller")


@pytest.fixture
def ctx() -> dict:
    """Per-scenario scratch state passed between steps."""
    return {}


# ── Given: authenticated principals ───────────────────────────────────────
@given("an authenticated buyer")
def authenticated_buyer(ctx: dict, buyer_token: str) -> None:
    ctx["token"] = buyer_token


@given("an authenticated seller")
def authenticated_seller(ctx: dict, seller_token: str) -> None:
    ctx["token"] = seller_token


# ── Referral ──────────────────────────────────────────────────────────────
@when("the buyer creates a referral code")
def create_referral_code(ctx: dict) -> None:
    resp = ReferralService(token=ctx["token"]).create_referral_code()
    code = resp.get("code", "")
    assert code, f"CreateReferralCode returned no code: {resp}"
    ctx["referral_code"] = code


@then("GetMyReferral returns that same code")
def get_my_referral_matches(ctx: dict) -> None:
    resp = ReferralService(token=ctx["token"]).get_my_referral()
    assert resp.get("code") == ctx["referral_code"], (
        f"GetMyReferral code {resp.get('code')!r} != created {ctx['referral_code']!r}"
    )


@then("listing referral rewards succeeds")
def list_referral_rewards_ok(ctx: dict) -> None:
    resp = ReferralService(token=ctx["token"]).list_referral_rewards()
    # A fresh account may have zero rewards; the RPC must still return a list field.
    assert isinstance(resp.get("rewards", []), list), f"unexpected ListReferralRewards: {resp}"


# ── Sharing ───────────────────────────────────────────────────────────────
@when(parsers.parse('the seller creates a share link for target "{target_type}" "{target_id}"'))
def create_share_link(ctx: dict, target_type: str, target_id: str) -> None:
    ctx["share_target_type"] = target_type
    ctx["share_target_id"] = target_id
    resp = SharingService(token=ctx["token"]).create_share_link(
        target_type, target_id, utm={"utm_source": "e2e"}
    )
    short_code = resp.get("shortCode", "")
    assert short_code, f"CreateShareLink returned no shortCode: {resp}"
    ctx["short_code"] = short_code


@then(parsers.parse('resolving the short code returns target "{target_type}" "{target_id}"'))
def resolve_share_link(ctx: dict, target_type: str, target_id: str) -> None:
    resp = SharingService(token=ctx["token"]).resolve_share_link(ctx["short_code"])
    assert resp.get("targetType") == target_type, f"targetType mismatch: {resp}"
    assert resp.get("targetId") == target_id, f"targetId mismatch: {resp}"
    ctx["resolved"] = resp


@then("the resolved link carries OG meta")
def resolved_has_og_meta(ctx: dict) -> None:
    og = ctx["resolved"].get("ogMeta") or {}
    assert og, f"ResolveShareLink returned no ogMeta: {ctx['resolved']}"
    assert og.get("title"), f"ogMeta has no title: {og}"


# ── Audit ─────────────────────────────────────────────────────────────────
@when("an audit event is written for the seller")
def write_audit_event(ctx: dict) -> None:
    token = uuid.uuid4().hex[:8]
    ctx["audit_actor_id"] = f"actor-{token}"
    ctx["audit_target_type"] = f"e2e-audit-{token}"
    ctx["audit_target_id"] = f"tgt-{token}"
    ctx["audit_action"] = "e2e.audit.write"
    AuditService(token=ctx["token"]).write_audit_event(
        actor_id=ctx["audit_actor_id"],
        action=ctx["audit_action"],
        target_type=ctx["audit_target_type"],
        target_id=ctx["audit_target_id"],
    )


@then("querying the audit log returns that event")
def query_audit_log_returns_event(ctx: dict) -> None:
    resp = AuditService(token=ctx["token"]).query_audit_log(target_type=ctx["audit_target_type"])
    events = resp.get("events", [])
    matches = [e for e in events if e.get("targetId") == ctx["audit_target_id"]]
    assert matches, (
        f"written audit event {ctx['audit_target_id']!r} not found in "
        f"QueryAuditLog(targetType={ctx['audit_target_type']!r}): {resp}"
    )
    assert matches[0].get("action") == ctx["audit_action"], f"action mismatch: {matches[0]}"


# ── Verification ──────────────────────────────────────────────────────────
@when("the buyer submits a KYC document")
def submit_kyc(ctx: dict) -> None:
    resp = VerificationService(token=ctx["token"]).submit_kyc(
        doc_type="national_id", doc_ref=f"kyc-ref-{uuid.uuid4().hex[:8]}"
    )
    ctx["kyc_submit"] = resp


@then("the verification status is PENDING")
def verification_status_pending(ctx: dict) -> None:
    # SubmitKyc must have created a record.
    assert ctx["kyc_submit"].get("id"), f"SubmitKyc returned no id: {ctx['kyc_submit']}"
    resp = VerificationService(token=ctx["token"]).get_verification_status()
    status = str(resp.get("status", ""))
    # proto3 JSON omits the zero enum value, so an absent status == PENDING (0).
    # Assert it is PENDING and explicitly not already VERIFIED/REJECTED.
    is_pending = status in ("", "VERIFICATION_STATUS_PENDING", "0") or "PENDING" in status
    assert is_pending, f"expected PENDING verification status, got {resp}"
    assert "VERIFIED" not in status and "REJECTED" not in status, (
        f"verification status is not PENDING: {resp}"
    )
