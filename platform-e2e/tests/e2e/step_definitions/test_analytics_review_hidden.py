"""Binds analytics/review_hidden.feature (analytics-review-hidden).

Black box: ReviewCreated / ReviewHidden envelopes are produced to engagement.events with the
public proto field numbers (hand-encoded, no stubs) and read back from the DuckDB warehouse.
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, scenarios, then

from tests.e2e.support import efe_support as e
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import tii_support as tii
from tests.e2e.support.ga4_relay_support import f_bytes, f_varint, produce

scenarios("analytics/review_hidden.feature")

CREATED = e.TYPE_PREFIX + "ReviewCreated"
HIDDEN = e.TYPE_PREFIX + "ReviewHidden"
RATING = 4
HIDDEN_AT = 1792317600  # 2026-10-18T10:00:00Z


def _envelope(event_id: str, type_name: str, payload: bytes) -> bytes:
    # EventEnvelope: event_id=1, type=2, principal=4 (SYSTEM), payload=7
    return f_bytes(1, event_id) + f_bytes(2, type_name) + f_bytes(4, f_varint(2, 1)) + f_bytes(7, payload)


@pytest.fixture
def rh():
    return {}


@given("a ReviewCreated envelope for a new review of a new listing is produced to engagement.events")
def produce_created(rh):
    rh["review_id"] = f"rv-{uuid.uuid4().hex[:12]}"
    rh["listing_id"] = f"ls-{uuid.uuid4().hex[:12]}"
    rh["created_event"] = str(uuid.uuid4())
    payload = (
        f_bytes(1, rh["review_id"])
        + f_bytes(2, f"u-{uuid.uuid4().hex[:8]}")
        + f_bytes(3, rh["listing_id"])
        + f_varint(5, RATING)
    )
    produce(e.TOPIC, rh["listing_id"], _envelope(rh["created_event"], CREATED, payload))


@given("a ReviewHidden envelope for the same review is produced to engagement.events")
def produce_hidden(rh):
    rh["hidden_event"] = str(uuid.uuid4())
    payload = (
        f_bytes(1, rh["review_id"])
        + f_bytes(2, rh["listing_id"])
        + f_bytes(3, f_varint(1, HIDDEN_AT))  # google.protobuf.Timestamp.seconds
    )
    produce(e.TOPIC, rh["listing_id"], _envelope(rh["hidden_event"], HIDDEN, payload))


def _rows(rh, sql_cols: str, fact: str, until=bool):
    return tii.warehouse_rows(
        f"SELECT {sql_cols} FROM engagement_facts WHERE listing_id = ? AND fact = ?",
        [rh["listing_id"], fact],
        until=until,
        timeout_s=30,
    )


@then("within 30 seconds engagement_facts holds one review_created row and one review_hidden row for that review")
def both_rows(rh):
    created = _rows(rh, "event_id", "review_created", until=lambda r: len(r) >= 1)
    hidden = _rows(rh, "event_id", "review_hidden", until=lambda r: len(r) >= 1)
    assert created == [(rh["created_event"],)], created
    assert hidden == [(rh["hidden_event"],)], hidden


@then("the review_hidden row has that listing id and a NULL rating")
def hidden_shape(rh):
    rows = tii.warehouse_rows(
        "SELECT review_id, listing_id, rating, user_id, seller_id, epoch(occurred_at) "
        "FROM engagement_facts WHERE event_id = ?",
        [rh["hidden_event"]],
        until=bool,
        timeout_s=30,
    )
    assert len(rows) == 1, rows
    review_id, listing_id, rating, user_id, seller_id, at = rows[0]
    assert (review_id, listing_id) == (rh["review_id"], rh["listing_id"]), rows
    assert rating is None, rows
    assert not user_id and not seller_id, rows
    assert int(at) == HIDDEN_AT, rows


@then("the review_created row keeps its rating and review id")
def created_kept(rh):
    rows = tii.warehouse_rows(
        "SELECT review_id, rating FROM engagement_facts WHERE event_id = ?",
        [rh["created_event"]],
        until=bool,
        timeout_s=30,
    )
    assert rows == [(rh["review_id"], RATING)], rows


@then("the dead-letter topic holds no record for either event id")
def no_dlq(rh):
    # positive control: the rows above prove the pipeline consumed these very records.
    time.sleep(2)
    for ev in (rh["created_event"], rh["hidden_event"]):
        got = stack.find_records(e.DLQ_TOPIC, ev.encode(), timeout_s=8)
        assert not got, f"event {ev} was dead-lettered"
