# kev_eval: A/B/C experiment harness (plan section 14, W3-6)

Question: with the same checklist, does Kev + escalation catch seeded defects as well as a full-scan LLM reviewer, how much
cheaper/faster, and which defect classes does it miss? No script here calls an LLM or ssh's anywhere: Kev is reached only
through `engines.KevEngine` (`--engine kev`), the LLM reviewer is run by the orchestrator session through a subagent.

Python: `platform-e2e/.venv/bin/python`. Gate default: `gates/feature.v1.yaml` (any gate via `--gate`).

## Pipeline

```
build_set.py --limit 20 --seed 1            # archived changes -> states/clean/*.json (unit = one scenario)
seed.py --seed 1 --per-defect 5 --set-id S  # clean + seeded copies -> sets/S/units.json + labels.json
mock_answers.py --set S --noise 0.15        # (dry run only) fake Kev answers from the labels
run.py --set S --arm C --engine mock --answers sets/S/mock-answers.json
run.py --set S --arm B --engine mock --answers sets/S/mock-answers.json
run.py --set S --arm A --emit-prompts
score.py --set S --engine mock              # -> metrics/kev-eval/S/report.{md,json}
```

Defects (`seed.py`): proxy_status_only, mock_only, removed_error_case, removed_boundary_case, code_beyond_tests,
scenario_no_test (expected to be caught by a gate question) and wrong_field_assertion, off_by_one_impl,
hardcoded_test_data_leak (`expected: none`, the out-of-checklist group). A transform that cannot apply is skipped and
listed in `labels.json` -> `skipped` with the reason. `IMPL` is a synthetic handler stub (origin recorded in the unit),
because no real implementation hunk maps to a scenario; treat `code_beyond_tests` / `off_by_one_impl` numbers accordingly.
Clean units are the negatives: any question they fail counts as a false positive (human-check a sample, plan section 14).

## Running A, B, C once Kev is up

1. `run.py --set S --arm C --engine kev` (Kev only; stores `decisions.json`; exit 3 if the engine is down).
2. `run.py --set S --arm B --engine kev` reuses C's stored decisions (no second Kev pass; `--rerun` to ask again) and writes
   `runs/S/B-kev/escalations.json`: one entry per unit with the uncertain questions and a ready `prompt`.
   The orchestrator sends each prompt to an LLM subagent and saves `{unit: {question_id: "pass"|"fail"}}` as `llm.json`,
   then `run.py --set S --arm B --engine kev --llm-answers llm.json` -> `final.json` (Kev verdict, else LLM verdict).
3. `run.py --set S --arm A --emit-prompts` writes `runs/S/A-llm/raw/prompts/<unit>.md` (full checklist + all parts). The
   orchestrator answers each with `{"findings": [question ids whose pass condition is NOT met], "other": ["free text"]}`
   and saves `{unit: that}` as `a.json`; `run.py --set S --arm A --ingest a.json`.
4. `score.py --set S --engine kev` -> `metrics/kev-eval/S/report.md`.
5. Kev-4B side arm: point `KEV_PY` at the 4B setup and use `--label kev4b` on every run and `score.py --engine kev-kev4b`.

## What the report gives

Per question and per standard ID: recall, precision, F1 for each arm. B: escalation rate (units and questions), LLM token
estimate (escalation prompts); A: chars/4 of the bundles; C: Kev latency (mean/p50/p95, total, GPU-hours estimate).
Out-of-checklist: catch rate (flag on a tolerated question, or A's `other`) and false alarm rate. Threshold sweep from the
raw Kev answers: for every question, `threshold` (noul, p) or `min_confidence` (choice/score) over a grid with silent-miss
rate, false-fail rate, escalation rate and recall-with-ideal-LLM; the suggested value minimises escalation subject to
`--max-miss` (0.05) and `--max-false-fail` (0.05), ties go to the most conservative value. Verdict per question:
Kev (feasible, escalation <= 25%), Kev + LLM escalation (feasible, more escalation), keep LLM (infeasible),
insufficient positives (< 3). Update `gates/*.yaml` from these numbers by hand as a new gate version.

## Tests

`platform-e2e/.venv/bin/python -m pytest scripts/asdlc/kev_eval/tests -q` (synthetic set, mock engine, no network).
