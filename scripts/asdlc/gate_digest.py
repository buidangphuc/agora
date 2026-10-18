#!/usr/bin/env python3
"""Run a test / coverage / gate command and print a compact JSON verdict.

Agents read the JSON, not the raw log. The full log is written to a file
(`log` field); open it only at the lines a failure points to.

  gate_digest.py go <moddir> [--vet] [-- <go test args>]     go test -json ./... -count=1
  gate_digest.py pytest <cwd> -- <pytest args>               junit-xml summary
  gate_digest.py vitest <frontend dir> [-- <files>]          vitest json reporter
  gate_digest.py tsc <frontend dir>
  gate_digest.py biome <frontend dir> [--base-count N]       error count, new vs base
  gate_digest.py spec-sync <worktree> <change>               spec_sync --strict
  gate_digest.py features-check <worktree>
  gate_digest.py audit <worktree> <initiative> <packet> <base>
  gate_digest.py red-first <worktree> <base> [--script PATH]
  gate_digest.py task-gate <worktree> <initiative> <packet> <base> -- <test command>
  gate_digest.py run [--cwd DIR] -- <command...>             any command: exit + tail

Exit: 0 pass, 1 fail, 2 tooling error. Env: GATE_LOG_DIR (default $TMPDIR/gate-digest),
GATE_PYTHON (default platform-e2e/.venv/bin/python of the repo root).
Known non-defects are encoded here, not left to reviewers: packet edits made only by
`chore(plans): widen` / `Revert` commits are not write-set violations; biome errors are
compared with the base count; skipped tests are counted and listed, never treated as passed.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MAX_ITEMS = 10


def log_path(check: str) -> Path:
    d = Path(os.environ.get("GATE_LOG_DIR", Path(tempfile.gettempdir()) / "gate-digest"))
    d.mkdir(parents=True, exist_ok=True)
    return d / f"{time.strftime('%Y%m%d-%H%M%S')}-{os.getpid()}-{check}.log"


def run(cmd, cwd=None, check="run", shell=False):
    lp = log_path(check)
    with open(lp, "w") as fh:
        p = subprocess.run(cmd, cwd=cwd, stdout=fh, stderr=subprocess.STDOUT, text=True, shell=shell)
    return p.returncode, lp.read_text(errors="replace"), str(lp)


def tail(text, n=8):
    return [l for l in text.strip().splitlines()[-n:]]


def emit(res: dict) -> int:
    print(json.dumps(res, ensure_ascii=False, indent=1))
    return {"pass": 0, "fail": 1}.get(res.get("verdict"), 2)


def py() -> str:
    return os.environ.get("GATE_PYTHON", str(ROOT / "platform-e2e/.venv/bin/python"))


# ---------------------------------------------------------------- go
def go(a) -> dict:
    out = {"check": "go", "dir": a.dir}
    if a.vet:
        rc, txt, lp = run(["go", "vet", "./..."], cwd=a.dir, check="go-vet")
        out["vet"] = "pass" if rc == 0 else "fail"
        if rc:
            out["vet_errors"] = tail(txt, MAX_ITEMS)
    rc, txt, lp = run(["go", "test", "-json", "-count=1", *(a.rest or ["./..."])], cwd=a.dir, check="go-test")
    pkgs_ok, pkgs_fail, failed, skipped, build = set(), set(), [], [], []
    for line in txt.splitlines():
        try:
            e = json.loads(line)
        except ValueError:
            if line.strip() and not line.startswith(("ok", "FAIL", "?")):
                build.append(line.strip())
            continue
        act, pkg, test = e.get("Action"), e.get("Package", ""), e.get("Test")
        short = pkg.rsplit("/", 1)[-1]
        if act == "fail":
            (failed.append(f"{short}.{test}") if test else pkgs_fail.add(short))
        elif act == "pass" and not test:
            pkgs_ok.add(short)
        elif act == "skip" and test:
            skipped.append(f"{short}.{test}")
    out.update(log=lp, exit=rc, packages_ok=len(pkgs_ok), packages_failed=sorted(pkgs_fail)[:MAX_ITEMS],
               failed_tests=failed[:MAX_ITEMS], failed_total=len(failed),
               skipped_total=len(skipped), skipped_sample=skipped[:5])
    if build and rc:
        out["build_errors"] = build[:MAX_ITEMS]
    out["verdict"] = "pass" if rc == 0 and out.get("vet", "pass") == "pass" else "fail"
    if skipped:
        out["note"] = "skipped tests are NOT passes (e.g. Postgres tests without TEST_DATABASE_URL)"
    return out


# ---------------------------------------------------------------- pytest
def pytest(a) -> dict:
    junit = log_path("pytest-junit").with_suffix(".xml")
    rc, txt, lp = run([py(), "-m", "pytest", "-q", "-p", "no:cacheprovider", f"--junitxml={junit}", *a.rest],
                      cwd=a.dir, check="pytest")
    out = {"check": "pytest", "log": lp, "exit": rc}
    if not a.rest:
        return {**out, "verdict": "error", "error": "no test paths given (would run the whole suite)"}
    try:
        root = ET.parse(junit).getroot()
        suites = [root] if root.tag == "testsuite" else list(root)
        tot = {k: sum(int(s.get(k, 0)) for s in suites) for k in ("tests", "failures", "errors", "skipped")}
        fails = []
        for tc in root.iter("testcase"):
            for kind in ("failure", "error"):
                el = tc.find(kind)
                if el is not None:
                    msg = (el.get("message") or "").splitlines()[:1]
                    fails.append(f"{tc.get('classname')}::{tc.get('name')}: {msg[0][:200] if msg else kind}")
        out.update(passed=tot["tests"] - tot["failures"] - tot["errors"] - tot["skipped"],
                   failed=tot["failures"], errors=tot["errors"], skipped=tot["skipped"], failures=fails[:MAX_ITEMS])
    except (ET.ParseError, FileNotFoundError):
        out.update(error="no junit output", tail=tail(txt))
        return {**out, "verdict": "error"}
    out["verdict"] = "pass" if rc == 0 else "fail"
    return out


# ---------------------------------------------------------------- frontend
def vitest(a) -> dict:
    rep = log_path("vitest-json").with_suffix(".json")
    rc, txt, lp = run(["npx", "--no-install", "vitest", "run", "--reporter=json", f"--outputFile={rep}", *a.rest],
                      cwd=a.dir, check="vitest")
    out = {"check": "vitest", "log": lp, "exit": rc}
    try:
        r = json.loads(rep.read_text())
        fails = [f"{Path(t['name']).name} > {x.get('fullName') or x.get('title')}"
                 for t in r.get("testResults", []) for x in t.get("assertionResults", []) if x.get("status") == "failed"]
        bad_files = [Path(t["name"]).name for t in r.get("testResults", []) if t.get("status") == "failed" and not t.get("assertionResults")]
        out.update(files=r.get("numTotalTestSuites"), passed=r.get("numPassedTests"), failed=r.get("numFailedTests"),
                   skipped=r.get("numPendingTests", 0) + r.get("numTodoTests", 0), failures=fails[:MAX_ITEMS],
                   failed_to_load=bad_files[:MAX_ITEMS])
    except (ValueError, FileNotFoundError):
        return {**out, "verdict": "error", "tail": tail(txt)}
    out["verdict"] = "pass" if rc == 0 else "fail"
    return out


def tsc(a) -> dict:
    rc, txt, lp = run(["npx", "--no-install", "tsc", "--noEmit"], cwd=a.dir, check="tsc")
    errs = [l for l in txt.splitlines() if "error TS" in l]
    return {"check": "tsc", "log": lp, "exit": rc, "errors": len(errs), "first": errs[:MAX_ITEMS],
            "verdict": "pass" if rc == 0 else "fail"}


def biome(a) -> dict:
    rc, txt, lp = run(["npx", "--no-install", "biome", "check", "."], cwd=a.dir, check="biome")
    m = re.search(r"Found (\d+) errors?", txt)
    n = int(m.group(1)) if m else (0 if rc == 0 else -1)
    files = sorted({mm.group(1) for mm in re.finditer(r"^(\S+\.(?:tsx?|jsx?|json|css)):\d+:\d+", txt, re.M)})
    out = {"check": "biome", "log": lp, "exit": rc, "errors": n, "files_with_errors": files[:MAX_ITEMS]}
    if a.base_count is not None:
        out["base_errors"] = a.base_count
        out["new_errors"] = n - a.base_count
        out["verdict"] = "pass" if 0 <= n <= a.base_count else "fail"
    else:
        out["verdict"] = "pass" if rc == 0 else "fail"
        out["note"] = "pass --base-count N (count at the slice base) to compare instead of demanding 0"
    return out


# ---------------------------------------------------------------- asdlc gates
def spec_sync(a) -> dict:
    rc, txt, lp = run([py(), "platform-e2e/scripts/spec_sync.py", a.change, "--strict"], cwd=a.wt, check="spec-sync")
    unc = [l.strip()[2:] for l in txt.splitlines() if l.strip().startswith("✗ [")]
    issues = [l.strip()[2:] for l in txt.splitlines() if l.strip().startswith("✗ ") and not l.strip().startswith("✗ [")]
    m = re.search(r"(\d+)/(\d+) scenarios have an automated e2e", txt)
    u = re.search(r"(\d+) unit-verified", txt)
    return {"check": "spec-sync", "change": a.change, "log": lp, "exit": rc,
            "automated": int(m.group(1)) if m else None, "total": int(m.group(2)) if m else None,
            "unit_verified": int(u.group(1)) if u else 0, "uncovered": unc[:MAX_ITEMS], "uncovered_total": len(unc),
            "cross_check_issues": issues[:MAX_ITEMS], "verdict": "pass" if rc == 0 else "fail"}


def features_check(a) -> dict:
    venv = str(Path(py()).parent) + "/"
    rc, txt, lp = run(["make", "-C", "platform-e2e", "features-check", f"VENV={venv}"], cwd=a.wt, check="features-check")
    errs = [l.strip() for l in txt.splitlines() if re.search(r"error|✗|❌|invalid|does not resolve", l, re.I)]
    return {"check": "features-check", "log": lp, "exit": rc, "errors": errs[:MAX_ITEMS],
            "verdict": "pass" if rc == 0 else "fail"}


ORCH = re.compile(r'^(chore\(plans\): widen|Revert "chore\(plans\): widen|chore\([^)]*\): revert plan edit)')


def _own_paths(wt, base) -> dict:
    """path -> subjects of the slice's own (first-parent, non-merge) commits in base..HEAD."""
    txt = subprocess.run(["git", "log", "--first-parent", "--no-merges", "--format=@@%s", "--name-only",
                          f"{base}..HEAD"], cwd=wt, capture_output=True, text=True).stdout
    own, subj = {}, ""
    for l in txt.splitlines():
        if l.startswith("@@"):
            subj = l[2:]
        elif l.strip():
            own.setdefault(l.strip(), []).append(subj)
    return own


