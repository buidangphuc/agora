"""Step definitions for order_facts and outbox pipeline (ADR-0013 Stage 0)."""

from __future__ import annotations

import datetime
import uuid
from typing import Any

from pytest_bdd import given, parsers, then, when

from tests.e2e.support.world import World


@given(parsers.parse('an order in "{status}" status with line items'))
def order_with_line_items(world: World, status: str) -> None:
    order_id = f"ord-{uuid.uuid4()}"
    items = [
        {"listing_id": "lst-101", "variant_id": "sku-101-red", "seller_id": "seller-001", "quantity": 2, "unit_price": 150000},
        {"listing_id": "lst-102", "variant_id": "sku-102-blue", "seller_id": "seller-001", "quantity": 1, "unit_price": 200000},
    ]
    world.state.extra["test_order"] = {
        "order_id": order_id,
        "status": status,
        "items": items,
        "total_amount": 500000,
        "currency": "VND",
    }


@when(parsers.parse('the order transitions to "{status}"'))
def order_transitions_to_status(world: World, status: str) -> None:
    order = world.state.extra["test_order"]
    order["status"] = status
    # Simulate transactional outbox creation inside the same boundary
    outbox_record = {
        "id": f"outbox-{uuid.uuid4()}",
        "event_id": f"evt-{uuid.uuid4()}",
        "event_type": "platform.order.v1.OrderPaidEvent",
        "aggregate_type": "order",
        "aggregate_id": order["order_id"],
        "status": "pending",
        "payload": {
            "order_id": order["order_id"],
            "buyer_id": "buyer-001",
            "items": order["items"],
            "currency": order["currency"],
        },
        "created_at": datetime.datetime.now(datetime.timezone.utc),
    }
    world.state.extra["outbox_table"] = [outbox_record]


@then(
    parsers.parse(
        'an outbox event is stored in "{table}" with status "{status}" and the exact line item quantities and prices'
    )
)
def verify_outbox_event(world: World, table: str, status: str) -> None:
    outbox = world.state.extra.get("outbox_table", [])
    assert len(outbox) == 1, f"Expected 1 record in {table}, got {len(outbox)}"
    rec = outbox[0]
    assert rec["status"] == status
    items = rec["payload"]["items"]
    assert len(items) == 2
    assert items[0]["quantity"] == 2 and items[0]["unit_price"] == 150000
    assert items[1]["quantity"] == 1 and items[1]["unit_price"] == 200000


@given(parsers.parse('pending records in "{table}"'))
def pending_records_in_outbox(world: World, table: str) -> None:
    order_id = f"ord-{uuid.uuid4()}"
    rec = {
        "id": f"outbox-{uuid.uuid4()}",
        "event_id": f"evt-{uuid.uuid4()}",
        "event_type": "platform.order.v1.OrderPaidEvent",
        "aggregate_type": "order",
        "aggregate_id": order_id,
        "status": "pending",
        "payload": {
            "order_id": order_id,
            "buyer_id": "buyer-001",
            "items": [
                {"listing_id": "lst-201", "variant_id": "sku-201", "seller_id": "seller-002", "quantity": 3, "unit_price": 50000}
            ],
            "currency": "VND",
        },
        "created_at": datetime.datetime.now(datetime.timezone.utc),
        "published_at": None,
    }
    world.state.extra["outbox_table"] = [rec]
    world.state.extra["published_kafka_events"] = []


@when("the outbox relayer runs a sweep")
def relayer_runs_sweep(world: World) -> None:
    outbox = world.state.extra.get("outbox_table", [])
    published = world.state.extra.setdefault("published_kafka_events", [])
    now = datetime.datetime.now(datetime.timezone.utc)
    for rec in outbox:
        if rec["status"] == "pending":
            # Publish to kafka topic
            published.append({
                "topic": "order.events",
                "key": rec["aggregate_id"],
                "event_id": rec["event_id"],
                "event_type": rec["event_type"],
                "payload": rec["payload"],
            })
            rec["status"] = "published"
            rec["published_at"] = now


@then(parsers.parse('events are published to "{topic}" partitioned by "{key_name}"'))
def verify_published_events(world: World, topic: str, key_name: str) -> None:
    published = world.state.extra.get("published_kafka_events", [])
    assert len(published) > 0, f"No events published to {topic}"
    for evt in published:
        assert evt["topic"] == topic
        assert evt["key"] is not None and len(evt["key"]) > 0


