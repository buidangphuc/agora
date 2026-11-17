"""Verification (KYC) service: documents, submission, status, review, queue and public badge.

Gateway Connect/JSON RPCs for platform.verification.v1.VerificationService.
"""

from __future__ import annotations

import httpx

from .base_service import BaseService

_SVC = "/platform.verification.v1.VerificationService"


class VerificationService(BaseService):
    def submit_kyc(self, doc_type: str, doc_ref: str) -> dict:
        return self.post(f"{_SVC}/SubmitKyc", {"docType": doc_type, "docRef": doc_ref})

    def submit_kyc_response(
        self, doc_type: str, doc_ref: str, additional_doc_refs: list[str] | None = None
    ) -> httpx.Response:
        """SubmitKyc without raising on 4xx: the document scenarios assert the error code."""
        body: dict = {"docType": doc_type, "docRef": doc_ref}
        if additional_doc_refs:
            body["additionalDocRefs"] = additional_doc_refs
        return self.send("POST", f"{_SVC}/SubmitKyc", json_body=body)

    def get_verification_status(self, user_id: str = "") -> dict:
        payload = {"userId": user_id} if user_id else {}
        return self.post(f"{_SVC}/GetVerificationStatus", payload)

    def get_verification_status_response(self, user_id: str = "") -> httpx.Response:
        payload = {"userId": user_id} if user_id else {}
        return self.send("POST", f"{_SVC}/GetVerificationStatus", json_body=payload)

    def review_kyc_response(
        self, submission_id: str, decision: str, reason_code: str | None = None
    ) -> httpx.Response:
        """ReviewKyc without raising on 4xx: the access scenarios assert the status code."""
        body: dict = {"id": submission_id, "decision": decision}
        if reason_code:
            body["reasonCode"] = reason_code
        return self.send("POST", f"{_SVC}/ReviewKyc", json_body=body)

    def create_kyc_upload_url_response(self, content_type: str, size_bytes: int) -> httpx.Response:
        return self.send(
            "POST",
            f"{_SVC}/CreateKycUploadUrl",
            json_body={"contentType": content_type, "sizeBytes": size_bytes},
        )

    def list_kyc_submissions_response(
        self, status: str | None = None, page_size: int | None = None, cursor: str | None = None
    ) -> httpx.Response:
        body: dict = {}
        if status:
            body["status"] = status
        page: dict = {}
        if page_size is not None:
            page["pageSize"] = page_size
        if cursor is not None:
            page["cursor"] = cursor
        if page:
            body["page"] = page
        return self.send("POST", f"{_SVC}/ListKycSubmissions", json_body=body)

    def get_kyc_document_url_response(self, submission_id: str, index: int = 0) -> httpx.Response:
        body: dict = {"submissionId": submission_id}
        if index:
            body["index"] = index
        return self.send("POST", f"{_SVC}/GetKycDocumentUrl", json_body=body)

    def get_public_badges_response(self, user_ids: list[str]) -> httpx.Response:
        return self.send("POST", f"{_SVC}/GetPublicBadges", json_body={"userIds": user_ids})