def audit(a) -> dict:
    rc, txt, lp = run([py(), "scripts/validate_plan.py", f"plans/{a.initiative}", "--audit", a.packet, "--base", a.base],
                      cwd=a.wt, check="audit")
    outside = [l.strip()[2:] for l in txt.splitlines() if l.strip().startswith("- ")]
    own = _own_paths(a.wt, a.base)
    via_merge = [p for p in outside if p not in own]                      # brought in by merging the integration branch
    def orch_only(subjects):  # newest first; worker edits older than a "revert plan edit" commit are undone
        rev = next((i for i, s in enumerate(subjects) if "revert plan edit" in s), len(subjects))
        return all(ORCH.match(s) for s in subjects[:rev])
    orch = [p for p in outside if p in own and orch_only(own[p])]  # packet widen / reverted plan edits
    real = [p for p in outside if p not in via_merge and p not in orch]
    ok = rc == 0 or not real
    return {"check": "audit", "log": lp, "exit": rc, "outside": real[:MAX_ITEMS], "outside_total": len(real),
            "excused_orchestrator_edits": orch, "excused_via_merge": len(via_merge), "verdict": "pass" if ok else "fail"}


def red_first(a) -> dict:
    script = a.script or str(ROOT / "scripts/asdlc/red_first.sh")
    env_path = f"{ROOT}/team-frontend/node_modules/.bin:{Path(py()).parent}:{os.environ.get('PATH', '')}"
    cmd = f"PATH='{env_path}' REDFIRST_PYTHON='{py()}' bash '{script}' --base {a.base} --head HEAD"
    rc, txt, lp = run(cmd, cwd=a.wt, check="red-first", shell=True)
    rows = [l for l in txt.splitlines() if re.search(r"\s(OK|EXEMPT|FAIL|SKIP)\b", l) and not l.startswith("red_first:")]
    fails = [re.sub(r"\s+", " ", l).strip() for l in rows if "FAIL" in l]
    green_at_base = [l.split()[0] for l in fails if "green at base" in l]
    red_at_head = [l.split()[0] for l in fails if "red at head" in l]
    tooling = [l for l in txt.splitlines() if "WARN buf generate failed" in l or "WARN buf not installed" in l]
    head = [l for l in txt.splitlines() if l.startswith("red_first: FAIL:")]
    out = {"check": "red-first", "log": lp, "exit": rc, "files": len(rows),
           "ok": sum(1 for l in rows if " OK" in l), "exempt": sum(1 for l in rows if "EXEMPT" in l),
           "green_at_base": green_at_base[:MAX_ITEMS], "red_at_head": red_at_head[:MAX_ITEMS],
           "precondition_fail": head[:3], "tooling_warnings": tooling[:3]}
    if rc == 0:
        out["verdict"] = "pass"
    elif tooling and red_at_head and not green_at_base:
        out["verdict"] = "error"
        out["hint"] = "red at head with buf/codegen warnings: tooling, reproduce by hand (README gen.sh)"
    else:
        out["verdict"] = "fail"
        if green_at_base:
            out["hint"] = "green at base: verification-only task? then Red-First-Exempt trailer per exact path"
    return out


