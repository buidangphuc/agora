"""Steps for port-payment-ledger-integrity (area plp-pay): seller-settlement-credit,
seller-refund-deduction and seller-payout-holdback.

Black box through the gateway with real, distinct users (one seller and its buyers per scenario).
The ledger is read through `ListLedgerEntries` / `GetWalletBalance`; the reference of a row, the
store-constraint scenarios and the Kafka replay/DLQ scenarios go to the real stores (`psql` on
payment_db, confluent-kafka and `rpk` on the broker). "Absence" assertions never sleep blindly:
they follow a sentinel order of the same seller, whose credit proves the single ordered consumer
has applied (or parked) everything produced before it. Scenarios that stop team-payment, replay
Kafka records or wait out the hold window (overlay
`platform-e2e/compose/payment-ledger.override.yaml`) are tagged @destructive in the features.
"""

from __future__ import annotations

import re
import time
import uuid
from datetime import timedelta

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import (
    CANCELLED,
    ORDER,
    PAID,
    SHIPPED,
    Actor,
    OicWorld,
    code_of,
    post,
    race,
    stays_status,
    wait_status,
)

_STATUS = {"Paid": PAID, "Cancelled": CANCELLED, "Shipped": SHIPPED}
_PAYMENT_STATUS = {"PAID": p.PAID, "REFUNDED": p.REFUNDED}
_ORDER_PAID_TYPE = b"platform.order.v1.OrderPaidEvent"
_ORDER_CANCELLED_TYPE = b"platform.order.v1.OrderCancelled"
_ORDER_SHIPPED_TYPE = b"OrderShipped"
_HELD_RE = re.compile(r"^amount is held until (?P<ts>\S+) \(refund window\)$")


def any_step(pattern):
    """Register one function as a Given, When and Then step (the feature may use And)."""

    def deco(fn):
        for kind in (given, when, then):
            kind(pattern, stacklevel=2)(fn)
        return fn

    return deco


@pytest.fixture
def plp():
    w = OicWorld()
    w.data.update(paid=[], credited=[], inserted=[])
    yield w
    # never leave team-payment stopped; remove rows a missing constraint let through
    if w.data.get("payment_stopped"):
        stack.start_payment()
        w.data.pop("payment_stopped", None)
    for row_id in w.data.get("inserted", []):
        stack.psql(f"DELETE FROM wallet_ledger WHERE id = {stack.sql_lit(row_id)}", check=False)


def _actor(w: OicWorld, name: str) -> Actor:
    return w.actors[name]


def _seller(w: OicWorld) -> Actor:
    return w.actors["seller"]


def _last(w: OicWorld):
    return w.responses["last"]


def _record_paid(w: OicWorld, listing: str, price: int, order_id: str, tx_id: str) -> None:
    w.orders["order"] = order_id
    w.data["paid"].append({"listing": listing, "price": price, "order": order_id, "tx": tx_id})


def _paid(w: OicWorld, listing: str | None = None) -> dict:
    items = [e for e in w.data["paid"] if listing is None or e["listing"] == listing]
    assert items, f"no paid order recorded for listing {listing!r}"
    return items[-1]


def _price(w: OicWorld, listing: str) -> int:
    return w.listings[listing]["amount"]


# ── actors and catalogue ─────────────────────────────────────────────────
@any_step(parsers.parse('a seller with a listing "{name}" whose order pays {price:d}'))
def seller_with_listing(plp, name, price):
    p.seller_with_listing(plp, price, name)
    plp.data["balance0"] = p.balance(plp, _seller(plp))


@any_step(
    parsers.parse(
        'a seller with listings "{a}" whose order pays {pa:d} and "{b}" whose order pays {pb:d}'
    )
)
def seller_with_two_listings(plp, a, pa, b, pb):
    p.seller_with_listing(plp, pa, a)
    p.seller_with_listing(plp, pb, b)
    plp.data["balance0"] = p.balance(plp, _seller(plp))


@any_step(parsers.parse('a buyer "{name}"'))
def a_buyer(plp, name):
    p.buyer(plp, name)


