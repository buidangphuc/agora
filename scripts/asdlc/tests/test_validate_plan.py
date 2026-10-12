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
