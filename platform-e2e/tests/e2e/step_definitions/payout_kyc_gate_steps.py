"""Payouts require a VERIFIED seller (change payment-payout-kyc-gate).

Built on the payment-ledger steps (`plp_steps`: actors, aged credits, payout calls, generic call
assertions) and the payout-scope steps. Verification state is driven only through the real KYC
RPCs (`flows.kyc_flow`); the scenario owns it, so automatic verification of the shared payout
helpers is switched off (`manual_kyc`). team-verification is stopped and started with docker.
"""

from __future__ import annotations

import time

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import kyc_flow as k
from tests.e2e.step_definitions.pear_authz_payout_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _actor, _last, _seller, any_step
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import Actor, code_of, post

REFUSAL = "seller verification required"


def _manual(plp) -> None:
    plp.data["manual_kyc"] = True


@pytest.fixture
def kyc_gate_cleanup(plp):
    yield
    if plp.data.pop("verification_stopped", None):
        stack.docker("start", stack.container_name("team-verification"), check=False)
        stack.wait_settled("team-verification")


# ── verification state of the scenario's seller ──────────────────────────
@given("the seller is verified through the KYC RPCs")
def seller_verified(plp):
    _manual(plp)
    k.verify_seller(_seller(plp).token)


@given("the seller has never submitted verification")
def seller_never_submitted(plp):
    _manual(plp)
    assert k.status_of(_seller(plp).token) == k.NOT_SUBMITTED


@given("the seller has a PENDING verification submission")
def seller_pending(plp):
    _manual(plp)
    k.submit(_seller(plp).token)
    assert k.status_of(_seller(plp).token) == k.PENDING


@given("the seller has a REJECTED verification submission")
def seller_rejected(plp):
    _manual(plp)
    k.reject_seller(_seller(plp).token)


@when("the seller submits a newer verification that stays PENDING")
def seller_newer_pending(plp):
    k.submit(_seller(plp).token)
    assert k.status_of(_seller(plp).token) == k.PENDING


@given(parsers.parse('a second seller "{name}" who has never submitted verification'))
def second_seller(plp, name):
    other = p.o.register(plp, name, "seller")
    assert k.status_of(other.token) == k.NOT_SUBMITTED


# ── team-verification outage ─────────────────────────────────────────────
@any_step("team-verification is stopped")
def verification_stopped(plp, kyc_gate_cleanup):
    stack.docker("stop", stack.container_name("team-verification"))
    plp.data["verification_stopped"] = True


@any_step("team-verification is started again")
def verification_started(plp, kyc_gate_cleanup):
    stack.docker("start", stack.container_name("team-verification"))
    plp.data.pop("verification_stopped", None)
    stack.wait_settled("team-verification")


@when(parsers.parse("the seller requests a bank payout of {amount:d} and the call is timed"))
def requests_bank_payout_timed(plp, amount):
    started = time.monotonic()
    plp.responses["last"] = p.bank_payout(plp, _seller(plp), amount)
    plp.data["elapsed_s"] = time.monotonic() - started


@then(parsers.parse('the call fails with "{code}" within {seconds:d} seconds'))
def fails_within(plp, code, seconds):
    resp = _last(plp)
    assert code_of(resp) == code, f"expected {code}, got {p.describe(resp)}"
    assert (
        plp.data["elapsed_s"] <= seconds
    ), f"took {plp.data['elapsed_s']:.1f}s, want <= {seconds}s"


# ── payout assertions ────────────────────────────────────────────────────
@then(parsers.parse("the seller's payout history lists a payout of {amount:d}"))
def history_lists_payout(plp, amount):
    history = p.payout_history(plp, _seller(plp))
    assert any(int(x.get("amount", 0)) == amount for x in history), history


@then(parsers.parse("the seller's payout history lists exactly {n:d} payout"))
def history_lists_n(plp, n):
    history = p.payout_history(plp, _seller(plp))
    assert len(history) == n, history


