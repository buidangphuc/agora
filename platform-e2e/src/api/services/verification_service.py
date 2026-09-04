"""Verification (KYC) service: submit a document and read verification status.

Gateway Connect/JSON RPCs for platform.verification.v1.VerificationService.
"""

from __future__ import annotations

from .base_service import BaseService

_SVC = "/platform.verification.v1.VerificationService"


class VerificationService(BaseService):
    def submit_kyc(self, doc_type: str, doc_ref: str) -> dict:
        return self.post(f"{_SVC}/SubmitKyc", {"docType": doc_type, "docRef": doc_ref})

    def get_verification_status(self, user_id: str = "") -> dict:
        payload = {"userId": user_id} if user_id else {}
        return self.post(f"{_SVC}/GetVerificationStatus", payload)
