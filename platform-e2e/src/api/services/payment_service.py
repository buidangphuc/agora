"""Payment, Refund & Seller Wallet service client."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService


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

    def refund(self, payment_id: str, amount: int = 5000000, reason: str = "") -> dict[str, Any]:
        return self.post(
            "/platform.payment.v1.PaymentService/RefundPayment",
            {"paymentId": payment_id, "amount": amount, "reason": reason},
        )

    def refund_payment(
        self, payment_id: str, amount: int = 5000000, reason: str = ""
    ) -> dict[str, Any]:
        return self.refund(payment_id, amount, reason)

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
