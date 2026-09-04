"""Order service: cart checkout, purchase saga, RMA & shipment tracking."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService
from src.constants import gateway_endpoints as ep


class OrderService(BaseService):
    def create_order(self, payload: dict | None = None) -> dict[str, Any]:
        """Create an order from cart items. Default payload uses COD with default address."""
        body = payload if payload is not None else {"paymentMethod": "PAYMENT_METHOD_COD"}
        return self.post(ep.ORDER_CREATE, body)

    def get_order(self, order_id: str) -> dict[str, Any]:
        return self.post(ep.ORDER_GET, {"id": order_id})

    def get_saga_state(self, order_id: str) -> dict[str, Any]:
        return self.post(ep.ORDER_SAGA_STATE, {"orderId": order_id})

    def force_fail_saga(self, order_id: str) -> dict[str, Any]:
        """Trigger the compensating-transaction path (ReleaseStock, refund)."""
        return self.post(ep.ORDER_FORCE_FAIL_SAGA, {"orderId": order_id, "failStep": "payment"})

    def create_return_request(
        self, order_id: str, reason: str, refund_amount: int
    ) -> dict[str, Any]:
        return self.post(
            ep.ORDER_RETURN_CREATE,
            {"orderId": order_id, "reason": reason, "refundAmount": refund_amount},
        )

    def update_return_status(self, return_id: str, status: str) -> dict[str, Any]:
        return self.post(
            ep.ORDER_RETURN_UPDATE,
            {"id": return_id, "status": status},
        )

    def create_shipment(
        self, order_id: str, carrier: str = "SPX Express", tracking_code: str = ""
    ) -> dict[str, Any]:
        return self.post(
            ep.ORDER_SHIPMENT_CREATE,
            {"orderId": order_id, "carrier": carrier, "trackingCode": tracking_code},
        )

    def get_shipment_tracking(self, tracking_code: str) -> dict[str, Any]:
        return self.post(
            ep.ORDER_SHIPMENT_TRACKING,
            {"trackingCode": tracking_code},
        )
