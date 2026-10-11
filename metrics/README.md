# metrics/

| File | Written by | Purpose |
|---|---|---|
| `baseline.json` | `scripts/asdlc/baseline.py` | "Before" numbers from git history (see `docs/baseline.md`) |
| `runs/<change>.jsonl` | the `feature-run` skill (one line per event) | raw events of one change run |
| `summary.json` | `scripts/asdlc/metrics.py` | per-run aggregates + comparison with the baseline |

```bash
platform-e2e/.venv/bin/python scripts/asdlc/metrics.py            # table + metrics/summary.json
platform-e2e/.venv/bin/python scripts/asdlc/metrics.py --no-git   # skip git (no post-merge fix data)
# no metrics/runs/*.jsonl yet -> prints "no ASDLC runs yet"
```

## Event format: `metrics/runs/<change>.jsonl`

One JSON object per line, append-only. `ts` is ISO-8601 with offset. A torn line is skipped, not fatal.

```json
{"ts":"2026-10-11T09:30:00+00:00","change":"payout-holdback","task":"T2","phase":"build","event":"round","tokens":18000}
{"ts":"2026-10-11T09:41:00+00:00","change":"payout-holdback","task":"T2","phase":"build","event":"gate","gate":"task","result":"fail"}
{"ts":"2026-10-11T12:00:00+00:00","change":"payout-holdback","phase":"release","event":"done"}
```

| Field | Required | Meaning |
|---|---|---|
| `ts` | yes | event time |
| `change` | yes | OpenSpec change id (also the Kev `--tag` prefix, so Kev calls join on it) |
| `event` | yes | `round` = one worker<->reviewer round; `gate` = a gate verdict; `done` = change finished |
| `task`, `phase` | no | task id; lifecycle phase name |
| `gate`, `result` | on `gate` | gate name (`spec`, `task`, `feature`, `release`, `e2e`, ...); result `pass` / `fail` / `uncertain` / `block` |
| `tokens` | no | tokens spent in that step (summed per change) |

Aliases accepted because `feature-run` SKILL.md writes them today: `task_round` = `round`,
`run_done` = `done`; `run_done.rounds_total` is used when it exceeds the counted rounds.
Other `feature-run` events (`preflight`, `e2e_red`, `escalation`, `task_done`) only extend wall time.

## Definitions

- **cycle_hours**: first event -> `done` event; if the run never emitted `done`, the git cycle time
  (proposal commit -> archive commit, same definition as the baseline) when the change is archived.
- **wall_hours**: first -> last event. **rounds**: count of round events. **gate_red**: gate events whose
  result is `fail`/`red`/`block`/`would_block`/`veto`. **tokens**: sum of `tokens`.
- **Kev latency/calls**: Kev log rows (`$KEV_LOG`, `~/tools/kev/log.jsonl`, `gates/runs/kev.jsonl`) whose
  `tag` starts with the change id; `latency_ms` is the server time, errors are rows with `exit != 0`.
- **post-merge fixes**: `fix` commits on the change's repos within 14 days after archive
  (`baseline.py` heuristic, repo-level not change-level); `-` while the window is incomplete or not archived.
- Comparison: median cycle hours and fix rate vs `metrics/baseline.json` summary. Baseline is a proxy
  (batch proposals, see `docs/baseline.md` caveats), so quote deltas only with >= 3 finished runs.
