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


# RequestWalletPayout books a PAYOUT ledger entry; ListPayoutHistory lists only the
# bank-detail RequestPayout payouts, so the wallet ledger is the record to check.
@then(parsers.parse('the wallet ledger of "{b}" has no payout entry'))
def user_has_no_payout_entry(plp, b):
    payouts = p.rows(plp, _actor(plp, b), "PAYOUT")
    assert not payouts, f"a payout was recorded for {b}: {payouts}"


@then(parsers.parse("a payout entry of {amount:d} appears in the seller's wallet ledger"))
def payout_in_ledger(plp, amount):
    payouts = p.rows(plp, _seller(plp), "PAYOUT")
    assert any(abs(int(x.get("amount", 0))) == amount for x in payouts), payouts
    assert _last(plp).status_code == 200