# ── paying ───────────────────────────────────────────────────────────────
@any_step(parsers.parse('"{b}" pays a new order of "{listing}"'))
def pays_new_order(plp, b, listing):
    order_id, tx_id = p.place_and_pay(plp, _actor(plp, b), listing)
    _record_paid(plp, listing, _price(plp, listing), order_id, tx_id)


@any_step(parsers.parse('"{b}" pays a new order of "{listing}" and the seller is credited'))
def pays_new_order_credited(plp, b, listing):
    pays_new_order(plp, b, listing)
    price = _price(plp, listing)
    # later steps read the credit (its createdAt) from last_rows
    plp.data["last_rows"] = p.wait_rows(
        plp, _seller(plp), p.SETTLEMENT, price, plp.data["credited"].count(price) + 1
    )
    plp.data["credited"].append(price)


@any_step(parsers.parse('"{b}" has paid an order of "{listing}" and the seller is credited'))
def has_paid_credited(plp, b, listing):
    pays_new_order_credited(plp, b, listing)


@any_step(
    parsers.parse('"{b}" has paid {n:d} orders of "{listing}" and the seller is credited for each')
)
def has_paid_n_credited(plp, b, n, listing):
    plp.data["t0_ms"] = int(time.time() * 1000) - 5000
    for _ in range(n):
        pays_new_order_credited(plp, b, listing)
    plp.data["snapshot"] = (p.ledger(plp, _seller(plp)), p.balance(plp, _seller(plp)))


@any_step(
    parsers.parse(
        '"{b}" has paid an order of "{listing}" whose credit has aged past the hold window'
    )
)
def has_paid_aged(plp, b, listing):
    price = _price(plp, listing)
    order_id = p.credit_and_age(plp, _seller(plp), _actor(plp, b), listing, price)
    _record_paid(plp, listing, price, order_id, plp.data["tx"][order_id])
    plp.data["credited"].append(price)


@any_step(parsers.parse('"{b}" has a Pending order of "{listing}" with an open payment'))
def has_pending_with_payment(plp, b, listing):
    buyer = _actor(plp, b)
    order_id = p.o.place_order(plp, buyer, listing, 1)
    tx_id = p.o.open_payment(plp, buyer, order_id)
    plp.orders["order"] = order_id
    plp.data["open_tx"] = tx_id
    plp.data.setdefault("tx", {})[order_id] = tx_id


@any_step(parsers.parse('"{b}" calls the mock payment again for the same order'))
def mock_payment_again(plp, b):
    tx_id = _paid(plp)["tx"]
    plp.responses["last"] = p.o.settle_payment(plp, _actor(plp, b), tx_id)


@any_step(parsers.parse('"{b}" sends {n:d} concurrent successful mock payments for the order'))
def concurrent_payments(plp, b, n):
    buyer, tx_id = _actor(plp, b), plp.data["open_tx"]
    plp.data["race"] = race([lambda: p.o.settle_payment(plp, buyer, tx_id) for _ in range(n)])
    plp.data["paid"].append({"listing": "L", "price": 0, "order": plp.orders["order"], "tx": tx_id})


@any_step(parsers.parse('the payment of "{b}" succeeds'))
def payment_succeeds(plp, b):
    resp = p.o.settle_payment(plp, _actor(plp, b), plp.data["open_tx"])
    assert resp.status_code == 200, f"late payment failed: {p.describe(resp)}"
    plp.data["paid"].append(
        {"listing": "L", "price": 0, "order": plp.orders["order"], "tx": plp.data["open_tx"]}
    )


@any_step("a sentinel order of the seller is credited")
def sentinel(plp):
    p.settle_sentinel(plp, _seller(plp))
    plp.data["credited"].append(p.SENTINEL_PRICE + plp.data["sentinels"])


# ── orders ───────────────────────────────────────────────────────────────
@any_step(parsers.parse('"{b}" cancels the order'))
def cancels_order(plp, b):
    resp = post(plp, _actor(plp, b), ORDER, "CancelOrder", {"id": plp.orders["order"]})
    plp.responses["last"] = resp
    assert resp.status_code == 200, f"CancelOrder failed: {p.describe(resp)}"


@any_step(parsers.parse('"{b}" cancels the order as soon as it reads Paid'))
def cancels_as_soon_as_paid(plp, b):
    # place_and_pay already returned on the first read of Paid, before any credit is observed
    cancels_order(plp, b)


