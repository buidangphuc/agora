# metrics/kev-eval

Eval data and results for the Kev A/B/C experiment (ASDLC-v2-PLAN section 14). Code: `scripts/asdlc/kev_eval/`.

| Path | Content | Tracked |
|---|---|---|
| `states/clean/*.json` | clean units (SPEC / FEATURE / STEPS / IMPL) built from archived changes | no (gitignored, rebuildable) |
| `sets/<set-id>/units.json`, `labels.json` | seeded eval set + ground truth (defect id, standard ID, expected questions, skipped transforms) | yes |
| `runs/<set-id>/<arm>-<engine>/` | `decisions.json`, `meta.json`, `escalations.json`, `final.json`, `findings.json` | yes; `raw/` (answers, prompt bundles) no |
| `<set-id>/report.md`, `report.json` | per-question / per-standard recall, precision, F1; cost; threshold sweep | yes |

A set is versioned by its id and by the gate version in `labels.json`. Changing a question means re-running on the stored
set (`run.py ... --rerun`), never rebuilding it. See `scripts/asdlc/kev_eval/README.md`.
