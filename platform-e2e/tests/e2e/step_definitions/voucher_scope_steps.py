"""Steps of promo/voucher_scope.feature (change promotion-voucher-scope).

Black box: vouchers are seeded through the gateway CreateVoucher (shop voucher as a seller, platform
voucher as admin); ValidateAndReserve is driven by `support/voucher_scope_support` (grpcurl from a
throwaway container); holds are read from voucher_reservations in promotion_db through psql.
"""

from __future__ import annotations

from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.utils import data as fake
from tests.e2e.step_definitions.flash_sale_ownership_steps import _register_seller
from tests.e2e.support import audit_support as aud
from tests.e2e.support import voucher_scope_support as vs
from tests.e2e.support.world import World

SETTINGS = get_settings()


def _x(world: World) -> dict:
    return world.state.extra


def _ok(result: tuple[str, object]) -> dict:
    code, body = result
    assert code == "ok", f"expected an OK call, got {code}: {body}"
    assert isinstance(body, dict)
    return body


def _record(world: World, key: str, result: tuple[str, object]) -> None:
    st = _x(world)
    st[key] = result
    st["vs_rids"][key] = st["vs_last_rid"]


def _reserve(world: World, key: str, code: str, subtotal: int, seller_id: str = "", principal: str = "service",
             rid: str | None = None) -> None:  # fmt: skip
    st = _x(world)
    st.setdefault("vs_rids", {})
    st["vs_last_rid"] = rid or vs.fresh_reservation()
    _record(world, key, vs.validate_and_reserve(code, st["vs_last_rid"], subtotal, seller_id, principal))


# ── Given ────────────────────────────────────────────────────────────────
@given("a seller with a shop voucher")
def seller_with_shop_voucher(world: World) -> None:
    token = _register_seller(world, "vscope_seller")
    code = vs.seed_shop_voucher(token, vs.fresh_code("VSSHOP"))
    _x(world).update(vs_seller_id=vs.token_sub(token), vs_code=code)


@given("another seller")
def another_seller(world: World) -> None:
    token = _register_seller(world, "vscope_other")
    _x(world)["vs_other_seller_id"] = vs.token_sub(token)


@given(parsers.parse("a platform voucher with minimum spend {min_spend:d}"))
def platform_voucher(world: World, min_spend: int) -> None:
    admin = aud.admin_token(world)
    _x(world)["vs_code"] = vs.seed_platform_voucher(admin, vs.fresh_code("VSPLAT"), min_spend=min_spend)


@given("a registered voucher buyer")
def voucher_buyer(world: World) -> None:
    token = world.service_factory.auth.register(
        fake.unique_username("vscope_buyer"), SETTINGS.seed_password, "buyer"
    )
    _x(world).update(vs_buyer_token=token, vs_buyer_id=vs.token_sub(token))


# ── When ─────────────────────────────────────────────────────────────────
@when("the order service reserves the shop voucher for its own seller")
def reserve_own_seller(world: World) -> None:
    st = _x(world)
    _reserve(world, "first", st["vs_code"], 200000, st["vs_seller_id"])


@when("the order service reserves the shop voucher for the other seller under a new reservation id")
def reserve_other_seller(world: World) -> None:
    st = _x(world)
    _reserve(world, "second", st["vs_code"], 200000, st["vs_other_seller_id"])


@when("the order service reserves the platform voucher with an empty seller id and subtotal 100000")
def reserve_platform(world: World) -> None:
    _reserve(world, "first", _x(world)["vs_code"], 100000)


@when(parsers.parse("the order service reserves the platform voucher with subtotal {subtotal:d}"))
def reserve_platform_subtotal(world: World, subtotal: int) -> None:
    _reserve(world, "first", _x(world)["vs_code"], subtotal)


@when(
    parsers.parse(
        "the order service reserves the platform voucher again under a new reservation id with subtotal {subtotal:d}"
    )
)
def reserve_platform_again(world: World, subtotal: int) -> None:
    _reserve(world, "second", _x(world)["vs_code"], subtotal)


@when("the order service reserves a code that does not exist")
def reserve_unknown(world: World) -> None:
    _reserve(world, "unknown", vs.fresh_code("VSNONE"), 100000)


@when("the order service reserves an empty code")
def reserve_empty(world: World) -> None:
    _reserve(world, "empty", "", 100000)


@when("the buyer reserves the platform voucher under their own preview namespace")
def buyer_own_preview(world: World) -> None:
    st = _x(world)
    rid = f"preview:{st['vs_buyer_id']}:{vs.fresh_reservation('own')}"
    _reserve(world, "first", st["vs_code"], 100000, principal=f"user:{st['vs_buyer_id']}", rid=rid)


@when("the buyer reserves the platform voucher under another user's preview namespace")
def buyer_other_preview(world: World) -> None:
    st = _x(world)
    rid = f"preview:someone-else-{vs.fresh_reservation('other')}:abc"
    _reserve(world, "other_ns", st["vs_code"], 100000, principal=f"user:{st['vs_buyer_id']}", rid=rid)