@any_step("the seller ships the order")
def ships_order(plp):
    # CreateShipment moves the order to Shipped and commits the OrderShipped outbox row with it
    resp = post(
        plp,
        _seller(plp),
        ORDER,
        "CreateShipment",
        {
            "orderId": plp.orders["order"],
            "carrier": "SPX Express",
            "trackingCode": f"PLP{uuid.uuid4().hex[:10]}",
        },
    )
    plp.responses["last"] = resp
    assert resp.status_code == 200, f"CreateShipment failed: {p.describe(resp)}"


@any_step(parsers.parse('the order read by "{b}" is {status}'))
def order_is(plp, b, status):
    buyer, order_id = _actor(plp, b), plp.orders["order"]
    wait_status(plp, buyer, order_id, _STATUS[status])
    if status == "Cancelled":  # a late payment must not move it
        stays_status(plp, buyer, order_id, _STATUS[status], seconds=4.0)


@any_step(
    parsers.re(
        r'(?:within the settle window )?the payment read by "(?P<b>[^"]+)" is (?P<st>PAID|REFUNDED)'
    )
)
def payment_is(plp, b, st):
    p.wait_payment_status(plp, _actor(plp, b), plp.orders["order"], _PAYMENT_STATUS[st])


# ── refunds ──────────────────────────────────────────────────────────────
@any_step(parsers.parse("the seller refunds {amount:d} of the payment"))
def seller_refunds(plp, amount):
    plp.responses["last"] = p.refund(plp, _seller(plp), _paid(plp)["tx"], amount)


@any_step(parsers.parse('the seller refunds {amount:d} of the payment of the order of "{listing}"'))
def seller_refunds_of_listing(plp, amount, listing):
    plp.responses["last"] = p.refund(plp, _seller(plp), _paid(plp, listing)["tx"], amount)


# ── payouts ──────────────────────────────────────────────────────────────
@any_step(parsers.parse("the seller requests a wallet payout of {amount:d}"))
def requests_wallet_payout(plp, amount):
    plp.responses["last"] = p.wallet_payout(plp, _seller(plp), amount)


@any_step(parsers.parse("the seller requests a bank payout of {amount:d}"))
def requests_bank_payout(plp, amount):
    plp.responses["last"] = p.bank_payout(plp, _seller(plp), amount)


@any_step(parsers.parse("the seller sends {n:d} concurrent wallet payouts of {amount:d}"))
def concurrent_payouts(plp, n, amount):
    seller = _seller(plp)
    plp.data["race"] = race([lambda: p.wallet_payout(plp, seller, amount) for _ in range(n)])


# ── generic call assertions ──────────────────────────────────────────────
@then("the call succeeds")
def call_succeeds(plp):
    resp = _last(plp)
    assert resp.status_code == 200, f"expected success, got {p.describe(resp)}"


@then(parsers.parse('the call fails with "{code}"'))
def call_fails_with(plp, code):
    resp = _last(plp)
    assert code_of(resp) == code, f"expected {code}, got {p.describe(resp)}"


@then(parsers.parse('the call fails with "{code}" and the message "{message}"'))
def call_fails_with_message(plp, code, message):
    resp = _last(plp)
    assert code_of(resp) == code, f"expected {code}, got {p.describe(resp)}"
    assert p.message_of(resp) == message, f"expected message {message!r}, got {p.describe(resp)}"


@then(parsers.parse('the call fails with "{code}" because the amount is held'))
def call_fails_held(plp, code):
    resp = _last(plp)
    assert code_of(resp) == code, f"expected {code}, got {p.describe(resp)}"
    msg = p.message_of(resp)
    assert _HELD_RE.match(
        msg
    ), f"expected 'amount is held until <RFC3339> (refund window)': {msg!r}"


@then("the call reports the order already paid")
def reports_already_paid(plp):
    resp = _last(plp)
    assert resp.status_code == 200, f"expected 200, got {p.describe(resp)}"
    body = resp.json()
    assert body.get("success") is True, body
    assert (body.get("transaction") or {}).get("status") == p.PAID, body
    assert "đã được thanh toán trước đó" in body.get("message", ""), body


