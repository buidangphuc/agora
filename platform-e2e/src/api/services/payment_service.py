"""Payment, Refund & Seller Wallet service client."""

from __future__ import annotations

import uuid
from typing import Any

import httpx

from src.api.services.base_service import BaseService

_PAYMENT = "/platform.payment.v1.PaymentService/"


class PaymentService(BaseService):
    def mock_pay(
        self, order_id: str, amount: int = 5000000, success: bool = True
    ) -> dict[str, Any]:
        create_res = self.post(
            "/platform.payment.v1.PaymentService/CreatePayment",
            {"orderId": order_id, "method": "PAYMENT_METHOD_MOCK_BANK"},
        )
        tx_id = (create_res.get("transaction") or {}).get("id", "")
        if tx_id:
            return self.post(
                "/platform.payment.v1.PaymentService/ProcessMockPayment",
                {"transactionId": tx_id, "simulateSuccess": success},
            )
        return create_res

    def create_payment(
        self, order_id: str, method: str = "PAYMENT_METHOD_MOCK_MOMO"
    ) -> dict[str, Any]:
        """Open a pending payment transaction for an order (online method)."""
        return self.post(
            "/platform.payment.v1.PaymentService/CreatePayment",
            {"orderId": order_id, "method": method},
        )

    def refund(
        self,
        payment_id: str,
        amount: int = 5000000,
        reason: str = "",
        refund_id: str | None = None,
    ) -> dict[str, Any]:
        # refund_id is required (cumulative refunds): one id = one refund; a retry reuses it
        return self.post(
            "/platform.payment.v1.PaymentService/RefundPayment",
            {
                "paymentId": payment_id,
                "amount": amount,
                "reason": reason,
                "refundId": refund_id or f"e2e-{uuid.uuid4().hex}",
            },
        )

    def refund_payment(
        self,
        payment_id: str,
        amount: int = 5000000,
        reason: str = "",
        refund_id: str | None = None,
    ) -> dict[str, Any]:
        return self.refund(payment_id, amount, reason, refund_id)

    def get_seller_wallet(self, seller_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.payment.v1.PaymentService/GetSellerWallet",
            {"sellerId": seller_id},
        )

    def request_payout(
        self,
        seller_id: str,
        amount: int,
        bank_code: str,
        account_number: str,
        account_name: str,
    ) -> dict[str, Any]:
        return self.post(
            "/platform.payment.v1.PaymentService/RequestPayout",
            {
                "sellerId": seller_id,
                "amount": amount,
                "bankCode": bank_code,
                "accountNumber": account_number,
                "accountName": account_name,
            },
        )

    def list_payout_history(self, seller_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.payment.v1.PaymentService/ListPayoutHistory",
            {"sellerId": seller_id},
        )

    def wallet_balance(self) -> int:
        """The caller's own ledger balance (Connect JSON omits a zero balance)."""
        res = self.post(_PAYMENT + "GetWalletBalance", {})
        return int(res.get("balance") or 0)

    # ── Raw responses for the wallet access scenarios (no raise on 4xx) ──
    def wallet_response(self, seller_id: str) -> httpx.Response:
        return self.send("POST", _PAYMENT + "GetSellerWallet", json_body={"sellerId": seller_id})

    def ledger_response(self, seller_id: str) -> httpx.Response:
        return self.send("POST", _PAYMENT + "ListLedgerEntries", json_body={"sellerId": seller_id})

    def payout_response(self, seller_id: str, amount: int = 1000) -> httpx.Response:
        return self.send(
            "POST",
            _PAYMENT + "RequestPayout",
            json_body={
                "sellerId": seller_id,
                "amount": amount,
                "bankCode": "VCB",
                "accountNumber": "0000000000",
                "accountName": "E2E ATTACKER",
            },
        )