@when("the buyer reserves the platform voucher with a reservation id that has no preview prefix")
def buyer_no_prefix(world: World) -> None:
    st = _x(world)
    _reserve(world, "no_prefix", st["vs_code"], 100000, principal=f"user:{st['vs_buyer_id']}")


# ── Then ─────────────────────────────────────────────────────────────────
def _assert_valid(body: dict) -> None:
    assert body.get("valid") is True, body
    assert not body.get("reason"), body


@then(parsers.parse('the first reserve is valid with scope "{scope}" and the seller of the voucher'))
@then(parsers.parse('the reserve is valid with scope "{scope}" and the seller of the voucher'))
def valid_with_scope_and_seller(world: World, scope: str) -> None:
    st = _x(world)
    body = _ok(st["first"])
    _assert_valid(body)
    assert vs.scope_of(body) == scope, body
    assert vs.seller_of(body) == st["vs_seller_id"], body


@then(parsers.parse('the reserve is valid with scope "{scope}" and no seller'))
def valid_with_scope_no_seller(world: World, scope: str) -> None:
    body = _ok(_x(world)["first"])
    _assert_valid(body)
    assert vs.scope_of(body) == scope, body
    assert vs.seller_of(body) == "", body


@then("a hold exists for the reservation")
def hold_exists(world: World) -> None:
    rid = _x(world)["vs_rids"]["first"]
    assert len(vs.reservation_rows(rid)) == 1, vs.reservation_rows(rid)


@then(parsers.parse('the first reserve is refused with reason "{reason}"'))
def first_refused(world: World, reason: str) -> None:
    body = _ok(_x(world)["first"])
    assert body.get("valid") is not True, body
    assert body.get("reason") == reason, body


@then("the second reserve is valid")
def second_valid(world: World) -> None:
    _assert_valid(_ok(_x(world)["second"]))


@then(parsers.parse('both reserves report scope "{scope}"'))
def both_scope(world: World, scope: str) -> None:
    st = _x(world)
    for key in ("first", "second"):
        body = _ok(st[key])
        assert vs.scope_of(body) == scope, (key, body)


@then(parsers.parse('the second reserve is refused with reason "{reason}"'))
def second_refused(world: World, reason: str) -> None:
    body = _ok(_x(world)["second"])
    assert body.get("valid") is not True, body
    assert body.get("reason") == reason, body


@then(parsers.parse('the second reserve reports scope "{scope}" and the seller of the voucher'))
def second_scope_seller(world: World, scope: str) -> None:
    st = _x(world)
    body = _ok(st["second"])
    assert vs.scope_of(body) == scope, body
    assert vs.seller_of(body) == st["vs_seller_id"], body


@then("no hold exists for the second reservation")
def no_hold_second(world: World) -> None:
    rid = _x(world)["vs_rids"]["second"]
    assert vs.reservation_rows(rid) == [], vs.reservation_rows(rid)


@then(parsers.parse('the reserve of the existing voucher reports scope "{scope}"'))
def existing_scope(world: World, scope: str) -> None:
    body = _ok(_x(world)["first"])
    _assert_valid(body)
    assert vs.scope_of(body) == scope, body


def _refused_no_scope(world: World, key: str, reason: str) -> None:
    body = _ok(_x(world)[key])
    assert body.get("valid") is not True, body
    assert body.get("reason") == reason, body
    assert vs.scope_of(body) == vs.UNSPECIFIED, body
    assert vs.seller_of(body) == "", body


@then(parsers.parse('the unknown code is refused with reason "{reason}" and no scope'))
def unknown_refused(world: World, reason: str) -> None:
    _refused_no_scope(world, "unknown", reason)


@then(parsers.parse('the empty code is refused with reason "{reason}" and no scope'))
def empty_refused(world: World, reason: str) -> None:
    _refused_no_scope(world, "empty", reason)


@then("the hold is bound to the buyer id of the principal")
def hold_bound_to_principal(world: World) -> None:
    st = _x(world)
    rows = vs.reservation_rows(st["vs_rids"]["first"])
    assert len(rows) == 1, rows
    assert rows[0][1] == st["vs_buyer_id"], rows


@then(parsers.parse('the reserve under their own namespace is valid with scope "{scope}"'))
def own_ns_valid(world: World, scope: str) -> None:
    body = _ok(_x(world)["first"])
    _assert_valid(body)
    assert vs.scope_of(body) == scope, body


def _denied_no_hold(world: World, key: str) -> None:
    st = _x(world)
    code, body = st[key]
    assert code == "permission_denied", f"expected permission_denied, got {code}: {body}"
    assert vs.reservation_rows(st["vs_rids"][key]) == []


@then("the reserve under another user's namespace is denied and no hold exists")
def other_ns_denied(world: World) -> None:
    _denied_no_hold(world, "other_ns")


@then("the reserve without a preview prefix is denied and no hold exists")
def no_prefix_denied(world: World) -> None:
    _denied_no_hold(world, "no_prefix")