@then(parsers.parse('exactly {n:d} of the calls succeed and the others fail with "{code}"'))
@then(parsers.parse('exactly {n:d} of the calls succeeds and the others fail with "{code}"'))
def race_outcomes(plp, n, code):
    results = plp.data["race"]
    won = [r for r in results if r.status_code == 200]
    lost = [r for r in results if r.status_code != 200]
    assert len(won) == n, f"{len(won)} of {len(results)} succeeded, want {n}: " + str(
        [p.describe(r) for r in results]
    )
    wrong = [p.describe(r) for r in lost if code_of(r) != code]
    assert not wrong, f"failures that are not {code}: {wrong}"


@then(parsers.parse("the payout is accepted with a PENDING PAYOUT entry of {amount:d}"))
def payout_accepted(plp, amount):
    resp = _last(plp)
    assert resp.status_code == 200, f"payout refused: {p.describe(resp)}"
    entry = resp.json().get("entry") or {}
    assert entry.get("type") == p.PAYOUT, entry
    assert entry.get("status") == "PENDING", entry
    assert int(entry.get("amount", 0)) == amount, entry


# ── ledger assertions ────────────────────────────────────────────────────
@any_step(
    parsers.re(
        r"(?:within the settle window )?the seller has exactly (?P<n>\d+) (?P<type>[A-Z_]+) "
        r"entr(?:y|ies) of (?P<amount>-?\d+)(?: with status (?P<status>[A-Z]+))?"
    )
)
def seller_has_exactly(plp, n, type, amount, status):
    seller, n, amount = _seller(plp), int(n), int(amount)
    if n:
        got = p.wait_rows(plp, seller, type, amount, n)
        plp.data["last_rows"] = got
    p.quiet(plp, lambda: _exact(plp, seller, type, amount, n))
    if status:
        bad = [e for e in p.rows(plp, seller, type, amount) if e.get("status") != status]
        assert not bad, f"entries not {status}: {bad}"


def _exact(w: OicWorld, seller: Actor, type_: str, amount: int, n: int) -> None:
    got = p.rows(w, seller, type_, amount)
    assert (
        len(got) == n
    ), f"seller has {len(got)} {type_} of {amount}, want {n}: {p.ledger(w, seller)}"


@then(parsers.re(r"the seller's ledger has no entry of (?P<amounts>-?\d+(?: or -?\d+)*)"))
def ledger_has_no_entry(plp, amounts):
    wanted = {int(a) for a in amounts.split(" or ")}
    seller = _seller(plp)

    def check():
        hit = [e for e in p.ledger(plp, seller) if int(e["amount"]) in wanted]
        assert not hit, f"unexpected ledger entries: {hit}"

    p.quiet(plp, check)


@then(parsers.parse("the seller's ledger holds only {n:d} ORDER_SETTLEMENT entries"))
def ledger_holds_only(plp, n):
    entries = p.ledger(plp, _seller(plp))
    kinds = [e["type"] for e in entries]
    assert kinds == [p.SETTLEMENT] * n, f"ledger is not {n} settlement credits only: {entries}"


@then("the seller's ledger has no PAYOUT entry")
def ledger_has_no_payout(plp):
    entries = p.rows(plp, _seller(plp), p.PAYOUT)
    assert not entries, f"unexpected PAYOUT entries: {entries}"


@then("the seller's payout history is empty")
def payout_history_empty(plp):
    history = p.payout_history(plp, _seller(plp))
    assert not history, f"unexpected payouts: {history}"


@then(parsers.parse("the seller's balance is {amount:d}"))
def balance_is(plp, amount):
    p.wait_balance(plp, _seller(plp), amount)


@then(parsers.parse("the seller's balance grew by {amount:d}"))
def balance_grew(plp, amount):
    p.wait_balance(plp, _seller(plp), plp.data["balance0"] + amount)


@then("the seller's balance equals the amounts of the credited orders")
def balance_equals_credited(plp):
    p.wait_balance(plp, _seller(plp), sum(plp.data["credited"]))


@then(parsers.parse("both GetWalletBalance and GetSellerWallet return {amount:d}"))
def both_balances(plp, amount):
    seller = _seller(plp)
    assert p.balance(plp, seller) == amount, p.ledger(plp, seller)
    assert p.wallet_balance(plp, seller) == amount, p.ledger(plp, seller)


