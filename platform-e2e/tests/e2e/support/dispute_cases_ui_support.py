from __future__ import annotations

from tests.e2e.support import dde_support as d
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as r
from tests.e2e.support.oic_order_support import OicWorld, ok

ORDER_AMOUNT = 500_000
RETURN_AMOUNT = 200_000
STAGES = ("rejected", "open", "investigating")


def seed(w: OicWorld, stage: str) -> None:
    assert stage in STAGES, stage
    p.seller_with_listing(w, ORDER_AMOUNT, "L")
    buyer = p.buyer(w, "buyer")
    seller = w.actors["seller"]
    order_id, _ = p.place_and_pay(w, buyer, "L")
    return_id = r.new_return(w, buyer, order_id, RETURN_AMOUNT).get("id", "")
    assert return_id, "CreateReturnRequest returned no return id"
    ok(r.set_return_status(w, seller, return_id, r.REJECTED))
    w.orders["order"] = order_id
    w.data["return_id"] = return_id
    if stage == "rejected":
        return
    resp = d.create_dispute(w, buyer, order_id, d.order_seller_id(w, order_id), return_id)
    w.data["dispute_id"] = ok(resp)["dispute"]["id"]
    if stage == "investigating":
        ok(d.resolve(w, d.admin(w), w.data["dispute_id"], d.INVESTIGATING))


def order_disputes(w: OicWorld) -> list[dict]:
    resp = d.list_order_disputes(w, w.actors["buyer"], w.orders["order"])
    return ok(resp).get("disputes", [])
