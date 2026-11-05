from __future__ import annotations

import base64
import json
import time
import uuid

from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import audit_support as aud
from tests.e2e.support import oic_order_support as s
from tests.e2e.support.edge_tokens import sign_dev_token

FLASH = "/platform.promotion.v1.FlashSaleService"
SPONSORED = "/platform.promotion.v1.SponsoredService"


def _scopes(token: str) -> list[str]:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return list(json.loads(base64.urlsafe_b64decode(payload)).get("scopes") or [])


def _dev_caller(oic, scopes: str) -> s.Actor:
    sub = f"e2e-pas-{uuid.uuid4().hex[:10]}"
    token = sign_dev_token(
        {
            "sub": sub,
            "typ": "user",
            "name": "e2e-pas",
            "scopes": [x.strip() for x in scopes.split(",") if x.strip()],
            "iat": int(time.time()),
            "exp": int(time.time()) + 600,
        }
    )
    actor = s.Actor(sub, token, "user", sub)
    oic.actors["dev"] = actor
    return actor


def _admin_token(oic) -> str:
    return oic.actors["admin"].token


@given("the seeded admin for the promotion-scope checks")
def seeded_admin(oic):
    s.login_admin(oic)


@given("the seeded admin has created a platform voucher")
def admin_platform_voucher(oic):
    code = av.fresh_code("PAS")
    voucher = av.create_voucher(_admin_token(oic), code)
    oic.data["voucher"] = voucher


@then(parsers.parse('the seeded admin token carries "{a}" and "{b}"'))
def admin_token_scopes(oic, a, b):
    scopes = _scopes(_admin_token(oic))
    assert a in scopes and b in scopes, f"admin scopes: {scopes}"


@then(parsers.parse('neither token of "{x}" and "{y}" carries "{one}" or "{two}"'))
def tokens_lack_scopes(oic, x, y, one, two):
    for name in (x, y):
        scopes = _scopes(oic.actors[name].token)
        assert one not in scopes and two not in scopes, f"{name} scopes: {scopes}"


@when(
    parsers.parse(
        'a caller whose token carries "{scopes}" calls ListCampaigns and DeactivateVoucher'
    )
)
def dev_calls_two(oic, scopes):
    caller = _dev_caller(oic, scopes)
    oic.data["pair"] = [
        s.post(oic, caller, FLASH, "ListCampaigns", {}),
        s.post(oic, caller, s.PROMO, "DeactivateVoucher", {"voucherId": "pas-unknown"}),
    ]


@then(parsers.parse("both promotion calls are answered with HTTP {status:d}"))
def both_status(oic, status):
    for resp in oic.data["pair"]:
        assert resp.status_code == status, f"HTTP {resp.status_code}: {resp.text[:300]}"


@when("a request without a token calls CancelCampaign")
def anon_cancel(oic):
    oic.responses["last"] = s.post(oic, None, FLASH, "CancelCampaign", {"campaignId": "pas-x"})


@then(parsers.parse("the promotion call is answered with HTTP {status:d}"))
def last_status(oic, status):
    resp = oic.responses["last"]
    assert resp.status_code == status, f"HTTP {resp.status_code}: {resp.text[:300]}"


@when("the seeded admin calls ListCampaigns")
def admin_list(oic):
    oic.responses["last"] = s.post(oic, oic.actors["admin"], FLASH, "ListCampaigns", {})


@then("the promotion call succeeds with a campaigns list")
def list_ok(oic):
    body = s.ok(oic.responses["last"])
    assert isinstance(body.get("campaigns", []), list), body


@when("the seeded admin calls DeactivateVoucher on that voucher")
def admin_deactivate(oic):
    oic.responses["last"] = av.deactivate(_admin_token(oic), oic.data["voucher"]["id"])


@when(
    parsers.parse('a caller whose token carries "{scopes}" calls DeactivateVoucher on that voucher')
)
def dev_deactivate(oic, scopes):
    caller = _dev_caller(oic, scopes)
    oic.responses["last"] = av.deactivate(caller.token, oic.data["voucher"]["id"])


