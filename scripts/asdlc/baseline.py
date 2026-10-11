#!/usr/bin/env python3
"""ASDLC baseline (W0-5): cycle time and post-merge fixes of archived OpenSpec changes.

Reads only git history (stdlib + `git` subprocess). Deterministic for a given HEAD.

Usage:
  python scripts/asdlc/baseline.py [--since YYYY-MM-DD] [--until YYYY-MM-DD]
                                   [--out metrics/baseline.json] [--md docs/baseline.md]

--since/--until filter on the ARCHIVE date (inclusive). See docs/baseline.md for the method
and the weaknesses of each heuristic.
"""
from __future__ import annotations

import argparse
import json
import re
import statistics
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
ARCHIVE = "openspec/changes/archive"
PENDING = "openspec/changes"
FIX_WINDOW_DAYS = 14
# top-level dirs that are not "repos" (process/meta/docs); never count as affected repos.
META_DIRS = {"openspec", "docs", "scripts", "metrics", "gates", "plans", "plan-analytics", "plan-mlops"}
FIX_RE = re.compile(r"^fix(\([^)]*\))?!?(:|\s)", re.IGNORECASE)
ARCHIVE_DIR_RE = re.compile(r"^(\d{4}-\d{2}-\d{2})-(.+)$")


@dataclass
class Commit:
    sha: str
    ts: datetime
    parents: int
    subject: str
    body: str
    files: list = field(default_factory=list)  # [(status, path)]


# ----------------------------------------------------------------- pure functions

def parse_ts(s: str) -> datetime:
    """ISO-8601 (git %aI) -> aware UTC datetime."""
    return datetime.fromisoformat(s.strip()).astimezone(timezone.utc)


def parse_log(raw: str) -> list:
    """Parse `git log --no-renames --name-status` output produced with LOG_FORMAT."""
    commits = []
    for rec in raw.split("\x1e"):
        if not rec.strip("\n"):
            continue
        parts = rec.lstrip("\n").split("\x1f")
        if len(parts) < 6:
            continue
        sha, ts, parents, subject, body, tail = parts[:6]
        files = []
        for line in tail.splitlines():
            if "\t" in line:
                st, path = line.split("\t", 1)
                files.append((st.strip(), path.strip()))
        commits.append(Commit(sha, parse_ts(ts), len(parents.split()), subject, body, files))
    return commits


LOG_FORMAT = "%x1e%H%x1f%aI%x1f%P%x1f%s%x1f%b%x1f"


def percentile(values, p: float):
    """Linear-interpolation percentile (numpy default), p in [0, 100]. None if empty."""
    if not values:
        return None
    v = sorted(values)
    if len(v) == 1:
        return float(v[0])
    k = (len(v) - 1) * p / 100.0
    lo = int(k)
    hi = min(lo + 1, len(v) - 1)
    return float(v[lo] + (v[hi] - v[lo]) * (k - lo))


def is_fix_commit(subject: str) -> bool:
    """Conventional-commit `fix` / `fix(scope)` / `fix!` subject."""
    return bool(FIX_RE.match(subject))


def top_dir(path: str):
    return path.split("/", 1)[0] if "/" in path else None


def touched_repos(files, repo_dirs) -> set:
    return {d for _, p in files if (d := top_dir(p)) and d in repo_dirs}


def parse_archive_dirname(name: str):
    m = ARCHIVE_DIR_RE.match(name)
    return (m.group(1), m.group(2)) if m else (None, name)


def repos_in_text(text: str, repo_dirs) -> set:
    """Repo dirs mentioned as a path-like token. Names without '-' (e.g. 'deploy')
    must be followed by '/' to avoid matching ordinary words."""
    found = set()
    for d in repo_dirs:
        tail = r"(?![\w-])" if "-" in d else r"/"
        if re.search(r"(?<![\w-])" + re.escape(d) + tail, text):
            found.add(d)
    return found


def mentions_id(commit: Commit, cid: str) -> bool:
    pat = re.compile(r"(?<![\w-])" + re.escape(cid) + r"(?![\w-])")
    return bool(pat.search(commit.subject) or pat.search(commit.body))


def first_add(commits, path_pred):
    """Earliest (by time, then sha) commit that ADDED a file matching path_pred."""
    best = None
    for c in commits:
        if any(st == "A" and path_pred(p) for st, p in c.files):
            if best is None or (c.ts, c.sha) < (best.ts, best.sha):
                best = c
    return best


