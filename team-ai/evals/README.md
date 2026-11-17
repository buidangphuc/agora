# Eval datasets

`completions_smoke.jsonl` grades the completions surface **contract-level**
against the built-in `EchoCompletionHandler`, so `make eval` runs out of the
box on a fresh fork. Case format:

```json
{"id": "...", "input": {"messages": [...]}, "expected": "...", "metadata": {"evaluator": "exact|contains|judge"}}
```

- `exact` / `contains` — deterministic, always run (these gate CI).
- `judge` — LLM-as-judge; runs only when `JUDGE_CHAT_MODEL` is set (and the
  `[ai]` extra installed); otherwise the case is **skipped**, keeping CI
  hermetic. `expected` holds the grading criteria text.

## Point it at your product

1. Copy this file to `evals/<your-domain>.jsonl` and write real cases.
2. In `scripts/run_eval.py`, swap the target: build your handler/service and
   return its output for `case.input`.
3. Gate: `python -m scripts.run_eval --cases evals/<file>.jsonl --min-score 0.8`
   exits non-zero below the threshold — wire it into CI once your cases are in.

## Assistant groundedness

`assistant_grounding.jsonl` grades the shopping assistant reply over fixed
cards (evaluator `grounded`): every bolded title and every price in the reply
must be on the returned cards, and a no-card reply names no product.

    python -m scripts.run_eval --cases evals/assistant_grounding.jsonl --min-score 0.8

## Copilot groundedness

`copilot_grounding.jsonl` grades the chat copilot drafts over fixed listing
facts (evaluator `copilot_grounded`): no draft claims availability unless the
listing is published, owned by the asking seller and in stock, and no draft
promises a delivery time.

    python -m scripts.run_eval --cases evals/copilot_grounding.jsonl --min-score 0.8