@then("the promotion call succeeds")
def call_ok(oic):
    s.ok(oic.responses["last"])


@then(parsers.parse('the promotion call fails with "{code}"'))
def call_fails(oic, code):
    resp = oic.responses["last"]
    assert (
        s.code_of(resp) == code
    ), f"expected {code}, got HTTP {resp.status_code}: {resp.text[:300]}"


@then("that voucher is inactive")
def voucher_inactive(oic):
    voucher = av.get_voucher(_admin_token(oic), oic.data["voucher"]["code"])
    assert voucher and not voucher.get("active"), voucher


@then("that voucher is still active")
def voucher_active(oic):
    voucher = av.get_voucher(_admin_token(oic), oic.data["voucher"]["code"])
    assert voucher and voucher.get("active"), voucher


@when(parsers.parse('a caller whose token carries "{scopes}" creates a "{scope}" voucher'))
def dev_create_voucher(oic, scopes, scope):
    caller = _dev_caller(oic, scopes)
    code = av.fresh_code("PAS")
    oic.data["code"] = code
    oic.responses["last"] = av.gw(caller.token, "CreateVoucher", av.create_body(code, scope=scope))


@then("GetVoucher for that code returns not found")
def voucher_missing(oic):
    assert av.get_voucher(_admin_token(oic), oic.data["code"]) is None


@then("the created voucher belongs to that caller")
def voucher_owned(oic):
    voucher = av.voucher_of(oic.responses["last"])
    assert voucher.get("sellerId") == oic.actors["dev"].user_id, voucher


@then(parsers.parse('the audit log holds no "{action}" event for that code'))
def no_audit(oic, action):
    resp = aud.query_events(
        _admin_token(oic), target_type="voucher", target_id=oic.data["code"], action=action
    )
    assert resp.status_code == 200, f"QueryAuditLog {resp.status_code}: {resp.text[:300]}"
    events = [e for e in resp.json().get("events", []) if e.get("action") == action]
    assert not events, events


@when(
    parsers.parse(
        '"{seller}" calls CancelCampaign and a request without a token calls UpdateVoucher'
    )
)
def seller_and_anon(oic, seller):
    oic.data["pair"] = [
        s.post(oic, oic.actors[seller], FLASH, "CancelCampaign", {"campaignId": "pas-x"}),
        s.post(oic, None, s.PROMO, "UpdateVoucher", {"voucherId": "pas-x", "quota": "9"}),
    ]


@then("the gateway answers HTTP 403 to the seller and HTTP 401 to the anonymous request")
def seller_403_anon_401(oic):
    seller, anon = oic.data["pair"]
    assert seller.status_code == 403, f"seller HTTP {seller.status_code}: {seller.text[:300]}"
    assert anon.status_code == 401, f"anonymous HTTP {anon.status_code}: {anon.text[:300]}"


@when(parsers.parse('a caller whose token carries "{scopes}" creates a flash sale for "{listing}"'))
def dev_flash_sale(oic, scopes, listing):
    caller = _dev_caller(oic, scopes)
    body = ac.admin_create_body(
        oic.listings[listing]["id"], f"pas{uuid.uuid4().hex[:12]}", sale_price=50_000, stock_cap=5
    )
    oic.responses["last"] = s.post(oic, caller, FLASH, "CreateCampaign", body)


@then(parsers.parse('the seeded admin lists no campaign for "{listing}"'))
def no_campaign(oic, listing):
    assert ac.for_listing(_admin_token(oic), oic.listings[listing]["id"]) == []


@when(
    parsers.parse('a caller whose token carries "{scopes}" creates an ad campaign for "{listing}"')
)
def dev_ad_campaign(oic, scopes, listing):
    caller = _dev_caller(oic, scopes)
    oic.responses["last"] = s.post(
        oic,
        caller,
        SPONSORED,
        "CreateAdCampaign",
        {"listingId": oic.listings[listing]["id"], "budget": "1000", "bid": "10"},
    )