def summarize(rows) -> dict:
    cyc = [r["cycle_hours"] for r in rows if r["cycle_hours"] is not None]
    comp = [r for r in rows if r["fix_window_complete"]]

    def rnd(x):
        return None if x is None else round(x, 2)

    def rate(rs):
        return rnd(sum(1 for r in rs if r["post_merge_fixes"] > 0) / len(rs)) if rs else None

    return {
        "n_changes": len(rows),
        "n_cycle_measurable": len(cyc),
        "n_archive_only": sum(1 for r in rows if r["cycle_status"] != "measured"),
        "cycle_hours": {
            "median": rnd(percentile(cyc, 50)),
            "p75": rnd(percentile(cyc, 75)),
            "p90": rnd(percentile(cyc, 90)),
            "mean": rnd(statistics.fmean(cyc)) if cyc else None,
            "max": rnd(max(cyc)) if cyc else None,
        },
        "cycle_days": {
            "median": rnd(percentile(cyc, 50) / 24) if cyc else None,
            "p75": rnd(percentile(cyc, 75) / 24) if cyc else None,
            "p90": rnd(percentile(cyc, 90) / 24) if cyc else None,
        },
        "n_fix_window_complete": len(comp),
        "fix_rate_changes_with_fix": rate(comp),
        "fix_rate_all_changes": rate(rows),
        "fixes_per_change_mean": rnd(statistics.fmean(r["post_merge_fixes"] for r in comp)) if comp else None,
        "fixes_per_change_median": rnd(percentile([r["post_merge_fixes"] for r in comp], 50)) if comp else None,
    }


def build_row(cid, archive_name, start, end, commits, repos, head_ts) -> dict:
    """One per-change record. start may be None (no pre-archive proposal found)."""
    measured = start is not None and start.ts < end.ts
    if start is None:
        status = "archive_only"       # proposal first appears already inside archive/
    elif not measured:
        status = "same_instant"       # proposed and archived at the same instant
    else:
        status = "measured"
    cycle_h = (end.ts - start.ts).total_seconds() / 3600 if measured else None
    size = None
    if measured and repos:
        size = sum(1 for c in commits if c.parents <= 1 and start.ts <= c.ts <= end.ts
                   and touched_repos(c.files, repos))
    win_end = end.ts + timedelta(days=FIX_WINDOW_DAYS)
    fixes = [c for c in commits
             if c.parents <= 1 and end.ts < c.ts <= win_end and is_fix_commit(c.subject)
             and touched_repos(c.files, repos)]
    return {
        "id": cid,
        "archive_dir": archive_name,
        "archive_date": end.ts.date().isoformat(),
        "start_commit": start.sha[:10] if start else None,
        "start": start.ts.isoformat() if start else None,
        "end_commit": end.sha[:10],
        "end": end.ts.isoformat(),
        "cycle_status": status,
        "cycle_hours": None if cycle_h is None else round(cycle_h, 2),
        "cycle_days": None if cycle_h is None else round(cycle_h / 24, 2),
        "repos": sorted(repos),
        "commits_in_window": size,
        "post_merge_fixes": len(fixes),
        "post_merge_fix_commits": [c.sha[:10] for c in sorted(fixes, key=lambda c: (c.ts, c.sha))],
        "fix_window_complete": win_end <= head_ts,
    }


# ----------------------------------------------------------------- git plumbing

def git(*args, cwd=REPO_ROOT) -> str:
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True,
                          text=True, encoding="utf-8").stdout


def read_blobs(paths, cwd=REPO_ROOT) -> dict:
    """path -> text at HEAD (missing -> '')."""
    if not paths:
        return {}
    inp = "".join(f"HEAD:{p}\n" for p in paths).encode()
    out = subprocess.run(["git", "cat-file", "--batch"], cwd=cwd, input=inp, check=True,
                         capture_output=True).stdout
    res, i = {}, 0
    for p in paths:
        nl = out.index(b"\n", i)
        hdr = out[i:nl].decode().split()
        if hdr[-1] == "missing":
            res[p] = ""
            i = nl + 1
            continue
        size = int(hdr[2])
        res[p] = out[nl + 1:nl + 1 + size].decode("utf-8", "replace")
        i = nl + 1 + size + 1
    return res


