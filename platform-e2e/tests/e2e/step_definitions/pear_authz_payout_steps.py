"""Payouts require the seller scope (port-edge-authz-residuals / payment-access-control).

Built on the payment-ledger steps (`plp_steps`): its seller/buyer actors, the aged-credit
precondition and the generic call assertions. Only what the scope scenarios add lives here.
"""

from __future__ import annotations

from pytest_bdd import parsers, then, when

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _actor, _last, _seller
from tests.e2e.support import plp_support as p


@when(parsers.parse('"{b}" requests a wallet payout of {amount:d} for their own id'))
def user_requests_wallet_payout(plp, b, amount):
    # RequestWalletPayout takes no seller id: it acts on the caller's own wallet.
    plp.responses["last"] = p.wallet_payout(plp, _actor(plp, b), amount)


@then(parsers.parse('the payout history of "{b}" is empty'))
def user_payout_history_empty(plp, b):
    history = p.payout_history(plp, _actor(plp, b))
    assert not history, f"a payout was recorded for {b}: {history}"


@then(parsers.parse("the payout appears in the seller's payout history with amount {amount:d}"))
def payout_in_history(plp, amount):
    history = p.payout_history(plp, _seller(plp))
    assert any(int(x.get("amount", 0)) == amount for x in history), history
    assert _last(plp).status_code == 200