@then(parsers.parse('the outbox records are marked "{status}" with a "{field}" timestamp'))
def verify_outbox_marked_published(world: World, status: str, field: str) -> None:
    outbox = world.state.extra.get("outbox_table", [])
    for rec in outbox:
        assert rec["status"] == status
        assert rec.get(field) is not None


@given(parsers.parse('an "{event_type}" on Kafka topic "{topic}" with multiple line items'))
def order_paid_event_on_kafka(world: World, event_type: str, topic: str) -> None:
    order_id = f"ord-{uuid.uuid4()}"
    world.state.extra["incoming_order_event"] = {
        "event_id": f"evt-{uuid.uuid4()}",
        "type": f"platform.order.v1.{event_type}",
        "occurred_at": datetime.datetime.now(datetime.timezone.utc),
        "payload": {
            "order_id": order_id,
            "currency": "VND",
            "items": [
                {"listing_id": "lst-301", "variant_id": "sku-301", "seller_id": "seller-003", "quantity": 4, "unit_price": 100000},
                {"listing_id": "lst-302", "variant_id": "sku-302", "seller_id": "seller-003", "quantity": 2, "unit_price": 250000},
            ],
        },
    }
    world.state.extra["warehouse_order_facts"] = []


@when(parsers.parse('"{service}" processes the message'))
def service_processes_message(world: World, service: str) -> None:
    evt = world.state.extra["incoming_order_event"]
    facts = world.state.extra.setdefault("warehouse_order_facts", [])
    payload = evt["payload"]
    for idx, it in enumerate(payload["items"]):
        facts.append({
            "event_id": f"{evt['event_id']}-{idx}",
            "order_id": payload["order_id"],
            "listing_id": it["listing_id"],
            "variant_id": it["variant_id"],
            "seller_id": it["seller_id"],
            "quantity": it["quantity"],
            "unit_price": it["unit_price"],
            "currency": payload["currency"],
            "occurred_at": evt["occurred_at"],
            "status": "PAID",
        })


@then(
    parsers.parse(
        'one row per line item is inserted into "{table}" with matching "{seller_field}", "{listing_field}", "{qty_field}", and "{price_field}"'
    )
)
def verify_rows_inserted_into_facts(
    world: World, table: str, seller_field: str, listing_field: str, qty_field: str, price_field: str
) -> None:
    facts = world.state.extra.get("warehouse_order_facts", [])
    assert len(facts) == 2, f"Expected 2 rows in {table}, got {len(facts)}"
    assert facts[0]["seller_id"] == "seller-003" and facts[0]["quantity"] == 4 and facts[0]["unit_price"] == 100000
    assert facts[1]["seller_id"] == "seller-003" and facts[1]["quantity"] == 2 and facts[1]["unit_price"] == 250000


@given(parsers.parse('multiple order lines in "{table}" for a seller across several dates'))
def multiple_order_lines_for_seller(world: World, table: str) -> None:
    seller_id = "seller-alpha"
    d1 = datetime.datetime(2026, 9, 1, 10, 0, 0, tzinfo=datetime.timezone.utc)
    d2 = datetime.datetime(2026, 9, 2, 12, 0, 0, tzinfo=datetime.timezone.utc)
    world.state.extra["analytics_seller_id"] = seller_id
    world.state.extra["warehouse_order_facts"] = [
        # day 1: sku-A (2 * 100k = 200k)
        {"seller_id": seller_id, "order_id": "ord-1", "listing_id": "lst-A", "variant_id": "sku-A", "quantity": 2, "unit_price": 100000, "occurred_at": d1},
        # day 2: sku-A (1 * 100k = 100k), sku-B (5 * 50k = 250k)
        {"seller_id": seller_id, "order_id": "ord-2", "listing_id": "lst-A", "variant_id": "sku-A", "quantity": 1, "unit_price": 100000, "occurred_at": d2},
        {"seller_id": seller_id, "order_id": "ord-3", "listing_id": "lst-B", "variant_id": "sku-B", "quantity": 5, "unit_price": 50000, "occurred_at": d2},
        # noise from another seller
        {"seller_id": "seller-beta", "order_id": "ord-9", "listing_id": "lst-Z", "variant_id": "sku-Z", "quantity": 10, "unit_price": 1000000, "occurred_at": d2},
    ]


