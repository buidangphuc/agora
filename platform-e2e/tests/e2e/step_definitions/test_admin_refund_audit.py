# ruff: noqa: F811
"""Binds payment/admin_refund_audit.feature (payment-admin-refund-audit).

Refund, payment-read and call-result steps are reused from plp_steps / prm_pay_steps. Audit
assertions and the team-audit stop/start use support/audit_support.py (import only); that helper
takes the browser World, so a minimal stand-in carries the three things it touches.
"""

from __future__ import annotations

from types import SimpleNamespace

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _actor, _paid, plp  # noqa: F401
from tests.e2e.step_definitions.prm_pay_steps import *  # noqa: F401,F403
from tests.e2e.support import audit_support as audit
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import AUTH, login_admin, post

from pytest_bdd import scenarios

scenarios("payment/admin_refund_audit.feature")

TARGET_TYPE = "payment"
ACTION = "payment.admin_refund"


class _Auth:
    def __init__(self, w):
        self._w = w

    def login(self, username: str, password: str) -> str:
        resp = post(self._w, None, AUTH, "Login", {"username": username, "password": password})
        return (resp.json().get("result") or {}).get("token", "")


class _AuditWorld:
    """Duck-typed stand-in for support.world.World: service_factory.auth, state.extra, add_cleanup."""

    def __init__(self, w):
        self.service_factory = SimpleNamespace(auth=_Auth(w))
        self.state = SimpleNamespace(extra={})
        self._cleanups = []

    def add_cleanup(self, fn):
        self._cleanups.append(fn)

    def run_cleanups(self):
        while self._cleanups:
            self._cleanups.pop()()


@pytest.fixture
def aw(plp):
    world = _AuditWorld(plp)
    yield world
    world.run_cleanups()  # starts team-audit again if a step left it stopped


@given("a logged-in admin")
def logged_in_admin(plp):
    login_admin(plp)


@when(parsers.parse("the admin refunds {amount:d} of the payment with a fresh refund id"))
@given(parsers.parse("the admin refunds {amount:d} of the payment with a fresh refund id"))
def admin_refunds(plp, amount):
    plp.responses["last"] = p.refund(plp, plp.actors["admin"], _paid(plp)["tx"], amount)


@when(parsers.parse('"{name}" refunds {amount:d} of the payment with a fresh refund id'))
def actor_refunds(plp, name, amount):
    plp.responses["last"] = p.refund(plp, _actor(plp, name), _paid(plp)["tx"], amount)


@given("team-audit is stopped")
def audit_stopped(aw):
    audit.stop_audit(aw)


@when("team-audit is started again")
def audit_started(aw):
    audit.start_audit(aw)


@then(
    parsers.parse(
        "exactly one payment.admin_refund audit event exists for the payment with the admin as "
        'actor and metadata amount "{amount}" and the order id'
    )
)
def one_audit_event(plp, aw, amount):
    paid = _paid(plp)
    audit.assert_one_event(
        aw,
        target_type=TARGET_TYPE,
        target_id=paid["tx"],
        action=ACTION,
        actor_id=plp.actors["admin"].user_id,
        metadata={"order_id": paid["order"], "amount": amount},
    )


@then("no audit event exists for the payment")
def no_audit_event(plp, aw):
    audit.assert_no_event(aw, target_type=TARGET_TYPE, target_id=_paid(plp)["tx"])


# ── payment-refund-audit-id ──────────────────────────────────────────────
def _rid(plp, label: str) -> str:
    """Per-run unique refund id for a label; the same label always maps to the same id in a scenario."""
    ids = plp.data.setdefault("labelled_refund_ids", {})
    if label not in ids:
        ids[label] = f"{label}-{p.new_refund_id()[-8:]}"[:64]
    return ids[label]


@when(parsers.parse('the admin refunds {amount:d} of the payment with refund id "{label}"'))
def admin_refunds_with_id(plp, amount, label):
    plp.responses["last"] = p.refund(
        plp, plp.actors["admin"], _paid(plp)["tx"], amount, refund_id=_rid(plp, label)
    )


@then(
    parsers.parse(
        "exactly one payment.admin_refund audit event exists for the payment with metadata "
        'refund id "{label}" and amount "{amount}"'
    )
)
def one_audit_event_with_refund_id(plp, aw, label, amount):
    paid = _paid(plp)
    audit.assert_one_event(
        aw,
        target_type=TARGET_TYPE,
        target_id=paid["tx"],
        action=ACTION,
        actor_id=plp.actors["admin"].user_id,
        metadata={"refund_id": _rid(plp, label), "amount": amount, "order_id": paid["order"]},
    )


@then(
    parsers.parse(
        "every payment.admin_refund audit event for the payment carries a refund id of "
        '"{a}" or "{b}" and both appear'
    )
)
def every_event_has_refund_id(plp, aw, a, b):
    events = audit.events_for(aw, TARGET_TYPE, _paid(plp)["tx"], action=ACTION)
    events = [e for e in events if e.get("action") == ACTION]
    allowed = {_rid(plp, a), _rid(plp, b)}
    seen = {(e.get("metadata") or {}).get("refund_id") for e in events}
    assert events, "no payment.admin_refund event was written"
    missing = [e for e in events if not (e.get("metadata") or {}).get("refund_id")]
    assert not missing, f"events lack refund_id: {missing}"
    assert seen == allowed, f"refund ids in audit {seen}, want exactly {allowed}: {events}"


@when(parsers.parse('"{name}" queries the audit log for the payment'))
def actor_queries_audit(plp, name):
    plp.responses["audit_query"] = audit.query_events(
        _actor(plp, name).token, target_type=TARGET_TYPE, target_id=_paid(plp)["tx"]
    )


@then(parsers.parse('the audit query is denied with "{code}" and returns no events'))
def audit_query_denied(plp, code):
    from tests.e2e.support.oic_order_support import code_of

    resp = plp.responses["audit_query"]
    assert code_of(resp) == code, f"want {code}, got {resp.status_code} {resp.text}"
    assert not (resp.json().get("events") or []), f"metadata leaked: {resp.text}"