@then("that entry references the payment transaction of the order")
def entry_references_payment(plp):
    entry = plp.data["last_rows"][-1]
    proc = stack.psql(
        f"SELECT reference_id FROM wallet_ledger WHERE id = {stack.sql_lit(entry['id'])}",
        check=False,
    )
    assert proc.returncode == 0, f"wallet_ledger has no reference_id: {proc.stderr.strip()[:300]}"
    assert proc.stdout.strip() == _paid(plp)["tx"], (
        f"credit references {proc.stdout.strip()!r}, want the payment transaction "
        f"{_paid(plp)['tx']!r}"
    )


@then("the held message names the instant the credit leaves the window and shows no amount")
def held_message(plp):
    resp = _last(plp)
    assert code_of(resp) == "failed_precondition", f"expected a refusal, got {p.describe(resp)}"
    msg = p.message_of(resp)
    m = _HELD_RE.match(msg)
    assert m, f"expected 'amount is held until <RFC3339> (refund window)': {msg!r}"
    credit = p.parse_ts(plp.data["last_rows"][-1]["createdAt"])
    expected = credit + timedelta(seconds=stack.hold_window_s())
    got = p.parse_ts(m.group("ts"))
    assert abs((got - expected).total_seconds()) <= 5, f"release {got} is not ~{expected} (T + W)"
    for amount in ("100000", "500000"):
        assert amount not in msg, f"message reveals an amount: {msg!r}"


# ── store scenarios (psql into payment_db) ───────────────────────────────
def _insert(w: OicWorld, type_: str, amount: int, reference: str | None) -> None:
    seller = _seller(w)
    row_id = str(uuid.uuid4())
    w.data["balance_before_insert"] = p.balance(w, seller)
    cols = ["id", "seller_id", "type", "amount", "status"]
    vals = [row_id, seller.user_id, type_, str(amount), "COMPLETED"]
    if reference is not None:
        cols.append("reference_id")
        vals.append(reference)
    literals = ", ".join(
        v if c == "amount" else stack.sql_lit(v) for c, v in zip(cols, vals, strict=True)
    )
    proc = stack.psql(
        f"INSERT INTO wallet_ledger ({', '.join(cols)}) VALUES ({literals})", check=False
    )
    if proc.returncode == 0:
        w.data["inserted"].append(row_id)
    w.data["insert"] = proc


@any_step(
    "a second ORDER_SETTLEMENT row with the reference of the credited payment is inserted "
    "directly into team-payment's database"
)
def insert_duplicate_credit(plp):
    _insert(plp, p.SETTLEMENT, 500000, _paid(plp)["tx"])


@any_step(
    "an ORDER_SETTLEMENT row of -1 with a fresh reference is inserted directly into "
    "team-payment's database"
)
def insert_negative_credit(plp):
    _insert(plp, p.SETTLEMENT, -1, f"plp-fresh-{uuid.uuid4()}")


@any_step(
    "a REFUND_DEDUCTION row of -1 without a reference is inserted directly into "
    "team-payment's database"
)
def insert_unreferenced_deduction(plp):
    _insert(plp, p.DEDUCTION, -1, None)


_VIOLATION = {
    "unique": "duplicate key value violates unique constraint",
    "check": "violates check constraint",
}


@then(parsers.parse("the insert fails with a {kind} violation"))
def insert_fails(plp, kind):
    proc = plp.data["insert"]
    assert proc.returncode != 0, "the insert was accepted: the ledger store has no such constraint"
    assert _VIOLATION[kind] in proc.stderr, f"not a {kind} violation: {proc.stderr.strip()[:400]}"


@then("the seller's balance through the gateway is unchanged")
def balance_unchanged(plp):
    assert p.balance(plp, _seller(plp)) == plp.data["balance_before_insert"]


# ── Kafka replay / dead letters / restarts (destructive lane) ────────────
def _record_of(w: OicWorld, type_marker: bytes) -> dict:
    order_id = w.orders["order"]
    records = [
        r
        for r in stack.find_records(stack.ORDER_EVENTS_TOPIC, order_id.encode())
        if type_marker in r["value"]
    ]
    assert records, f"no {type_marker.decode()} record of order {order_id} on order.events"
    return records[0]


