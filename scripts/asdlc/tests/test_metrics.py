import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import metrics as m  # noqa: E402


def write(p, rows):
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("\n".join(json.dumps(r) for r in rows) + "\n")


def ev(ts, event, **kw):
    return {"ts": f"2026-10-11T{ts}:00+00:00", "change": "c1", "task": "t1", "phase": "build", "event": event, **kw}


def fixtures(tmp_path):
    runs = tmp_path / "runs"
    write(runs / "c1.jsonl", [
        ev("08:00", "round", tokens=100),
        ev("09:00", "gate", gate="task", result="fail", tokens=50),
        ev("10:00", "round", tokens=200),
        ev("11:00", "gate", gate="task", result="pass"),
        ev("12:00", "done"),
    ])
    write(runs / "c2.jsonl", [dict(ev("08:00", "round"), change="c2")])  # unfinished
    kev = tmp_path / "kev.jsonl"
    write(kev, [
        {"tag": "c1/task.v1", "latency_ms": 100, "wall_ms": 150, "exit": 0},
        {"tag": "c1/feature.v1", "latency_ms": 300, "wall_ms": 350, "exit": 0},
        {"tag": "c1/x", "latency_ms": None, "exit": 5},
        {"tag": "other/x", "latency_ms": 9999, "exit": 0},
    ])
    return runs, kev


def test_aggregate_and_kev(tmp_path):
    runs, kev = fixtures(tmp_path)
    data = m.build(m.load_runs(runs), m.load_kev([kev]), None, None)
    c1 = next(r for r in data["runs"] if r["change"] == "c1")
    assert c1["cycle_hours"] == 4.0 and c1["rounds"] == 2 and c1["gate_runs"] == 2
    assert c1["gate_red"] == 1 and c1["gate_red_by_gate"] == {"task": 1} and c1["tokens"] == 350
    assert c1["kev"] == {"calls": 3, "errors": 1, "latency_ms_p50": 200.0, "latency_ms_p95": 290.0, "wall_ms_p50": 250.0}
    c2 = next(r for r in data["runs"] if r["change"] == "c2")
    assert c2["cycle_hours"] is None and not c2["finished"]
    assert data["summary"]["n_runs"] == 2 and data["summary"]["n_finished"] == 1


def test_git_rows_and_baseline(tmp_path):
    runs, kev = fixtures(tmp_path)
    git = {"c1": {"post_merge_fixes": 1, "cycle_hours": 7.0, "fix_window_complete": True},
           "c2": {"post_merge_fixes": 0, "cycle_hours": 20.0, "fix_window_complete": False}}
    base = {"cycle_hours": {"median": 8.0}, "fix_rate_changes_with_fix": 0.09}
    data = m.build(m.load_runs(runs), [], base, git)
    by = {r["change"]: r for r in data["runs"]}
    assert by["c1"]["post_merge_fixes"] == 1 and by["c1"]["cycle_hours"] == 4.0  # run events win
    assert by["c2"]["post_merge_fixes"] is None and by["c2"]["cycle_hours"] == 20.0  # git fallback
    cmp = data["comparison"]
    assert cmp["baseline_cycle_hours_median"] == 8.0 and cmp["cycle_delta_pct"] == round((12.0 - 8) / 8 * 100, 1)
    assert "vs baseline" in m.render_table(data)


def test_torn_line_ignored(tmp_path):
    p = tmp_path / "runs" / "c1.jsonl"
    p.parent.mkdir()
    p.write_text(json.dumps(ev("08:00", "round")) + "\n{broken\n")
    assert len(m.load_runs(p.parent)["c1"]) == 1


def test_no_runs(tmp_path, capsys):
    assert m.main(["--runs-dir", str(tmp_path / "none"), "--out", str(tmp_path / "s.json")]) == 0
    assert "no ASDLC runs yet" in capsys.readouterr().out
    assert not (tmp_path / "s.json").exists()
    assert m.render_table(m.build({}, [], None, None)) == "no ASDLC runs yet"


def test_main_writes_summary(tmp_path, capsys):
    runs, kev = fixtures(tmp_path)
    out = tmp_path / "summary.json"
    assert m.main(["--runs-dir", str(runs), "--kev-log", str(kev), "--baseline", str(tmp_path / "nob.json"),
                   "--out", str(out), "--no-git"]) == 0
    assert json.loads(out.read_text())["summary"]["n_runs"] == 2
    assert "c1" in capsys.readouterr().out


def test_skill_event_aliases(tmp_path):
    runs = tmp_path / "runs"
    write(runs / "c1.jsonl", [ev("08:00", "task_round"), ev("09:00", "gate", gate="e2e", result="red"),
                              ev("10:00", "run_done", rounds_total=4)])
    r = m.build(m.load_runs(runs), [], None, None)["runs"][0]
    assert r["rounds"] == 4 and r["gate_red"] == 1 and r["cycle_hours"] == 2.0
