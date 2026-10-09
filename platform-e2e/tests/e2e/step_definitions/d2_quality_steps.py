"""Shared quality steps (layout shift, lazy images, token lint, source checks) for the
d2 UI changes: ui-phase-cart-checkout, ui-phase-orders and ui-phase-seller."""

from __future__ import annotations

import re
import subprocess
from pathlib import Path

from playwright.sync_api import Page
from pytest_bdd import parsers, then, when

from tests.e2e.support.world import World

REPO = Path(__file__).resolve().parents[4]
FRONTEND = REPO / "team-frontend"

CLS_SCRIPT = """
window.__cls = 0;
new PerformanceObserver((list) => {
  for (const e of list.getEntries()) if (!e.hadRecentInput) window.__cls += e.value;
}).observe({type: 'layout-shift', buffered: true});
"""


def install_cls(world: World) -> None:
    """Accumulate layout-shift entries from the start of every document."""
    if not world.state.extra.get("d2_cls_installed"):
        world.state.extra["d2_cls_installed"] = True
        world.page.add_init_script(CLS_SCRIPT)


def cls(page: Page) -> float:
    return page.evaluate("window.__cls || 0")


# ── Token lint ───────────────────────────────────────────────────────────
@then(parsers.parse('the token lint reports no violation under "{paths}"'))
def token_lint_clean(paths: str) -> None:
    wanted = [p.strip() for p in paths.split(",")]
    for p in wanted:
        assert (FRONTEND / p).exists(), f"{p} does not exist: the lint would be vacuous"
    proc = subprocess.run(
        ["node", "scripts/check-tokens.mjs", *wanted],
        cwd=FRONTEND,
        capture_output=True,
        text=True,
        timeout=120,
        check=False,
    )
    assert proc.returncode == 0 and "0 violation(s)" in proc.stdout, proc.stdout + proc.stderr


# ── Source inspection ────────────────────────────────────────────────────
def has_use_client(path: Path) -> bool:
    return bool(re.search(r"^\s*[\"']use client[\"']", path.read_text("utf-8"), re.M))


@then(parsers.parse('none of the sources "{paths}" contains a "use client" directive'))
def no_use_client(paths: str) -> None:
    for rel in (p.strip() for p in paths.split(",")):
        path = FRONTEND / "src" / rel
        assert path.exists(), path
        assert not has_use_client(path), f"{rel} is a client component"


# ── Images ───────────────────────────────────────────────────────────────
@then("thumbnails in the second and third shop groups are lazy and the first group's are eager")
def lazy_groups(world: World) -> None:
    """Read the server-rendered markup: the browser swaps a failed picture for its fallback."""
    resp = world.context.request.get(f"{world.settings.base_url}/cart")
    assert resp.ok, resp.status
    html = resp.text()
    loading = []
    for shop in world.state.extra["d2_shops"]:
        tag = re.search(rf'<img[^>]*alt="{re.escape(shop["title"])}"[^>]*>', html)
        assert tag, f"no <img> for {shop['title']} in the server HTML"
        m = re.search(r'loading="(\w+)"', tag.group(0))
        loading.append(m.group(1) if m else "")
    assert loading == ["eager", "lazy", "lazy"], loading


@when("the viewport is 375 by 812")
def viewport_mobile(world: World) -> None:
    world.page.set_viewport_size({"width": 375, "height": 812})