def _replay(w: OicWorld, type_marker: bytes) -> None:
    rec = _record_of(w, type_marker)
    stack.produce(stack.ORDER_EVENTS_TOPIC, rec["key"], rec["value"], rec["headers"] or None)


@any_step("the OrderPaidEvent record of the order is produced to order.events again, byte for byte")
def replay_paid(plp):
    _replay(plp, _ORDER_PAID_TYPE)


@any_step("the OrderCancelled record of the order is produced to order.events again, byte for byte")
def replay_cancelled(plp):
    _replay(plp, _ORDER_CANCELLED_TYPE)


@any_step("the OrderShipped event of the order is on order.events")
def shipped_event_published(plp):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if any(
            _ORDER_SHIPPED_TYPE in r["value"]
            for r in stack.find_records(stack.ORDER_EVENTS_TOPIC, plp.orders["order"].encode())
        ):
            return
        time.sleep(1)
    raise AssertionError(f"no OrderShipped record of order {plp.orders['order']} on order.events")


@any_step("a malformed record is produced to order.events")
def produce_malformed(plp):
    marker = f"plp-malformed-{uuid.uuid4().hex}"
    plp.data["malformed"] = marker
    stack.produce(
        stack.ORDER_EVENTS_TOPIC, marker.encode(), b"\x00not-an-event-envelope:" + marker.encode()
    )


@then("the malformed record appears on the settlement dead-letter topic")
def malformed_on_dlq(plp):
    deadline = time.monotonic() + 40
    while time.monotonic() < deadline:
        if stack.find_records(stack.DLQ_TOPIC, plp.data["malformed"].encode(), timeout_s=5):
            return
    raise AssertionError(
        f"malformed record {plp.data['malformed']} is not on {stack.DLQ_TOPIC} "
        "(missing topic or not dead-lettered)"
    )


@any_step("team-payment is stopped")
def stop_payment(plp):
    stack.stop_payment()
    plp.data["payment_stopped"] = True


@any_step("team-payment is started again")
def start_payment(plp):
    stack.start_payment()
    plp.data.pop("payment_stopped", None)
    p.wait_payment_up(plp, _seller(plp))


@any_step("the consumer group is moved back to before those orders' events")
def move_group_back(plp):
    stack.seek_group_to_ms(plp.data["t0_ms"])


@then("once the group has caught up the seller's ledger entries and balance are unchanged")
def group_caught_up_unchanged(plp):
    stack.wait_group_caught_up()
    seller = _seller(plp)
    entries, balance = plp.data["snapshot"]
    p.quiet(
        plp,
        lambda: (
            _same(p.ledger(plp, seller), entries),
            _eq(p.balance(plp, seller), balance),
        ),
    )


def _same(got: list[dict], want: list[dict]) -> None:
    key = lambda e: (e["id"], e["type"], e["amount"], e["status"])  # noqa: E731
    assert sorted(map(key, got)) == sorted(map(key, want)), f"ledger changed: {got}"


def _eq(got: int, want: int) -> None:
    assert got == want, f"balance changed: {got}, want {want}"


# ── startup validation ───────────────────────────────────────────────────
@when(
    "the team-payment image is started with PAYOUT_HOLD_DAYS=-1, with PAYOUT_HOLD_DAYS=abc "
    "and with PAYOUT_HOLD_WINDOW=soon"
)
def start_with_invalid_windows(plp):
    plp.data["startups"] = [
        (key, value, *stack.run_payment_with_env({key: value}))
        for key, value in (
            ("PAYOUT_HOLD_DAYS", "-1"),
            ("PAYOUT_HOLD_DAYS", "abc"),
            ("PAYOUT_HOLD_WINDOW", "soon"),
        )
    ]


@then("each container exits non-zero and its log names the offending key")
def startup_refused(plp):
    problems = []
    for key, value, code, logs in plp.data["startups"]:
        if code is None:
            problems.append(f"{key}={value}: still running after the timeout (no startup error)")
        elif code == 0:
            problems.append(f"{key}={value}: exited 0")
        elif key not in logs:
            problems.append(
                f"{key}={value}: exit {code} but the log does not name {key}: {logs[-300:]!r}"
            )
    assert not problems, "; ".join(problems)
