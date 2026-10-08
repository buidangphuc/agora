"""repo_doctor env-var check (port-edge-authz-residuals / repo-coherence).

The ghost scenario runs the doctor on a temp copy of the workspace (git-ignored, dependency and
build directories left out) so the real README is never touched; the real-workspace scenario
runs it on the workspace itself. Python 3.12 via uv, as the doctor documents.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest
from pytest_bdd import given, parsers, then, when

WORKSPACE = Path(__file__).resolve().parents[3].parent
SKIP = shutil.ignore_patterns(
    ".git",
    ".codegraph",
    "*.sock",
    "node_modules",
    ".venv",
    "venv",
    "__pycache__",
    ".next",
    ".pytest_cache",
    "allure-results",
    "test-results",
)
GHOST = "E2E_GHOST_SETTING"


@pytest.fixture
def doctor(tmp_path):
    return {"tmp": tmp_path}


def _run(root: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [
            "uv",
            "run",
            "--python",
            "3.12",
            "--no-project",
            "python",
            str(WORKSPACE / "scripts" / "repo_doctor.py"),
            "--root",
            str(root),
        ],
        capture_output=True,
        text=True,
        timeout=300,
    )


@given(
    parsers.parse(
        "a copy of the workspace whose team-referral README env table gains the row {var}"
    )
)
def workspace_copy_with_ghost(doctor, var):
    copy = doctor["tmp"] / "workspace"
    shutil.copytree(WORKSPACE, copy, ignore=SKIP, symlinks=True)
    readme = copy / "team-referral" / "README.md"
    text = readme.read_text(encoding="utf-8")
    # append the row to the existing env table (the one whose header is "| Variable |")
    lines = text.splitlines()
    start = next(i for i, ln in enumerate(lines) if ln.startswith("| Variable"))
    end = start
    while end + 1 < len(lines) and lines[end + 1].startswith("|"):
        end += 1
    lines.insert(end + 1, f"| `{var}` | `unset` |")
    readme.write_text("\n".join(lines) + "\n", encoding="utf-8")
    doctor["root"] = copy


@when("repo_doctor runs on the copy")
def run_on_copy(doctor):
    doctor["run"] = _run(doctor["root"])


@when("repo_doctor runs on the workspace")
def run_on_workspace(doctor):
    doctor["run"] = _run(WORKSPACE)


@then("it exits non-zero and names team-referral and E2E_GHOST_SETTING")
def exits_naming_ghost(doctor):
    run = doctor["run"]
    out = run.stdout + run.stderr
    assert (
        run.returncode != 0
    ), f"repo_doctor passed on a README documenting {GHOST}:\n{out[-1500:]}"
    hits = [ln for ln in out.splitlines() if "team-referral" in ln and GHOST in ln]
    assert hits, f"no finding names team-referral and {GHOST}:\n{out[-1500:]}"


@then("it reports no undocumented-in-code env var")
def no_ghost_env(doctor):
    run = doctor["run"]
    out = run.stdout + run.stderr
    assert run.returncode == 0, f"repo_doctor exited {run.returncode}:\n{out[-2000:]}"
    # The doctor exits non-zero on any such finding; a ghost row must not leak in from elsewhere.
    assert GHOST not in out, out[-1500:]