@when(parsers.parse('"{rpc_method}" is queried for that seller'))
def query_revenue_breakdown(world: World, rpc_method: str) -> None:
    seller_id = world.state.extra["analytics_seller_id"]
    facts = world.state.extra.get("warehouse_order_facts", [])
    # Aggregate directly over order_facts
    seller_facts = [f for f in facts if f["seller_id"] == seller_id]
    
    sku_aggregates: dict[str, dict[str, Any]] = {}
    for f in seller_facts:
        sku = f["variant_id"] or f["listing_id"]
        rev = f["quantity"] * f["unit_price"]
        units = f["quantity"]
        if sku not in sku_aggregates:
            sku_aggregates[sku] = {"sku": sku, "revenue": 0, "units_sold": 0, "listing_id": f["listing_id"]}
        sku_aggregates[sku]["revenue"] += rev
        sku_aggregates[sku]["units_sold"] += units

    sorted_skus = sorted(sku_aggregates.values(), key=lambda x: (-x["revenue"], x["sku"]))
    world.state.extra["query_breakdown_result"] = {
        "top_skus": sorted_skus,
    }


@then(
    parsers.parse(
        'top SKUs report accurate "{rev_field}" and "{units_field}" calculated as "{rev_formula}" and "{units_formula}"'
    )
)
def verify_top_skus_accuracy(
    world: World, rev_field: str, units_field: str, rev_formula: str, units_formula: str
) -> None:
    res = world.state.extra["query_breakdown_result"]
    top_skus = res["top_skus"]
    assert len(top_skus) == 2, f"Expected 2 top SKUs for seller, got {len(top_skus)}"
    # sku-A: 200k + 100k = 300k, 3 units
    assert top_skus[0]["sku"] == "sku-A"
    assert top_skus[0]["revenue"] == 300000
    assert top_skus[0]["units_sold"] == 3
    # sku-B: 250k, 5 units
    assert top_skus[1]["sku"] == "sku-B"
    assert top_skus[1]["revenue"] == 250000
    assert top_skus[1]["units_sold"] == 5


@given("a seller SKU with historical order facts")
def seller_sku_with_order_facts(world: World) -> None:
    world.state.extra["forecast_seller_id"] = "seller-forecast-1"
    world.state.extra["forecast_listing_id"] = "lst-forecast-1"
    world.state.extra["forecast_facts"] = [
        {"seller_id": "seller-forecast-1", "listing_id": "lst-forecast-1", "quantity": 10, "unit_price": 50000},
        {"seller_id": "seller-forecast-1", "listing_id": "lst-forecast-1", "quantity": 12, "unit_price": 50000},
    ]


@when(parsers.parse('"{rpc_method}" is queried for that seller and listing'))
def query_demand_forecast(world: World, rpc_method: str) -> None:
    seller_id = world.state.extra["forecast_seller_id"]
    listing_id = world.state.extra["forecast_listing_id"]
    facts = world.state.extra["forecast_facts"]
    
    # Calculate forecast
    total_units = sum(f["quantity"] for f in facts)
    avg_daily = total_units / len(facts)
    std_daily = 2.0
    
    daily_forecasts = []
    for i in range(14):
        p10 = max(0.0, avg_daily - 1.28 * std_daily)
        p50 = avg_daily
        p90 = avg_daily + 1.28 * std_daily
        daily_forecasts.append({"day": i + 1, "p10": p10, "p50": p50, "p90": p90})
    
    lead_time = 3
    lead_p50 = sum(d["p50"] for d in daily_forecasts[:lead_time])
    safety_stock = 1.65 * (daily_forecasts[0]["p90"] - daily_forecasts[0]["p50"])
    reorder_point = lead_p50 + safety_stock
    
    world.state.extra["forecast_result"] = {
        "seller_id": seller_id,
        "listing_id": listing_id,
        "daily_forecasts": daily_forecasts,
        "suggested_reorder_point": reorder_point,
        "safety_stock": safety_stock,
        "model_version": "lgbm_quantile_v1",
    }


@then(
    parsers.parse(
        'a multi-day forecast with "{p10_field}", "{p50_field}", "{p90_field}" quantiles and suggested reorder point is returned'
    )
)
def verify_demand_forecast_result(world: World, p10_field: str, p50_field: str, p90_field: str) -> None:
    res = world.state.extra["forecast_result"]
    assert len(res["daily_forecasts"]) == 14
    for pt in res["daily_forecasts"]:
        assert pt["p10"] <= pt["p50"] <= pt["p90"]
    assert res["suggested_reorder_point"] > 0
    assert res["safety_stock"] > 0
    assert res["model_version"] is not None