@then(parsers.parse("the seller's ledger has exactly {n:d} PAYOUT entry"))
def ledger_has_n_payouts(plp, n):
    entries = p.rows(plp, _seller(plp), p.PAYOUT)
    assert len(entries) == n, entries


# ── authorization before the gate ────────────────────────────────────────
@when(parsers.parse('"{b}" requests a bank payout from the seller\'s wallet'))
def buyer_requests_foreign_bank_payout(plp, b):
    buyer = _actor(plp, b)
    plp.responses["last"] = _bank_payout_for(plp, buyer, _seller(plp).user_id)


@when(parsers.parse('"{name}" requests a bank payout naming the verified seller\'s id'))
def other_requests_with_verified_id(plp, name):
    plp.responses["last"] = _bank_payout_for(plp, _actor(plp, name), _seller(plp).user_id)


def _bank_payout_for(plp, actor: Actor, seller_id: str) -> httpx.Response:
    return post(
        plp,
        actor,
        p.PAYMENT,
        "RequestPayout",
        {
            "sellerId": seller_id,
            "amount": 100000,
            "bankCode": "VCB",
            "accountNumber": "0123456789",
            "accountName": "E2E SELLER",
        },
    )


@then(parsers.parse('"{name}" has no payout in the payout history'))
def other_has_no_payout(plp, name):
    history = p.payout_history(plp, _actor(plp, name))
    assert not history, f"a payout was booked for {name}: {history}"


@then(parsers.parse("the seller's verification status is {status}"))
def seller_status_is(plp, status):
    assert k.status_of(_seller(plp).token) == f"VERIFICATION_STATUS_{status}"


# ── configuration ────────────────────────────────────────────────────────
@when("team-payment is started with UPSTREAM_VERIFICATION_ADDR unset and then blank")
def start_without_address(plp):
    plp.data["startups"] = [
        (mode, *stack.run_payment_with_env(env))
        for mode, env in (("unset", {}), ("blank", {"UPSTREAM_VERIFICATION_ADDR": ""}))
    ]


@then(parsers.parse('each container exits non-zero and its log names "{key}"'))
def startup_refused_naming(plp, key):
    problems = []
    for mode, code, logs in plp.data["startups"]:
        if code is None:
            problems.append(f"{mode}: still running after the timeout (no startup error)")
        elif code == 0:
            problems.append(f"{mode}: exited 0")
        elif key not in logs:
            problems.append(f"{mode}: exit {code} but the log does not name {key}: {logs[-300:]!r}")
    assert not problems, "; ".join(problems)


# ── seeded demo seller ───────────────────────────────────────────────────
@given("the marketplace seed has run")
def seed_has_run(plp):
    from src.utils import get_test_data_manager

    seller = get_test_data_manager().get_user_by_role("seller")
    resp = httpx.post(
        f"{plp.gateway}/platform.identity.v1.AuthService/Login",
        json={"username": seller.username, "password": seller.password},
        timeout=45,
    )
    assert resp.status_code == 200, f"seeded seller login failed: {resp.text[:300]}"
    token = (resp.json().get("result") or {}).get("token", "")
    assert token, "seeded seller login returned no token"
    plp.actors["seeded"] = Actor(seller.username, token, "seller", p.o._sub(token))


@when(parsers.parse("the seeded shop seller requests a wallet payout of {amount:d}"))
def seeded_requests_payout(plp, amount):
    plp.data["manual_kyc"] = True
    plp.responses["last"] = p.wallet_payout(plp, _actor(plp, "seeded"), amount)


@then("the seeded seller's verification status is VERIFIED")
def seeded_verified(plp):
    assert k.status_of(_actor(plp, "seeded").token) == k.VERIFIED


@then("the call is not refused for seller verification")
def not_refused_for_verification(plp):
    resp = _last(plp)
    assert p.message_of(resp) != REFUSAL, f"seeded seller was gated: {p.describe(resp)}"
    assert code_of(resp) != "unavailable", f"verification unreadable: {p.describe(resp)}"
