"""The eval gate, run as a black box: `make -C team-ai eval` (spec requirement 9).

The report is the JSON line `scripts.run_eval` prints (`pass_rate`, `failures`, ...). The
chat resilience cases are recognised by name in the output (primary 429 fallback, all
targets failing, failure after the first chunk). The judge scenario feeds an extra eval file
through `EVAL_EXTRA_CASES` and clears `JUDGE_CHAT_MODEL`. Needs `uv` and the team-ai
dependencies on the host, like `make -C team-ai test`.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile

from pytest_bdd import given, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

TEAM_AI = pe.REPO_ROOT / "team-ai"
EVAL_TIMEOUT_S = 900
JUDGE_CASE_ID = "e2e-judge-case-without-judge"
CASES = {
    "primary 429 fallback": r"primary[\s._-]*429",
    "all targets failing": r"all[\s._-]*(targets?[\s._-]*)?(fail|down|500)",
    "failure after the first chunk": r"(after|mid)[\s._-]*(the[\s._-]*)?first[\s._-]*chunk",
}


def _x(world: World) -> dict:
    return world.state.extra


def _run_eval(extra_cases: str | None) -> subprocess.CompletedProcess[str]:
    env = dict(os.environ, JUDGE_CHAT_MODEL="", ENVIRONMENT="local")
    env.pop("EVAL_EXTRA_CASES", None)
    if extra_cases:
        env["EVAL_EXTRA_CASES"] = extra_cases
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


@given("an extra eval set with one judge case")
def extra_cases(world: World) -> None:
    handle = tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False, encoding="utf-8")
    case = {
        "id": JUDGE_CASE_ID,
        "input": {"messages": [{"role": "user", "content": "hello"}]},
        "expected": "a polite greeting",
        "metadata": {"evaluator": "judge"},
    }
    handle.write(json.dumps(case) + "\n")
    handle.close()
    _x(world)["apr_extra"] = handle.name
    world.add_cleanup(lambda: os.unlink(handle.name) if os.path.exists(handle.name) else None)


@when("make eval runs in team-ai with no judge configured and no judge cases")
def run_plain(world: World) -> None:
    _x(world)["apr_eval"] = _run_eval(None)


@when("make eval runs with that eval set and JUDGE_CHAT_MODEL empty")
def run_with_judge_case(world: World) -> None:
    _x(world)["apr_eval"] = _run_eval(_x(world)["apr_extra"])


@then(
    "it exits zero and its report lists the primary-429, all-fail and after-first-chunk cases as passed"
)
def eval_passes(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["apr_eval"]
    output = proc.stdout + "\n" + proc.stderr
    assert proc.returncode == 0, f"exit {proc.returncode}:\n{output[-2000:]}"
    report = _report(proc)
    assert report["pass_rate"] == 1.0 and not report.get("failures"), report
    missing = [n for n, pattern in CASES.items() if not re.search(pattern, output, re.I)]
    assert not missing, f"the report does not list the case(s) {missing}:\n{output[-2000:]}"


@then("it exits non-zero, names that case, and reports a pass rate below 100%")
def eval_fails(world: World) -> None:
    proc: subprocess.CompletedProcess[str] = _x(world)["apr_eval"]
    output = proc.stdout + "\n" + proc.stderr
    assert proc.returncode != 0, f"the gate passed with an unscorable judge case:\n{output[-2000:]}"
    assert JUDGE_CASE_ID in output, f"the unscored case is not named:\n{output[-2000:]}"
    assert _report(proc)["pass_rate"] < 1.0, output[-2000:]