def collect(since=None, until=None, cwd=REPO_ROOT) -> dict:
    raw = git("log", "--no-renames", "--name-status", f"--format={LOG_FORMAT}", cwd=cwd)
    commits = parse_log(raw)
    head = git("rev-parse", "HEAD", cwd=cwd).strip()
    head_ts = max(c.ts for c in commits)
    repo_dirs = {d for d in git("ls-tree", "-d", "--name-only", "HEAD", cwd=cwd).split()
                 if d not in META_DIRS and not d.startswith(".")}

    arch_files = [p for p in git("ls-tree", "-r", "--name-only", "HEAD", ARCHIVE, cwd=cwd).splitlines()
                  if p.endswith(".md")]
    by_dir = {}
    for p in arch_files:
        by_dir.setdefault(p.split("/")[3], []).append(p)
    blobs = read_blobs(arch_files, cwd=cwd)

    rows = []
    for name in sorted(by_dir):
        _, cid = parse_archive_dirname(name)
        end = first_add(commits, lambda p, n=name: p == f"{ARCHIVE}/{n}/proposal.md") or \
            first_add(commits, lambda p, n=name: p.startswith(f"{ARCHIVE}/{n}/"))
        if end is None:
            continue
        d = end.ts.date()
        if (since and d < since) or (until and d > until):
            continue
        start = first_add(commits, lambda p, c=cid: p == f"{PENDING}/{c}/proposal.md")
        repos = repos_in_text("\n".join(blobs[p] for p in by_dir[name]), repo_dirs)
        for c in commits:
            if mentions_id(c, cid):
                repos |= touched_repos(c.files, repo_dirs)
        rows.append(build_row(cid, name, start, end, commits, repos, head_ts))
    rows.sort(key=lambda r: r["id"])
    return {"head": head[:12], "head_ts": head_ts.isoformat(), "rows": rows}


# ----------------------------------------------------------------- output

