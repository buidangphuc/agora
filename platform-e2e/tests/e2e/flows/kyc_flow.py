"""Seller KYC flows through the real RPC sequence (payment-payout-kyc-gate).

A seller becomes VERIFIED only through CreateKycUploadUrl, a presigned PUT, SubmitKyc and an
admin ReviewKyc approve: no fake service, no bypass flag.
"""

from __future__ import annotations

import httpx

from config.settings import get_settings
from src.api.services.verification_service import VerificationService
from tests.e2e.support import kyc_documents_support as kyc

VERIFIED = "VERIFICATION_STATUS_VERIFIED"
PENDING = "VERIFICATION_STATUS_PENDING"
REJECTED = "VERIFICATION_STATUS_REJECTED"
NOT_SUBMITTED = "VERIFICATION_STATUS_NOT_SUBMITTED"

_AUTH = "/platform.identity.v1.AuthService/Login"


def admin_token() -> str:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = httpx.post(
        f"{get_settings().gateway_url}{_AUTH}",
        json={"username": admin.username, "password": admin.password},
        timeout=45,
    )
    assert resp.status_code == 200, f"admin login failed: {resp.status_code} {resp.text[:300]}"
    token = (resp.json().get("result") or {}).get("token", "")
    assert token, "admin login returned no token"
    return token


def status_of(seller_token: str) -> str:
    svc = VerificationService(token=seller_token)
    try:
        return svc.get_verification_status().get("status", "")
    finally:
        svc.close()


def submit(seller_token: str) -> str:
    """Upload a document and SubmitKyc; returns the submission id (status PENDING)."""
    svc = VerificationService(token=seller_token)
    try:
        sub_id, _, _ = kyc.submit_with_uploads(svc, 1)
        return sub_id
    finally:
        svc.close()


def review(submission_id: str, decision: str, reason: str | None = None) -> None:
    svc = VerificationService(token=admin_token())
    try:
        resp = svc.review_kyc_response(submission_id, decision, reason)
        assert (
            resp.status_code == 200
        ), f"ReviewKyc {decision}: {resp.status_code} {resp.text[:300]}"
    finally:
        svc.close()


def verify_seller(seller_token: str) -> None:
    """Make the seller VERIFIED through the real KYC RPCs; a no-op when already VERIFIED."""
    if status_of(seller_token) == VERIFIED:
        return
    review(submit(seller_token), "approve")
    assert status_of(seller_token) == VERIFIED


def reject_seller(seller_token: str, reason: str = "document_unreadable") -> None:
    review(submit(seller_token), "reject", reason)
    assert status_of(seller_token) == REJECTED


def status_of_user_as_admin(user_id: str) -> str:
    svc = VerificationService(token=admin_token())
    try:
        return svc.get_verification_status(user_id).get("status", "")
    finally:
        svc.close()
