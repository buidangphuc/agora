#!/usr/bin/env python3
"""ASDLC metrics (W5-1): per-change-run aggregates compared with metrics/baseline.json.

Inputs (all optional; with none of them it prints "no ASDLC runs yet"):
  metrics/runs/<change>.jsonl   run events written by the feature-run skill (format: metrics/README.md)
  Kev log(s)                    $KEV_LOG, ~/tools/kev/log.jsonl, gates/runs/kev.jsonl (rows filtered by
                                tag prefix = change id)
  git history                   post-merge fix commits + archived cycle time (via baseline.py)

Usage:
  python scripts/asdlc/metrics.py [--runs-dir metrics/runs] [--kev-log PATH ...]
                                  [--baseline metrics/baseline.json] [--out metrics/summary.json] [--no-git]
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import baseline as bl  # noqa: E402  (reuse: parse_ts, percentile, collect)

REPO_ROOT = bl.REPO_ROOT
RED_RESULTS = {"fail", "red", "block", "would_block", "veto"}  # gate results that count as "gate red"
# Canonical names (metrics/README.md) plus the aliases the feature-run skill writes today.
ROUND_EVENTS = {"round", "task_round"}
DONE_EVENTS = {"done", "run_done"}


def _read_jsonl(path: Path) -> list:
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError:
            continue  # a torn/partial line must not kill the report
    return rows


def load_runs(runs_dir: Path) -> dict:
    """change -> [event, ...] (sorted by ts). Change id = event['change'] or the file stem."""
    runs: dict = {}
    if not runs_dir.is_dir():
        return runs
    for p in sorted(runs_dir.glob("*.jsonl")):
        for ev in _read_jsonl(p):
            if not isinstance(ev, dict) or "ts" not in ev:
                continue
            runs.setdefault(ev.get("change") or p.stem, []).append(ev)
    for evs in runs.values():
        evs.sort(key=lambda e: bl.parse_ts(e["ts"]))
    return runs


def load_kev(paths) -> list:
    rows = []
    for p in paths:
        p = Path(p).expanduser()
        if p.is_file():
            rows += [r for r in _read_jsonl(p) if isinstance(r, dict)]
    return rows


def kev_stats(rows, change: str) -> dict:
    mine = [r for r in rows if str(r.get("tag") or "").startswith(change)]
    lat = [r["latency_ms"] for r in mine if isinstance(r.get("latency_ms"), (int, float))]
    wall = [r["wall_ms"] for r in mine if isinstance(r.get("wall_ms"), (int, float))]
    r1 = lambda x: None if x is None else round(x, 1)  # noqa: E731
    return {
        "calls": len(mine),
        "errors": sum(1 for r in mine if r.get("exit") not in (0, None)),
        "latency_ms_p50": r1(bl.percentile(lat, 50)),
        "latency_ms_p95": r1(bl.percentile(lat, 95)),
        "wall_ms_p50": r1(bl.percentile(wall, 50)),
    }


def aggregate_run(change: str, events: list) -> dict:
    """One record per change run from its events."""
    tss = [bl.parse_ts(e["ts"]) for e in events]
    start, last = min(tss), max(tss)
    done = [bl.parse_ts(e["ts"]) for e in events if e.get("event") in DONE_EVENTS]
    rounds = [e for e in events if e.get("event") in ROUND_EVENTS]
    declared = max([e["rounds_total"] for e in events if isinstance(e.get("rounds_total"), int)], default=0)
    gates = [e for e in events if e.get("event") == "gate"]
    red = [e for e in gates if str(e.get("result", "")).lower() in RED_RESULTS]
    tokens = sum(e["tokens"] for e in events if isinstance(e.get("tokens"), (int, float)))
    end = max(done) if done else None
    return {
        "change": change,
        "events": len(events),
        "start": start.isoformat(),
        "end": end.isoformat() if end else None,
        "finished": end is not None,
        "cycle_hours": round((end - start).total_seconds() / 3600, 2) if end else None,
        "wall_hours": round((last - start).total_seconds() / 3600, 2),
        "rounds": max(len(rounds), declared),
        "gate_runs": len(gates),
        "gate_red": len(red),
        "gate_red_by_gate": {g: sum(1 for e in red if e.get("gate") == g)
                             for g in sorted({e.get("gate") for e in red if e.get("gate")})},
        "tokens": int(tokens),
    }


def summarize(rows: list) -> dict:
    cyc = [r["cycle_hours"] for r in rows if r["cycle_hours"] is not None]
    fixes = [r["post_merge_fixes"] for r in rows if r.get("post_merge_fixes") is not None]
    rnd = lambda x: None if x is None else round(x, 2)  # noqa: E731
    return {
        "n_runs": len(rows),
        "n_finished": len(cyc),
        "cycle_hours_median": rnd(bl.percentile(cyc, 50)),
        "cycle_hours_p75": rnd(bl.percentile(cyc, 75)),
        "rounds_median": rnd(bl.percentile([r["rounds"] for r in rows], 50)),
        "gate_red_total": sum(r["gate_red"] for r in rows),
        "tokens_total": sum(r["tokens"] for r in rows),
        "kev_calls_total": sum(r["kev"]["calls"] for r in rows),
        "n_with_fix_data": len(fixes),
        "fix_rate": rnd(sum(1 for f in fixes if f > 0) / len(fixes)) if fixes else None,
    }


def compare(summary: dict, base: dict | None) -> dict:
    """Run summary vs baseline summary (metrics/baseline.json['summary'])."""
    if not base:
        return {"baseline": None}
    b_med = (base.get("cycle_hours") or {}).get("median")
    cur = summary["cycle_hours_median"]
    return {
        "baseline_cycle_hours_median": b_med,
        "cycle_hours_median": cur,
        "cycle_delta_pct": None if (b_med in (None, 0) or cur is None) else round((cur - b_med) / b_med * 100, 1),
        "baseline_fix_rate": base.get("fix_rate_changes_with_fix"),
        "fix_rate": summary["fix_rate"],
    }


def build(runs: dict, kev_rows: list, base_summary: dict | None, git_rows: dict | None) -> dict:
    """git_rows: change id -> baseline.py row (post_merge_fixes, cycle_hours) or None to skip git."""
    rows = []
    for change in sorted(runs):
        r = aggregate_run(change, runs[change])
        g = (git_rows or {}).get(change)
        r["post_merge_fixes"] = g["post_merge_fixes"] if g and g.get("fix_window_complete") else None
        r["git_cycle_hours"] = g["cycle_hours"] if g else None  # same definition as the baseline
        if r["cycle_hours"] is None and r["git_cycle_hours"] is not None:
            r["cycle_hours"] = r["git_cycle_hours"]
        r["kev"] = kev_stats(kev_rows, change)
        rows.append(r)
    s = summarize(rows)
    return {"summary": s, "comparison": compare(s, base_summary), "runs": rows}


def render_table(data: dict) -> str:
    rows = data["runs"]
    if not rows:
        return "no ASDLC runs yet"
    f = lambda v: "-" if v is None else str(v)  # noqa: E731
    hdr = ["change", "cycle_h", "wall_h", "rounds", "gate_red", "tokens", "kev_calls", "kev_p50_ms", "fixes"]
    body = [[r["change"], f(r["cycle_hours"]), f(r["wall_hours"]), f(r["rounds"]), f(r["gate_red"]),
             f(r["tokens"]), f(r["kev"]["calls"]), f(r["kev"]["latency_ms_p50"]), f(r["post_merge_fixes"])]
            for r in rows]
    w = [max(len(x[i]) for x in [hdr] + body) for i in range(len(hdr))]
    line = lambda x: "  ".join(c.ljust(w[i]) for i, c in enumerate(x)).rstrip()  # noqa: E731
    out = [line(hdr), line(["-" * n for n in w])] + [line(x) for x in body]
    c, s = data["comparison"], data["summary"]
    out.append("")
    if c.get("baseline_cycle_hours_median") is None:
        out.append(f"vs baseline: n/a (no metrics/baseline.json); runs={s['n_runs']} finished={s['n_finished']}")
    else:
        out.append(f"vs baseline: cycle median {f(c['cycle_hours_median'])} h vs {c['baseline_cycle_hours_median']} h "
                   f"({f(c['cycle_delta_pct'])}%), fix rate {f(c['fix_rate'])} vs {f(c['baseline_fix_rate'])}; "
                   f"runs={s['n_runs']} finished={s['n_finished']}")
    return "\n".join(out)


def _abs(p) -> Path:
    p = Path(p).expanduser()
    return p if p.is_absolute() else REPO_ROOT / p


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--runs-dir", default="metrics/runs")
    ap.add_argument("--kev-log", action="append", help="repeatable; default $KEV_LOG, ~/tools/kev/log.jsonl, gates/runs/kev.jsonl")
    ap.add_argument("--baseline", default="metrics/baseline.json")
    ap.add_argument("--out", default="metrics/summary.json")
    ap.add_argument("--no-git", action="store_true", help="skip git (post-merge fixes / archived cycle time)")
    a = ap.parse_args(argv)

    runs = load_runs(_abs(a.runs_dir))
    if not runs:
        print("no ASDLC runs yet")
        return 0
    kev_paths = a.kev_log or [os.environ.get("KEV_LOG") or "~/tools/kev/log.jsonl", "gates/runs/kev.jsonl"]
    kev_paths = [_abs(p) for p in kev_paths]
    bp = _abs(a.baseline)
    base = json.loads(bp.read_text(encoding="utf-8")).get("summary") if bp.is_file() else None
    git_rows = None
    if not a.no_git:
        try:
            git_rows = {r["id"]: r for r in bl.collect()["rows"]}
        except Exception as e:  # git missing / not a repo: still report the rest
            print(f"warning: git data unavailable ({e})", file=sys.stderr)
    data = build(runs, load_kev(kev_paths), base, git_rows)
    out = _abs(a.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(render_table(data))
    return 0


if __name__ == "__main__":
    sys.exit(main())