def render_md(data: dict, since, until) -> str:
    s, rows = data["summary"], data["changes"]
    ch, cd = s["cycle_hours"], s["cycle_days"]
    slow = sorted((r for r in rows if r["cycle_hours"] is not None),
                  key=lambda r: (-r["cycle_hours"], r["id"]))[:10]
    rng = f"{since or 'start'} .. {until or 'HEAD'}"
    cmd = "python scripts/asdlc/baseline.py"
    if since:
        cmd += f" --since {since}"
    if until:
        cmd += f" --until {until}"
    L = [
        "# ASDLC baseline (W0-5)",
        "",
        f"Source: git history at HEAD `{data['head']}`, archive dates {rng}. "
        "Generated by `scripts/asdlc/baseline.py`; raw data in `metrics/baseline.json`.",
        "Purpose: the \"before\" numbers that W5-1 compares against (ASDLC-v2-PLAN section 3).",
        "",
        "## Method",
        "",
        f"- **Change set**: every `{ARCHIVE}/<date>-<id>/` directory at HEAD (filtered by archive date).",
        f"- **start**: author time of the earliest commit that added `{PENDING}/<id>/proposal.md` "
        "(searched with `--no-renames`, so the later move into `archive/` does not hide it).",
        f"- **end**: author time of the earliest commit that added `{ARCHIVE}/<date>-<id>/proposal.md`.",
        "- **cycle time** = end - start, in hours. Only changes whose proposal was committed "
        "*before* the archive commit are *measurable*; the rest are `archive_only` / `same_instant` "
        "and are excluded from the percentiles (counted in `n_archive_only`).",
        "- **affected repos**: top-level directories (excluding meta dirs: "
        + ", ".join(f"`{d}`" for d in sorted(META_DIRS)) + ") that are either (a) mentioned as a path-like "
        "token in the archived artifacts (proposal/design/tasks/specs), or (b) touched by any commit "
        "whose message mentions the change id.",
        "- **commits in window** (size proxy): non-merge commits with start <= time <= end that touch an affected repo.",
        f"- **post-merge fix**: non-merge commit with a conventional `fix` / `fix(scope)` subject, "
        f"strictly after `end` and within {FIX_WINDOW_DAYS} days, touching at least one affected repo.",
        "- **fix rate** = share of changes with >= 1 post-merge fix, over changes whose 14-day window "
        "is complete (end + 14d <= newest commit); `fixes_per_change` uses the same set.",
        "",
        "## Caveats (read before quoting a number)",
        "",
        "1. **Archive-only changes.** Changes archived in the first snapshot commits (2026-09-04) and a few "
        "later batches never existed under `openspec/changes/<id>/` in history, so they have no start. "
        "They are listed but excluded from cycle time.",
        "2. **Batch proposals.** Many proposals were committed in one batch (e.g. 13 on 2026-09-20, 13 on 2026-10-08) "
        "while the work started earlier or was done in a different order; cycle time is then "
        "\"proposal committed -> archived\", not \"work started -> shipped\". It is a proxy, probably an "
        "underestimate of lead time and an overestimate for changes queued behind others in the same batch.",
        "3. **Single-author, agent-assisted history.** Timestamps reflect commit time, not working time; "
        "overnight/idle gaps are included. The slowest changes (about 20 days) are the 2026-09-20 proposal batch "
        "archived around 2026-10-10 after a pause; their cycle is mostly queue time, not build time.",
        "4. **Fix heuristic is repo-level, not change-level.** Fix commits are not linked to a change; any "
        "`fix` commit on a shared repo (team-ai, platform-gitops, ...) in the window counts. Overlapping "
        "changes in the same repo therefore share each other's fixes (over-count), and fixes mislabelled "
        "as `feat`/`chore` are missed (under-count). Treat it as an upper-bound-ish trend indicator.",
        "5. **Fixes that belong to the change itself.** Fixes made between start and end are not counted "
        "(they are in the window size); only fixes after archive are.",
        "6. **Right censoring.** Changes archived in the last 14 days before HEAD have an incomplete window "
        "and are excluded from the fix rate.",
        "7. **Repo detection** from text can miss a repo the change touched without naming it, or add one it "
        "only referenced. Changes with no detected repo get 0 fixes and `commits_in_window` null.",
        "8. The history itself is short (about 5 weeks, ~1k commits); percentiles over ~60 measurable changes "
        "are indicative, not statistically tight.",
        "",
        "## Summary",
        "",
        "| Metric | Value |",
        "|---|---|",
        f"| Archived changes | {s['n_changes']} |",
        f"| Cycle time measurable | {s['n_cycle_measurable']} |",
        f"| Archive-only / same instant (excluded) | {s['n_archive_only']} |",
        f"| Cycle time median | {ch['median']} h ({cd['median']} d) |",
        f"| Cycle time p75 | {ch['p75']} h ({cd['p75']} d) |",
        f"| Cycle time p90 | {ch['p90']} h ({cd['p90']} d) |",
        f"| Cycle time mean / max | {ch['mean']} h / {ch['max']} h |",
        f"| Changes with complete 14d fix window | {s['n_fix_window_complete']} |",
        f"| Fix rate (changes with >= 1 fix) | {s['fix_rate_changes_with_fix']} |",
        f"| Fixes per change (mean / median) | {s['fixes_per_change_mean']} / {s['fixes_per_change_median']} |",
        "",
        "## Top 10 slowest (cycle time)",
        "",
        "| # | Change | Cycle (h) | Cycle (d) | Commits in window | Post-merge fixes | Repos |",
        "|---|---|---|---|---|---|---|",
    ]
    for i, r in enumerate(slow, 1):
        L.append(f"| {i} | `{r['id']}` | {r['cycle_hours']} | {r['cycle_days']} | "
                 f"{r['commits_in_window']} | {r['post_merge_fixes']} | {', '.join(r['repos']) or '-'} |")
    L += ["", "## Rerun", "", "```bash", cmd, "```", "",
          "Output is deterministic for a given HEAD (no wall-clock values are written). "
          "Unit tests: `platform-e2e/.venv/bin/python -m pytest scripts/asdlc/tests/test_baseline.py`.", ""]
    return "\n".join(L)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--since", type=date.fromisoformat, help="archive date >= (YYYY-MM-DD)")
    ap.add_argument("--until", type=date.fromisoformat, help="archive date <= (YYYY-MM-DD)")
    ap.add_argument("--out", default="metrics/baseline.json")
    ap.add_argument("--md", default="docs/baseline.md")
    a = ap.parse_args(argv)

    res = collect(a.since, a.until)
    data = {
        "head": res["head"],
        "range": {"since": a.since.isoformat() if a.since else None,
                  "until": a.until.isoformat() if a.until else None},
        "fix_window_days": FIX_WINDOW_DAYS,
        "summary": summarize(res["rows"]),
        "changes": res["rows"],
    }
    for target, text in ((a.out, json.dumps(data, indent=2, ensure_ascii=False) + "\n"),
                         (a.md, render_md(data, a.since, a.until))):
        p = Path(target)
        p = p if p.is_absolute() else REPO_ROOT / p
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text, encoding="utf-8")
    s = data["summary"]
    print(f"HEAD {data['head']}: n={s['n_changes']} measurable={s['n_cycle_measurable']} "
          f"cycle_h median={s['cycle_hours']['median']} p75={s['cycle_hours']['p75']} "
          f"p90={s['cycle_hours']['p90']} fix_rate={s['fix_rate_changes_with_fix']} "
          f"(n_window={s['n_fix_window_complete']})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
