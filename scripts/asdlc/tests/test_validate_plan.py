"""validate_plan.py: task packets are required only once the initiative LOCK is approved."""
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
_spec = importlib.util.spec_from_file_location("validate_plan", ROOT / "scripts" / "validate_plan.py")
validate_plan = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(validate_plan)


def _plan(tmp_path: Path, lock_status: str | None) -> Path:
    (tmp_path / "00-spec.md").write_text("# spec\n\n## Locked\n2026-10-12 — ok\n", encoding="utf-8")
    if lock_status is not None:
        (tmp_path / "LOCK.md").write_text(
            f"# LOCK — x\nstatus: {lock_status}            # human sets: approved | rejected\n", encoding="utf-8")
    return tmp_path


def test_pending_lock_without_packets_passes(tmp_path, capsys):
    assert validate_plan.validate(_plan(tmp_path, "pending")) == 0
    assert "LOCK" in capsys.readouterr().out


def test_approved_lock_without_packets_fails(tmp_path, capsys):
    assert validate_plan.validate(_plan(tmp_path, "approved")) == 1
    assert "no task packets found" in capsys.readouterr().out


def test_no_lock_without_packets_fails(tmp_path, capsys):
    assert validate_plan.validate(_plan(tmp_path, None)) == 1
    assert "no task packets found" in capsys.readouterr().out


def _packet(plan: Path, name: str, write_set: str) -> Path:
    path = plan / name
    path.write_text(f"# packet\n\n## Write-set\n{write_set}\n## Verify\n- `true`\n", encoding="utf-8")
    return path


def test_widened_entries_parse_as_paths(tmp_path):
    packet = _packet(tmp_path, "wave1-task1-a.md", "\n".join([
        "- team-a/**",
        "- `team-a/go.mod` (owned)",
        "- team-a/README.md (widened at build, P1: docs-sync (rows))",
        "- widened (P1):",
        "  - team-a/x.go — reason with a/slash and (parens)",
        "- widened (P1)",
        "  - team-a/y.go — reason",
        "Widened (user-approved 2026-11-15):",
        "  - team-a/z.go — reason",
    ]))
    assert validate_plan.parse_packet(packet)["writes"] == [
        "team-a/**", "team-a/go.mod", "team-a/README.md", "team-a/x.go", "team-a/y.go", "team-a/z.go"]


def test_widened_header_does_not_cause_false_conflicts(tmp_path, capsys):
    _plan(tmp_path, "approved")
    _packet(tmp_path, "wave1-task1-a.md", "- team-a/**\n- widened (P1):\n  - team-a/x.go — reason\n")
    _packet(tmp_path, "wave1-task2-b.md", "- team-b/**\n- widened (P1):\n  - team-b/x.go — reason\n")
    assert validate_plan.validate(tmp_path) == 0
    assert "CONFLICT" not in capsys.readouterr().out


def test_same_wave_shared_pyproject_is_a_warning(tmp_path, capsys):
    _plan(tmp_path, "approved")
    _packet(tmp_path, "wave1-task1-a.md", "- team-a/**\n- platform-e2e/pyproject.toml (widened at build, P1: markers)\n")
    _packet(tmp_path, "wave1-task2-b.md", "- team-b/**\n- platform-e2e/pyproject.toml\n")
    assert validate_plan.validate(tmp_path) == 0
    out = capsys.readouterr().out
    assert "pytest-marker-only" in out and "CONFLICT" not in out


def test_same_wave_other_shared_file_still_conflicts(tmp_path, capsys):
    _plan(tmp_path, "approved")
    _packet(tmp_path, "wave1-task1-a.md", "- docker-compose.services.yaml\n")
    _packet(tmp_path, "wave1-task2-b.md", "- docker-compose.services.yaml\n")
    assert validate_plan.validate(tmp_path) == 1
    assert "CONFLICT" in capsys.readouterr().out


PYPROJECT = '[project]\nname = "e2e"\n\n[tool.pytest.ini_options]\naddopts = "-ra"\nmarkers = [\n    "smoke: fast",\n]\n'


def test_marker_addition_is_marker_only():
    new = PYPROJECT.replace('"smoke: fast",', '"smoke: fast",\n    "kyc: kyc change",')
    assert validate_plan.pytest_marker_only_change(PYPROJECT, new)


def test_non_marker_edits_are_rejected():
    assert not validate_plan.pytest_marker_only_change(PYPROJECT, PYPROJECT.replace('"-ra"', '"-ra -x"'))
    assert not validate_plan.pytest_marker_only_change(PYPROJECT, PYPROJECT.replace('"smoke: fast",', ""))
    assert not validate_plan.pytest_marker_only_change(PYPROJECT, PYPROJECT + "[broken\n")


def test_audit_tolerates_marker_only_shared_pyproject(tmp_path, monkeypatch, capsys):
    import subprocess
    repo = tmp_path / "repo"
    (repo / "platform-e2e").mkdir(parents=True)
    (repo / "platform-e2e" / "pyproject.toml").write_text(PYPROJECT, encoding="utf-8")
    git = ["git", "-C", str(repo), "-c", "user.email=t@t", "-c", "user.name=t"]
    subprocess.run(["git", "init", "-q", "-b", "main", str(repo)], check=True)
    subprocess.run(git + ["add", "."], check=True)
    subprocess.run(git + ["commit", "-qm", "base"], check=True)
    plan = repo / "plans" / "x"
    plan.mkdir(parents=True)
    a = _packet(plan, "wave1-task1-a.md", "- team-a/**\n- platform-e2e/pyproject.toml\n")
    _packet(plan, "wave1-task2-b.md", "- team-b/**\n- platform-e2e/pyproject.toml\n")
    monkeypatch.chdir(repo)
    pyproject = repo / "platform-e2e" / "pyproject.toml"
    pyproject.write_text(PYPROJECT.replace('"smoke: fast",', '"smoke: fast",\n    "a: a",'), encoding="utf-8")
    assert validate_plan.audit(a, "main") == 0
    assert "tolerated" in capsys.readouterr().out
    pyproject.write_text(PYPROJECT.replace('"-ra"', '"-ra -x"'), encoding="utf-8")
    assert validate_plan.audit(a, "main") == 1
    assert "only pytest marker additions" in capsys.readouterr().out
