"""Steps for frontend/audit_viewer_ui.feature (frontend-audit-viewer, initiative q3-audit-log).

Events are seeded with audit_support.seed_events (explicit created_at, unique target/actor per run).
Rendered rows are located by role (a row that is not the header row), filter inputs by their
GET-form field name (the URL parameter names fixed by the spec).
"""

from __future__ import annotations

import re
import uuid
from datetime import datetime, timedelta, timezone

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

_INVALID_RANGE = (
    "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu."
)


def _x(world: World) -> dict:
    return world.state.extra


def _url(world: World, **params: str) -> str:
    from urllib.parse import urlencode

    qs = urlencode({k: v for k, v in params.items() if v})
    return f"{world.settings.base_url}/admin/audit" + (f"?{qs}" if qs else "")


def _rows(world: World) -> Locator:
    page = world.page
    return page.get_by_role("row").filter(has_not=page.get_by_role("columnheader"))


def _next_link(world: World) -> Locator:
    return world.page.get_by_role("link", name="Trang sau")


def _wait_table(world: World) -> None:
    expect(world.page.get_by_role("heading", name="Nhật ký kiểm toán")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


def _submit(world: World, **fields: str) -> None:
    form = world.page.locator("form").filter(has=world.page.get_by_role("button", name="Lọc"))
    for name, value in fields.items():
        form.locator(f'input[name="{name}"]').fill(value)
    form.get_by_role("button", name="Lọc").click()


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("{n:d} audit events seeded one second apart for one fresh target"))
def seed_paging(world: World, n: int) -> None:
    ttype, tid = aud.fresh_target()
    base = datetime(2026, 9, 1, 10, 0, 0, tzinfo=timezone.utc)
    aud.seed_events(
        [
            {
                "actor_id": f"e2e-actor-{uuid.uuid4().hex[:8]}",
                "action": "e2e.page",
                "target_type": ttype,
                "target_id": tid,
                "metadata": {"seq": str(i)},
                "created_at": (base + timedelta(seconds=i)).strftime("%Y-%m-%dT%H:%M:%SZ"),
            }
            for i in range(n)
        ]
    )
    _x(world).update(aud_type=ttype, aud_id=tid, aud_n=n)


@given("events seeded for two different targets of the same type")
def seed_two_targets(world: World) -> None:
    ttype, first = aud.fresh_target()
    second = first + "-other"
    for tid in (first, second):
        aud.seed_events(
            [
                {
                    "action": "order.force_fail",
                    "target_type": ttype,
                    "target_id": tid,
                    "created_at": "2026-09-01T10:00:00Z",
                },
                {
                    "action": "order.status",
                    "target_type": ttype,
                    "target_id": tid,
                    "created_at": "2026-09-01T10:00:01Z",
                },
            ]
        )
    _x(world).update(aud_type=ttype, aud_id=first, aud_other=second)


@given(
    parsers.parse(
        'events with the actions "{a1}" and "{a2}" seeded for a unique actor and another actor'
    )
)
def seed_actions_actors(world: World, a1: str, a2: str) -> None:
    ttype, tid = aud.fresh_target()
    actor = f"e2e-actor-{uuid.uuid4().hex[:10]}"
    other = f"e2e-other-{uuid.uuid4().hex[:10]}"
    rows = [
        (actor, a1, "2026-09-01T10:00:00Z"),
        (actor, a2, "2026-09-01T10:00:01Z"),
        (other, a1, "2026-09-01T10:00:02Z"),
        (other, a2, "2026-09-01T10:00:03Z"),
    ]
    aud.seed_events(
        [
            {
                "actor_id": a,
                "action": act,
                "target_type": ttype,
                "target_id": tid,
                "created_at": ts,
            }
            for a, act, ts in rows
        ]
    )
    _x(world).update(aud_type=ttype, aud_id=tid, aud_actor=actor, aud_other_actor=other)


@given(
    parsers.parse(
        "events at {inside} and {outside} seeded for one fresh target"
    )
)
def seed_day_edge(world: World, inside: str, outside: str) -> None:
    ttype, tid = aud.fresh_target()
    aud.seed_events(
        [
            {
                "action": "e2e.edge.inside",
                "target_type": ttype,
                "target_id": tid,
                "created_at": inside,
            },
            {
                "action": "e2e.edge.outside",
                "target_type": ttype,
                "target_id": tid,
                "created_at": outside,
            },
        ]
    )
    _x(world).update(aud_type=ttype, aud_id=tid)


@given(parsers.parse("1 audit event seeded on {day} for one fresh target"))
def seed_one_on_day(world: World, day: str) -> None:
    ttype, tid = aud.fresh_target()
    aud.seed_events(
        [
            {
                "action": "e2e.day",
                "target_type": ttype,
                "target_id": tid,
                "created_at": f"{day}T05:00:00Z",
            }
        ]
    )
    _x(world).update(aud_type=ttype, aud_id=tid)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the admin opens the audit page")
@when("the visitor opens the audit page")
def open_audit(world: World) -> None:
    world.page.goto(_url(world), wait_until="domcontentloaded")


