import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import spec_lint  # noqa: E402

FX = Path(__file__).parent / "fixtures" / "spec_lint"


def rules(name, legacy=False):
    return [(f.rule, f.severity) for f in spec_lint.lint_change(FX / name, legacy)]


def test_good_change_is_clean():
    assert rules("good") == []
    assert spec_lint.main([str(FX / "good")]) == 0


def test_proposal_sections_error_and_legacy_warning():
    r = rules("bad-proposal")
    assert r.count(("PROPOSAL-sections", "error")) == 3
    assert all(s == "warning" for ru, s in rules("bad-proposal", True) if ru == "PROPOSAL-sections")
    assert spec_lint.main([str(FX / "bad-proposal")]) == 1
    assert spec_lint.main([str(FX / "bad-proposal"), "--legacy"]) == 0


def test_structure_rules():
    r = rules("bad-structure")
    assert ("REQ-scenario", "error") in r
    assert ("EARS-shall", "error") in r
    assert ("SCN-then", "error") in r
    assert ("SCN-when", "error") in r


def test_wording_rules():
    r = rules("bad-wording")
    assert ("THEN-vague", "warning") in r
    assert ("ISO29148-singular", "warning") in r
    msgs = [f.message for f in spec_lint.lint_change(FX / "bad-wording")]
    assert any('"properly"' in m for m in msgs) and any('"should"' in m for m in msgs)


def test_error_and_permission_scenarios_required():
    r = rules("bad-coverage")
    assert ("ERR-scenario", "error") in r
    assert ("PERM-scenario", "warning") in r


def test_warn_only_and_json(capsys):
    assert spec_lint.main([str(FX / "bad-coverage"), "--warn-only", "--json"]) == 0
    data = json.loads(capsys.readouterr().out)
    assert data["errors"] >= 1 and data["findings"][0]["rule"]


def test_unknown_change_exit_2():
    assert spec_lint.main(["no-such-change-xyz"]) == 2
