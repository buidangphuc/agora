# Eval datasets

`completions_smoke.jsonl` grades the completions surface **contract-level**
against the built-in `EchoCompletionHandler`, so `make eval` runs out of the
box on a fresh fork. Case format:

```json
{"id": "...", "input": {"messages": [...]}, "expected": "...", "metadata": {"evaluator": "exact|contains|judge"}}
```

- `exact` / `contains` — deterministic, always run (these gate CI).
- `judge` — LLM-as-judge; needs `JUDGE_CHAT_MODEL` (+ the `[ai]` extra).
  `expected` holds the grading criteria text. A judge case with no judge is **not**
  skipped: the gate exits 2 and names it, and it counts as a failure in the pass rate.
  Judge cases therefore live in `completions_judge.jsonl`, outside the default sets.

## Datasets and gates

| File | Target | Run by |
|---|---|---|
| `completions_smoke.jsonl` | echo `CompletionPipeline` | `make eval` |
| `chat_resilience.jsonl` | the real `LLMRouterChatStreamer` over scripted fake models (429 / 5xx / timeout / failure after the first chunk, breaker recovery, redaction) | `make eval` |
| `completions_judge.jsonl` | echo pipeline graded by an LLM judge | `python -m scripts.run_eval --cases evals/completions_judge.jsonl` |

`EVAL_EXTRA_CASES=<path.jsonl>` adds one more dataset to any run. Output: a
`PASS|FAIL|UNSCORED <id>` line per case, then a JSON summary (`pass_rate`, `failures`,
`unscored`, `passed_cases`). Exit 0 ok, 1 below `--min-score`, 2 unscored cases.

## Point it at your product

1. Copy this file to `evals/<your-domain>.jsonl` and write real cases.
2. In `scripts/run_eval.py`, swap the target: build your handler/service and
   return its output for `case.input`.
3. Gate: `python -m scripts.run_eval --cases evals/<file>.jsonl --min-score 0.8`
   exits non-zero below the threshold — wire it into CI once your cases are in.
