"""The eval gate, run as a black box: `make -C team-ai eval` (spec requirement 9).

The report is the JSON line `scripts.run_eval` prints (`pass_rate`, `failures`, ...). The gate
grades the completions surface against the built-in echo handler; judge cases are skipped when
`JUDGE_CHAT_MODEL` is empty. Needs `uv` and the team-ai dependencies on the host, like
`make -C team-ai test`.
"""

from __future__ import annotations

import json
import os
import subprocess

from pytest_bdd import given, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

TEAM_AI = pe.REPO_ROOT / "team-ai"
EVAL_TIMEOUT_S = 900


def _x(world: World) -> dict:
    return world.state.extra


def _run_eval() -> subprocess.CompletedProcess[str]:
    # Settings needs REDIS_HOST to load; the eval never connects to it.
    env = dict(os.environ, JUDGE_CHAT_MODEL="", ENVIRONMENT="local", REDIS_HOST="localhost")
    return subprocess.run(
        ["make", "-C", str(TEAM_AI), "eval"],
        capture_output=True,
        text=True,
        timeout=EVAL_TIMEOUT_S,
        env=env,
    )


def _report(proc: subprocess.CompletedProcess[str]) -> dict:
    for line in reversed((proc.stdout + "\n" + proc.stderr).splitlines()):
        line = line.strip()
        if line.startswith("{") and "pass_rate" in line:
            return json.loads(line)
    raise AssertionError(f"make eval printed no JSON report:\n{proc.stdout}\n{proc.stderr}")


@given("team-ai's eval gate with no judge configured")
def no_judge(world: World) -> None:
    assert (TEAM_AI / "Makefile").exists(), f"no team-ai Makefile at {TEAM_AI}"


@when("make eval runs in team-ai with no judge configured")
def run_plain(world: World) -> None:
    _x(world)["apr_eval"] = _run_eval()


@then("it exits zero with a pass rate of 100% and the judge cases skipped")
def eval_passes(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["apr_eval"]
    output = proc.stdout + "\n" + proc.stderr
    assert proc.returncode == 0, f"exit {proc.returncode}:\n{output[-2000:]}"
    report = _report(proc)
    assert report["pass_rate"] == 1.0 and not report.get("failures"), report
    assert report["scored"] > 0, f"the gate scored nothing: {report}"
    assert report["skipped_judge"] > 0, f"no judge case was skipped: {report}"