def task_gate(a) -> dict:
    parts = {
        "red_first": red_first(argparse.Namespace(wt=a.wt, base=a.base, script=a.script)),
        "audit": audit(a),
        "tests": generic(argparse.Namespace(cwd=a.wt, rest=a.rest, check="task-tests")) if a.rest else
                 {"verdict": "error", "error": "no test command after --"},
    }
    verdicts = {k: v["verdict"] for k, v in parts.items()}
    v = "pass" if all(x == "pass" for x in verdicts.values()) else ("error" if "error" in verdicts.values() else "fail")
    return {"check": "task-gate", "verdict": v, "summary": verdicts, **parts}


def generic(a) -> dict:
    cmd = a.rest[0] if len(a.rest) == 1 else a.rest
    rc, txt, lp = run(cmd, cwd=a.cwd, check=getattr(a, "check", "run"), shell=isinstance(cmd, str))
    return {"check": getattr(a, "check", "run"), "log": lp, "exit": rc, "tail": tail(txt),
            "verdict": "pass" if rc == 0 else "fail"}


def main(argv=None) -> int:
    argv = list(sys.argv[1:] if argv is None else argv)
    rest = []
    if "--" in argv:
        i = argv.index("--")
        argv, rest = argv[:i], argv[i + 1:]
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sp = ap.add_subparsers(dest="cmd", required=True)
    p = sp.add_parser("go"); p.add_argument("dir"); p.add_argument("--vet", action="store_true")
    for n in ("pytest", "vitest", "tsc"):
        sp.add_parser(n).add_argument("dir")
    p = sp.add_parser("biome"); p.add_argument("dir"); p.add_argument("--base-count", type=int)
    p = sp.add_parser("spec-sync"); p.add_argument("wt"); p.add_argument("change")
    sp.add_parser("features-check").add_argument("wt")
    for n in ("audit", "task-gate"):
        p = sp.add_parser(n)
        for x in ("wt", "initiative", "packet", "base"):
            p.add_argument(x)
        p.add_argument("--script")
    p = sp.add_parser("red-first"); p.add_argument("wt"); p.add_argument("base"); p.add_argument("--script")
    p = sp.add_parser("run"); p.add_argument("--cwd")
    a = ap.parse_args(argv)
    a.rest = rest
    fn = {"go": go, "pytest": pytest, "vitest": vitest, "tsc": tsc, "biome": biome, "spec-sync": spec_sync,
          "features-check": features_check, "audit": audit, "red-first": red_first, "task-gate": task_gate,
          "run": generic}[a.cmd]
    try:
        return emit(fn(a))
    except (OSError, subprocess.SubprocessError) as e:
        return emit({"check": a.cmd, "verdict": "error", "error": str(e)})


if __name__ == "__main__":
    sys.exit(main())