@when("the admin opens the audit page filtered to that target")
def open_filtered(world: World) -> None:
    x = _x(world)
    world.page.goto(_url(world, target_type=x["aud_type"], target_id=x["aud_id"]))
    _wait_table(world)


@when("the admin opens the audit page filtered to the first target")
def open_filtered_first(world: World) -> None:
    open_filtered(world)


@when("the admin follows the next page link")
def follow_next(world: World) -> None:
    _next_link(world).click()
    world.page.wait_for_url(re.compile(r"cursor="), timeout=timeouts.NAVIGATION)
    _wait_table(world)


@when(parsers.parse('the admin submits the form with the action "{action}" and the seeded actor id'))
def submit_action_actor(world: World, action: str) -> None:
    _submit(world, action=action, actor_id=_x(world)["aud_actor"])


@when(parsers.parse('the admin submits the form with from "{frm}" and to "{to}"'))
def submit_dates(world: World, frm: str, to: str) -> None:
    form = world.page.locator("form").filter(has=world.page.get_by_role("button", name="Lọc"))
    form.get_by_label("Từ ngày").fill(frm)
    form.get_by_label("Đến ngày").fill(to)
    form.get_by_role("button", name="Lọc").click()
    world.page.wait_for_url(re.compile(r"from="), timeout=timeouts.NAVIGATION)


@when(parsers.parse('the admin opens the cockpit and clicks "{name}"'))
def cockpit_click(world: World, name: str) -> None:
    world.page.goto(f"{world.settings.base_url}/admin/cockpit", wait_until="domcontentloaded")
    link = world.page.get_by_role("link", name=name)
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()


# ── Thens ────────────────────────────────────────────────────────────────
@then("the audit table shows 20 rows with the newest event first and a next page link")
def page_one(world: World) -> None:
    rows = _rows(world)
    expect(rows).to_have_count(20, timeout=timeouts.NAVIGATION)
    n = _x(world)["aud_n"]
    expect(rows.first).to_contain_text(re.compile(rf"seq=\s*{n - 1}\b"))
    expect(rows.last).to_contain_text(re.compile(rf"seq=\s*{n - 20}\b"))
    expect(_next_link(world)).to_be_visible()


@then("the audit table shows the remaining 5 events and no next page link")
def page_two(world: World) -> None:
    rows = _rows(world)
    expect(rows).to_have_count(5, timeout=timeouts.NAVIGATION)
    expect(rows.first).to_contain_text(re.compile(r"seq=\s*4\b"))
    expect(rows.last).to_contain_text(re.compile(r"seq=\s*0\b"))
    expect(_next_link(world)).to_have_count(0)


@then("every audit row shows the first target id and no row shows the second")
def only_first_target(world: World) -> None:
    x = _x(world)
    rows = _rows(world)
    expect(rows).to_have_count(2, timeout=timeouts.NAVIGATION)
    for i in range(rows.count()):
        text = rows.nth(i).inner_text()
        assert x["aud_id"] in text, text
        assert x["aud_other"] not in text, text
    expect(world.page.get_by_text(x["aud_other"])).to_have_count(0)


@then(
    parsers.parse(
        'only audit rows with the action "{action}" and the seeded actor are listed'
    )
)
def only_action_actor(world: World, action: str) -> None:
    x = _x(world)
    world.page.wait_for_url(re.compile(r"actor_id="), timeout=timeouts.NAVIGATION)
    rows = _rows(world)
    expect(rows).to_have_count(1, timeout=timeouts.NAVIGATION)
    text = rows.first.inner_text()
    assert action in text and x["aud_actor"] in text, text
    assert "order.force_fail" not in text and x["aud_other_actor"] not in text, text


@then("the audit list shows the 16:59:59Z event and does not show the 17:00:00Z event")
def day_edge(world: World) -> None:
    expect(world.page.get_by_text("e2e.edge.inside")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text("e2e.edge.outside")).to_have_count(0)


@then(parsers.parse('the audit page shows "{text}" and no audit rows'))
def empty_state(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_rows(world)).to_have_count(0)


@then("the audit page shows the invalid-range message and no audit table")
def invalid_range(world: World) -> None:
    expect(world.page.get_by_text(_INVALID_RANGE)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("table")).to_have_count(0)


@then(parsers.parse('the form keeps the dates "{frm}" and "{to}"'))
def form_keeps_dates(world: World, frm: str, to: str) -> None:
    expect(world.page.get_by_label("Từ ngày")).to_have_value(frm)
    expect(world.page.get_by_label("Đến ngày")).to_have_value(to)


@then("the browser lands on the login page")
def on_login(world: World) -> None:
    world.page.wait_for_url(re.compile(r"/login"), timeout=timeouts.NAVIGATION)


@then(parsers.parse('the audit page shows "{text}" and no audit table'))
def forbidden(world: World, text: str) -> None:
    expect(world.page.get_by_text(text).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("table")).to_have_count(0)


@then(parsers.parse('the browser is on the audit page with the heading "{heading}"'))
def on_audit_heading(world: World, heading: str) -> None:
    world.page.wait_for_url(re.compile(r"/admin/audit"), timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("heading", name=heading)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
