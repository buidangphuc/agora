"""Tests for the tasks.md <-> FEATURES.yaml <-> .feature cross-check in spec_sync.py."""

import sys
import textwrap
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import spec_sync  # noqa: E402

SPEC = """# cap
#### Scenario: Buyer pays
- **WHEN** x
"""


def build(tmp_path, monkeypatch, tasks, features_yaml, feature_files):
    poly = tmp_path / "poly"
    spec = poly / "openspec/changes/chg/specs/pay"
    spec.mkdir(parents=True)
    (spec / "spec.md").write_text(SPEC)
    (poly / "openspec/changes/chg/tasks.md").write_text(textwrap.dedent(tasks))
    (poly / "team-x").mkdir()
    (poly / "team-x/FEATURES.yaml").write_text(textwrap.dedent(features_yaml))
    fdir = poly / "platform-e2e/tests/e2e/features"
    fdir.mkdir(parents=True)
    for name, body in feature_files.items():
        (fdir / name).write_text(body)
    monkeypatch.setattr(spec_sync, "POLYREPO", poly)
    monkeypatch.setattr(spec_sync, "OPENSPEC", poly / "openspec")
    monkeypatch.setattr(spec_sync, "FEATURES_DIR", fdir)
    issues, build.warnings = spec_sync.cross_check("chg", spec_sync.change_scenarios("chg"))
    return issues


build.warnings = []


FEAT_OK = """
repo: team-x
features:
  - {id: x.pay, title: Buyer pays, persona: buyer, entry_route: /, acceptance: [a],
     status: automated, covered_by: "pay.feature::Buyer pays"}
"""
FILE_OK = {"pay.feature": "Feature: f\n  Scenario: Buyer pays\n    Given x\n"}
TASKS_OK = "- [ ] [e2e] cover it capability: pay scenario: `Buyer pays`\n- [ ] code thing\n"


def test_clean(tmp_path, monkeypatch):
    assert build(tmp_path, monkeypatch, TASKS_OK, FEAT_OK, FILE_OK) == []


def test_unknown_capability_and_scenario(tmp_path, monkeypatch):
    t = '- [ ] [e2e] x capability: nope scenario: "Ghost"\n'
    issues = build(tmp_path, monkeypatch, t, FEAT_OK, FILE_OK)
    assert any("unknown capability 'nope'" in i for i in issues)
    assert any("unknown scenario 'Ghost'" in i for i in issues)


def test_e2e_task_without_reference(tmp_path, monkeypatch):
    issues = build(tmp_path, monkeypatch, "- [ ] [e2e] vague\n", FEAT_OK, FILE_OK)
    assert any("no capability:/scenario:" in i for i in issues)


def test_automated_without_scenario_is_contradiction(tmp_path, monkeypatch):
    issues = build(tmp_path, monkeypatch, TASKS_OK, FEAT_OK, {"pay.feature": "Feature: f\n"})
    assert any("automated but no matching .feature" in i for i in issues)


def test_planned_acceptance_without_scenario(tmp_path, monkeypatch):
    feat = FEAT_OK.replace(
        'status: automated, covered_by: "pay.feature::Buyer pays"', "status: planned"
    )
    issues = build(tmp_path, monkeypatch, TASKS_OK, feat, {})
    assert any("acceptance lines but no .feature scenario" in i for i in issues)


def test_duplicate_pinned_by_covered_by_is_warning(tmp_path, monkeypatch):
    files = dict(FILE_OK, **{"other.feature": "Feature: g\n  Scenario: Buyer pays\n"})
    issues = build(tmp_path, monkeypatch, TASKS_OK, FEAT_OK, files)
    assert issues == []
    assert any(w.startswith("duplicate scenario 'Buyer pays'") for w in build.warnings)


def test_duplicate_ambiguous_is_issue(tmp_path, monkeypatch):
    feat = FEAT_OK.replace(
        'status: automated, covered_by: "pay.feature::Buyer pays"', "status: not-testable"
    )
    files = {
        "a.feature": "Feature: f\n  Scenario: Buyer pays\n",
        "b.feature": "Feature: g\n  Scenario: Buyer pays\n",
    }
    issues = build(tmp_path, monkeypatch, TASKS_OK, feat, files)
    assert any(i.startswith("duplicate scenario 'Buyer pays'") for i in issues)


def test_untied_duplicate_is_warning(tmp_path, monkeypatch):
    files = {
        "a.feature": "Feature: f\n  Scenario: Buyer pays\n",
        "b.feature": "Feature: g\n  Scenario: Buyer pays\n",
    }
    issues = build(tmp_path, monkeypatch, TASKS_OK, "repo: team-x\nfeatures: []\n", files)
    assert issues == [] and build.warnings


def test_features_tie_is_equality_not_substring(tmp_path, monkeypatch):
    feat = FEAT_OK.replace("title: Buyer pays", "title: Buyer pays twice").replace(
        'status: automated, covered_by: "pay.feature::Buyer pays"', "status: planned"
    )
    assert build(tmp_path, monkeypatch, TASKS_OK, feat, {}) == []


def test_strict_exit_code(tmp_path, monkeypatch):
    build(tmp_path, monkeypatch, "- [ ] [e2e] vague\n", FEAT_OK, FILE_OK)
    monkeypatch.setattr(sys, "argv", ["spec_sync.py", "chg", "--strict"])
    assert spec_sync.main() == 1
    (spec_sync.OPENSPEC / "changes/chg/tasks.md").write_text(TASKS_OK)
    assert spec_sync.main() == 0
