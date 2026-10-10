"""Steps for frontend/ui_token_gates.feature (ui-foundation repo checks).

The checks run the real team-frontend tooling (Tailwind CLI, the token lint, tsc, Vitest,
`npm run check`) in a subprocess; the colour scenario renders the built CSS in the browser.
`team-frontend` needs its node_modules and generated protobuf code (both untracked), so the
frontend directory can be overridden with FRONTEND_DIR.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

import httpx
from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from tests.e2e.support.world import World

_E2E_ROOT = Path(__file__).resolve().parents[3]
FRONTEND = Path(os.environ.get("FRONTEND_DIR", _E2E_ROOT.parent / "team-frontend"))
_MIN_FONT_PX = 12
_TYPE_SCALE = ("xs", "sm", "base", "lg", "xl", "2xl", "3xl", "4xl", "5xl")


def _run(cmd: list[str], cwd: Path, timeout: int = 600) -> subprocess.CompletedProcess[str]:
    env = {k: v for k, v in os.environ.items() if not k.startswith(("GATEWAY", "BASE_URL"))}
    return subprocess.run(
        cmd, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout, check=False
    )


def _bin(name: str) -> str:
    path = FRONTEND / "node_modules" / ".bin" / name
    assert path.exists(), f"{path} missing: run `npm ci` in team-frontend (or set FRONTEND_DIR)"
    return str(path)


def _scratch(world: World) -> Path:
    path = Path(tempfile.mkdtemp(prefix="d1-gates-"))
    world.add_cleanup(lambda: shutil.rmtree(path, ignore_errors=True))
    return path


# ── Brand scale ──────────────────────────────────────────────────────────
@when(parsers.parse('a component uses the "{css_class}" class built from the Tailwind config'))
def build_and_render_class(world: World, css_class: str) -> None:
    work = _scratch(world)
    (work / "in.css").write_text("@tailwind utilities;\n")
    (work / "fixture.html").write_text(f'<div id="probe" class="{css_class}">x</div>')
    built = _run(
        [
            _bin("tailwindcss"),
            "-c",
            str(FRONTEND / "tailwind.config.ts"),
            "--content",
            str(work / "fixture.html"),
            "-i",
            str(work / "in.css"),
            "-o",
            str(work / "out.css"),
        ],
        FRONTEND,
    )
    assert built.returncode == 0, built.stderr[-1500:]
    css = (work / "out.css").read_text()
    assert css_class in css, f"Tailwind generated no rule for {css_class}"
    world.page.set_content(f"<style>{css}</style><div id='probe' class='{css_class}'>x</div>")


@then(parsers.parse('the computed background colour is "{colour}"'))
def computed_background(world: World, colour: str) -> None:
    expect(world.page.locator("#probe")).to_have_css("background-color", colour)


# ── Type scale in the built CSS ──────────────────────────────────────────
@when("the built CSS served by the storefront is searched for type-scale font sizes")
def search_built_css(world: World) -> None:
    base = world.settings.base_url.rstrip("/")
    html = httpx.get(base + "/", timeout=30).text
    hrefs = sorted(set(re.findall(r"/_next/static/css/[^\"']+\.css", html)))
    assert hrefs, "the storefront linked no stylesheet"
    css = "".join(httpx.get(base + h, timeout=30).text for h in hrefs)
    sizes: dict[str, float] = {}
    for name in _TYPE_SCALE:
        m = re.search(r"\.text-" + re.escape(name) + r"\{font-size:([\d.]+)(rem|px)", css)
        if m:
            sizes[name] = float(m.group(1)) * (16 if m.group(2) == "rem" else 1)
    world.state.extra["type_scale_px"] = sizes


@then("every type-scale font size is at least 12px and the scale is present")
def type_scale_at_least_12(world: World) -> None:
    sizes: dict[str, float] = world.state.extra["type_scale_px"]
    assert sizes.get("xs") == _MIN_FONT_PX, f"text-xs should be 12px, got {sizes}"
    assert len(sizes) >= 5, f"type scale missing from the built CSS: {sizes}"
    too_small = {k: v for k, v in sizes.items() if v < _MIN_FONT_PX}
    assert not too_small, f"font sizes below 12px in the type scale: {too_small}"


# ── Token lint ───────────────────────────────────────────────────────────
@when(
    parsers.parse('a component with className "{value}" is added to a copy of the token lint tree')
)
def lint_fixture_with_violation(world: World, value: str) -> None:
    work = _scratch(world)
    (work / "scripts").mkdir()
    (work / "src").mkdir()
    shutil.copy(FRONTEND / "scripts" / "check-tokens.mjs", work / "scripts" / "check-tokens.mjs")
    (work / "package.json").write_text('{"type": "module"}')
    (work / "src" / "Probe.tsx").write_text(
        f'export const Probe = () => (\n  <p className="{value}">x</p>\n);\n'
    )
    world.state.extra["lint"] = _run(["node", "scripts/check-tokens.mjs"], work)


@then(parsers.parse('the token lint exits non-zero and names the file, line and "{token}"'))
def lint_fails_naming_token(world: World, token: str) -> None:
    result: subprocess.CompletedProcess[str] = world.state.extra["lint"]
    assert result.returncode != 0, result.stdout
    assert f"src/Probe.tsx:2  {token}" in result.stdout, result.stdout


@when("the token lint runs on the team-frontend tree")
def lint_real_tree(world: World) -> None:
    world.state.extra["lint"] = _run(["node", "scripts/check-tokens.mjs"], FRONTEND)


@then("the token lint exits zero and reports 0 violations")
def lint_clean(world: World) -> None:
    result: subprocess.CompletedProcess[str] = world.state.extra["lint"]
    assert result.returncode == 0, result.stdout
    assert "0 violation(s)" in result.stdout, result.stdout


# ── ActionResult typing ──────────────────────────────────────────────────
_ACTION_GOOD = """import { fail, type ActionResult } from "@/lib/action-result";
const r: ActionResult<number> = fail("Hết hàng");
if (!r.ok) {
  const message: string = r.error;
  if (message !== "Hết hàng") throw new Error("unexpected");
}
"""
_ACTION_BAD = _ACTION_GOOD.replace(
    "  const message: string = r.error;",
    "  const leaked = r.data;\n  const message: string = r.error;",
)


def _tsc(work: Path) -> subprocess.CompletedProcess[str]:
    config = {
        "extends": str(FRONTEND / "tsconfig.json"),
        "compilerOptions": {
            "noEmit": True,
            "incremental": False,
            "baseUrl": str(FRONTEND),
            "paths": {"@/*": ["src/*"]},
            "plugins": [],
        },
        "include": ["probe.ts"],
        "exclude": [],
    }
    (work / "tsconfig.json").write_text(json.dumps(config))
    return _run([_bin("tsc"), "-p", str(work / "tsconfig.json")], work)


@when("a Server Action result is narrowed on a failure")
def compile_action_result(world: World) -> None:
    good, bad = _scratch(world), _scratch(world)
    (good / "probe.ts").write_text(_ACTION_GOOD)
    (bad / "probe.ts").write_text(_ACTION_BAD)
    world.state.extra["tsc_good"] = _tsc(good)
    world.state.extra["tsc_bad"] = _tsc(bad)


@then("the compiler accepts the error branch and rejects reading data on it")
def compiler_verdicts(world: World) -> None:
    good: subprocess.CompletedProcess[str] = world.state.extra["tsc_good"]
    bad: subprocess.CompletedProcess[str] = world.state.extra["tsc_bad"]
    assert good.returncode == 0, good.stdout[-1500:]
    assert bad.returncode != 0, "reading `data` on the failure branch must not compile"
    assert "'data' does not exist" in bad.stdout, bad.stdout[-1500:]


# ── npm scripts ──────────────────────────────────────────────────────────
@when(parsers.parse('a developer runs "{command}" in team-frontend with no backend configured'))
@when(parsers.parse('a developer runs "{command}" in team-frontend'))
def run_npm(world: World, command: str) -> None:
    world.state.extra["npm"] = _run(command.split(), FRONTEND)


@then("the Vitest suite runs on jsdom and every test file passes")
def vitest_passes(world: World) -> None:
    result: subprocess.CompletedProcess[str] = world.state.extra["npm"]
    out = result.stdout + result.stderr
    assert result.returncode == 0, out[-2500:]
    assert re.search(r"Test Files\s+\d+ passed \(\d+\)", out), out[-1500:]
    assert "failed" not in re.search(r"Test Files.*", out).group(0), out[-1500:]
    config = (FRONTEND / "vitest.config.ts").read_text()
    assert 'environment: "jsdom"' in config


@then("Biome, tsc, the token lint and Vitest all pass and the command exits 0")
def check_passes(world: World) -> None:
    result: subprocess.CompletedProcess[str] = world.state.extra["npm"]
    out = result.stdout + result.stderr
    assert result.returncode == 0, out[-2500:]
    assert "token lint: 0 violation(s)" in out, out[-1500:]
    assert re.search(r"Test Files\s+\d+ passed", out), out[-1500:]
