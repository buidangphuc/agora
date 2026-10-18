import argparse
import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import gate_digest as g  # noqa: E402


@pytest.fixture(autouse=True)
def _logdir(tmp_path, monkeypatch):
    monkeypatch.setenv("GATE_LOG_DIR", str(tmp_path))


def fake_run(text, rc=0):
    def _run(cmd, cwd=None, check="run", shell=False):
        return rc, text, f"/tmp/{check}.log"
    return _run


# ------------------------------------------------------------- ORCH regex
@pytest.mark.parametrize("subject", [
    "chore(plans): widen packet t3 for proto stubs",
    'Revert "chore(plans): widen packet t3"',
    "chore(asdlc): revert plan edit",
])
def test_orch_matches_orchestrator_subjects(subject):
    assert g.ORCH.match(subject)


@pytest.mark.parametrize("subject", [
    "feat(order): add hold",
    "fix: chore(plans): widen",          # not anchored at start
    "chore(plans): narrow packet",
])
def test_orch_rejects_worker_subjects(subject):
    assert not g.ORCH.match(subject)


# ------------------------------------------------------------- audit
AUDIT_OUT = "outside the write-set:\n  - a.go\n  - b.go\n  - c.go\n  - d.go\n"


def run_audit(monkeypatch, own, rc=1):
    monkeypatch.setattr(g, "run", fake_run(AUDIT_OUT, rc))
    monkeypatch.setattr(g, "_own_paths", lambda wt, base: own)
    return g.audit(argparse.Namespace(wt=".", initiative="i", packet="p", base="b"))


def test_audit_excuses_merge_and_orchestrator_edits(monkeypatch):
    res = run_audit(monkeypatch, {
        "a.go": ["chore(plans): widen packet"],                       # orchestrator only
        "b.go": ["feat: worker edit"],                                # real violation
        # c.go absent from own commits -> came in via merge
        "d.go": ["chore(x): revert plan edit", "feat: worker edit"],  # worker edit undone by revert (newest first)
    })
    assert res["outside"] == ["b.go"]
    assert res["outside_total"] == 1
    assert res["excused_orchestrator_edits"] == ["a.go", "d.go"]
    assert res["excused_via_merge"] == 1
    assert res["verdict"] == "fail"


def test_audit_passes_when_only_excused_paths(monkeypatch):
    res = run_audit(monkeypatch, {"a.go": ["chore(plans): widen x"]})
    assert res["outside"] == [] and res["verdict"] == "pass"


def test_audit_worker_edit_after_revert_is_real(monkeypatch):
    res = run_audit(monkeypatch, {"a.go": ["feat: later worker edit", "chore(x): revert plan edit"]})
    assert "a.go" in res["outside"]


# ------------------------------------------------------------- verdict shape / exit codes
@pytest.mark.parametrize("verdict,code", [("pass", 0), ("fail", 1), ("error", 2), (None, 2)])
def test_emit_exit_code_and_json(capsys, verdict, code):
    assert g.emit({"check": "x", "verdict": verdict}) == code
    assert json.loads(capsys.readouterr().out) == {"check": "x", "verdict": verdict}


def test_tail_last_lines():
    assert g.tail("a\nb\nc\n", 2) == ["b", "c"]


def test_go_parses_test_json(monkeypatch):
    lines = [
        {"Action": "pass", "Package": "m/internal/a"},
        {"Action": "fail", "Package": "m/internal/b", "Test": "TestX"},
        {"Action": "fail", "Package": "m/internal/b"},
        {"Action": "skip", "Package": "m/internal/c", "Test": "TestDB"},
    ]
    monkeypatch.setattr(g, "run", fake_run("\n".join(json.dumps(l) for l in lines), rc=1))
    res = g.go(argparse.Namespace(dir=".", vet=False, rest=[]))
    assert res["verdict"] == "fail"
    assert res["failed_tests"] == ["b.TestX"] and res["packages_failed"] == ["b"]
    assert res["packages_ok"] == 1
    assert res["skipped_total"] == 1 and "note" in res
    assert {"check", "log", "exit", "verdict"} <= res.keys()


@pytest.mark.parametrize("found,base,verdict", [(3, 3, "pass"), (2, 3, "pass"), (4, 3, "fail")])
def test_biome_compares_with_base_count(monkeypatch, found, base, verdict):
    monkeypatch.setattr(g, "run", fake_run(f"src/a.ts:1:2 lint\nFound {found} errors.", rc=1))
    res = g.biome(argparse.Namespace(dir=".", base_count=base))
    assert res["verdict"] == verdict
    assert res["new_errors"] == found - base
    assert res["files_with_errors"] == ["src/a.ts"]


def test_pytest_without_paths_is_error(monkeypatch):
    monkeypatch.setattr(g, "run", fake_run("", rc=0))
    res = g.pytest(argparse.Namespace(dir=".", rest=[]))
    assert res["verdict"] == "error"
