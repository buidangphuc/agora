"""One mutation of the valid fixture per violation type (copied to tmp_path)."""
import shutil
import sys
from pathlib import Path

import pytest
import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import cross_change_check as c  # noqa: E402

VALID = Path(__file__).parent / "fixtures" / "cross_change" / "valid"


@pytest.fixture
def tree(tmp_path):
    dst = tmp_path / "t"
    shutil.copytree(VALID, dst)
    return dst


def run(root):
    return c.check(root / "plans/x/slices.yaml", root / "openspec/changes", root / "proto", root)


def rules(root):
    return {r for r, _ in run(root)[0]}


def edit_slices(root, fn):
    p = root / "plans/x/slices.yaml"
    d = yaml.safe_load(p.read_text())
    fn({s["id"]: s for s in d["slices"]})
    p.write_text(yaml.safe_dump(d))


def test_valid(tree):
    v, order = run(tree)
    assert v == []
    assert order == ["payout-contract", "payout-holdback", "payout-seller-ui"]
    assert c.main(["--slices", str(tree / "plans/x/slices.yaml"), "--root", str(tree), "--proto-dir", str(tree / "proto")]) == 0


def test_cli_exit_1_and_json(tree, capsys):
    (tree / "openspec/changes/payout-holdback/proposal.md").write_text("# P\n")
    assert c.main(["--slices", str(tree / "plans/x/slices.yaml"), "--root", str(tree), "--proto-dir", str(tree / "proto"), "--json"]) == 1
    assert '"rule": "schema"' in capsys.readouterr().out


def test_schema_missing_initiative_line(tree):
    (tree / "openspec/changes/payout-holdback/proposal.md").write_text("# P\n")
    assert rules(tree) == {"schema"}


def test_schema_wrong_initiative(tree):
    (tree / "openspec/changes/payout-holdback/proposal.md").write_text("initiative: other\n")
    assert rules(tree) == {"schema"}


def test_schema_missing_change_dir(tree):
    shutil.rmtree(tree / "openspec/changes/payout-seller-ui")
    assert "schema" in rules(tree)


def test_schema_missing_owner(tree):
    edit_slices(tree, lambda s: s["payout-holdback"].pop("owner"))
    assert "schema" in rules(tree)


def test_dag_unknown_dep(tree):
    edit_slices(tree, lambda s: s["payout-holdback"].update(depends_on=["ghost"]))
    assert "dag" in rules(tree)


def test_dag_cycle(tree):
    edit_slices(tree, lambda s: s["payout-contract"].update(depends_on=["payout-seller-ui"]))
    v, order = run(tree)
    assert any(r == "dag" and "cycle" in m for r, m in v)
    assert order == []


def test_contract_unknown_rpc(tree):
    edit_slices(tree, lambda s: s["payout-holdback"]["uses"].append("PaymentService.Nope"))
    v, _ = run(tree)
    assert [r for r, m in v] == ["contract"] and "Nope" in v[0][1]


def test_contract_unknown_event(tree):
    edit_slices(tree, lambda s: s["payout-holdback"]["uses"].append("payment.events/Ghost"))
    assert rules(tree) == {"contract"}


def test_contract_not_a_dependency(tree):
    # HoldPayout only exists in payout-contract's delta; drop the dependency on it
    edit_slices(tree, lambda s: s["payout-holdback"].update(depends_on=[]))
    assert rules(tree) == {"contract"}


def test_contract_provides_counts(tree):
    (tree / "openspec/changes/payout-contract/tasks.md").write_text("- [ ] nothing\n")
    assert rules(tree) == {"contract"}
    edit_slices(tree, lambda s: s["payout-contract"].update(
        provides=["PaymentService.HoldPayout", "payment.events/PayoutHeld"]))
    assert rules(tree) == set()


def test_ownership_wrong_owner(tree):
    edit_slices(tree, lambda s: s["payout-holdback"].update(owner="team-order"))
    v, _ = run(tree)
    assert [r for r, _ in v] == ["ownership"] and "payout-holdback" in v[0][1]


def test_ownership_scenario_in_two_manifests(tree):
    other = tree / "team-order"
    other.mkdir()
    (other / "FEATURES.yaml").write_text(
        "repo: team-order\nfeatures:\n  - {id: o.x, title: Hold payout on dispute, status: todo}\n")
    (tree / "team-payment/FEATURES.yaml").write_text(
        "repo: team-payment\nfeatures:\n  - {id: p.x, title: 'Hold payout on dispute', capability: payout-holdback}\n")
    v, _ = run(tree)
    assert any(r == "ownership" and "exactly one" in m for r, m in v)


def test_ownership_scenario_in_two_slices(tree):
    spec = tree / "openspec/changes/payout-seller-ui/specs/seller-payout-ui/spec.md"
    spec.write_text(spec.read_text().replace("Seller sees held payout", "Hold payout on dispute"))
    assert "ownership" in rules(tree)


def test_conflict_same_requirement(tree):
    d = tree / "openspec/changes/payout-seller-ui/specs/payout-holdback"
    d.mkdir()
    (d / "spec.md").write_text(
        "## MODIFIED Requirements\n\n### Requirement: Payout holdback\nx\n\n#### Scenario: Other\n- ok\n")
    edit_slices(tree, lambda s: s["payout-seller-ui"].update(owner="team-payment"))
    assert "conflict" in rules(tree)
