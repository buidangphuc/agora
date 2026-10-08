"""Eval gate for the completions surface and the chat resilience path.

Runs JSONL datasets and FAILS (exit != 0) when the pass rate drops below
--min-score. The gate cannot pass vacuously:

- every case counts in the pass-rate denominator; a case that could not be scored
  (judge case with no JUDGE_CHAT_MODEL, unknown evaluator) counts as a FAILURE and is
  listed under ``unscored``;
- a judge case without a configured judge makes the run exit non-zero, naming the cases.

``--allow-unjudged`` is a local-dev opt-out (unscored cases are skipped, not counted);
``make eval`` never uses it.

Per-case evaluator via metadata.evaluator: exact | contains | judge. Cases with
metadata.target == "chat_resilience" run the real LLMRouterChatStreamer over scripted
fake models (see app/modules/ai/evals/chat_resilience.py); all others run the echo
CompletionPipeline.

    python -m scripts.run_eval                    # deterministic sets + 0.8 gate
    python -m scripts.run_eval --cases evals/completions_judge.jsonl   # needs a judge
    EVAL_EXTRA_CASES=path/to/more.jsonl python -m scripts.run_eval

Output: one ``PASS|FAIL|UNSCORED <case id>`` line per case, then one JSON summary line
(keys include ``pass_rate``, ``failures``, ``unscored``, ``passed_cases``). Exit codes:
0 ok, 1 below ``--min-score``, 2 a case could not be scored (e.g. judge without a judge).
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os

from app.core.config import get_settings
from app.modules.ai.evals.evaluators import (
    ContainsEvaluator,
    Evaluator,
    ExactMatchEvaluator,
    build_llm_judge,
)
from app.modules.ai.evals.runner import (
    EvalCase,
    EvalTargetResult,
    evaluate_one,
    load_jsonl_cases,
)
from app.modules.business.completions.handlers.echo import EchoCompletionHandler
from app.modules.business.completions.pipeline import CompletionPipeline
from app.modules.business.completions.schemas import CompletionRequest

DEFAULT_CASES = [
    "evals/completions_smoke.jsonl",
    "evals/chat_resilience.jsonl",
]


def _default_settings():
    """Settings from the environment; hermetic when the DB/Redis vars are unset.

    The eval gate never touches Postgres or Redis, so a missing ``.env`` (CI, the
    e2e black-box run) must not stop it: placeholders fill the required fields.
    """
    from pydantic import ValidationError

    try:
        return get_settings()
    except ValidationError:
        from app.core.config import Settings

        return Settings(
            POSTGRES_HOST="localhost",  # pyright: ignore[reportCallIssue]
            POSTGRES_USER="eval",
            POSTGRES_PASSWORD="eval",  # pragma: allowlist secret
            POSTGRES_DB="eval",
            REDIS_HOST="localhost",
            ENVIRONMENT="test",  # pyright: ignore[reportArgumentType]
        )


# The eval target — swap the handler for your product's when you fork.
_pipeline = CompletionPipeline(EchoCompletionHandler())


async def _target(case: EvalCase, settings=None) -> EvalTargetResult:
    if case.metadata.get("target") == "chat_resilience":
        from app.modules.ai.evals.chat_resilience import run_chat_case

        return EvalTargetResult(
            output=await run_chat_case(case.input, settings or _default_settings())
        )
    result = await _pipeline.complete(CompletionRequest(**case.input))
    return EvalTargetResult(output=result.content)


def _build_evaluators(settings=None) -> dict[str, Evaluator | None]:
    settings = settings or _default_settings()
    judge: Evaluator | None = None
    if settings.JUDGE_CHAT_MODEL:
        judge = build_llm_judge(settings)
    return {
        "exact": ExactMatchEvaluator(),
        "contains": ContainsEvaluator(),
        "judge": judge,
    }


async def _run(args: argparse.Namespace, settings=None) -> int:
    paths = [args.cases] if isinstance(args.cases, str) else list(args.cases)
    # EVAL_EXTRA_CASES: one more JSONL file (path) graded alongside the datasets.
    extra = os.environ.get("EVAL_EXTRA_CASES", "").strip()
    if extra:
        paths.append(extra)
    cases = [case for path in paths for case in load_jsonl_cases(path)]
    evaluators = _build_evaluators(settings)
    allow_unjudged = bool(getattr(args, "allow_unjudged", False))

    scored = passed = 0
    failures: list[str] = []
    passed_cases: list[str] = []
    unscored: list[str] = []
    missing_judge: list[str] = []
    for case in cases:
        kind = case.metadata.get("evaluator", "exact")
        evaluator = evaluators.get(kind)
        if evaluator is None:
            unscored.append(case.id)
            if kind == "judge":
                missing_judge.append(case.id)
            continue
        target_result = await _target(case, settings)
        scores = await evaluate_one(
            output=target_result.output,
            expected=case.expected,
            evaluators=[evaluator],
        )
        for score in scores:
            scored += 1
            if score.passed:
                passed += 1
                passed_cases.append(case.id)
                print(f"PASS {case.id}")
            else:
                print(f"FAIL {case.id}")
                failures.append(
                    f"{case.id} [{score.name}] {score.comment or ''} "
                    f"output={target_result.output!r}"
                )

    # Unscored cases are failures unless explicitly skipped for local dev.
    denominator = scored if allow_unjudged else scored + len(unscored)
    rate = (passed / denominator) if denominator else 0.0
    report: dict[str, object] = {
        "cases": len(cases),
        "scored": scored,
        "passed": passed,
        "passed_cases": passed_cases,
        "unscored": unscored,
        "pass_rate": round(rate, 4),
        "min_score": args.min_score,
        "failures": failures,
    }
    if allow_unjudged:
        report["allow_unjudged"] = True
    print(json.dumps(report, sort_keys=True, ensure_ascii=False))

    for case_id in unscored:
        print(f"UNSCORED {case_id}")
    if missing_judge and not allow_unjudged:
        print(
            "eval gate FAILED: judge cases need JUDGE_CHAT_MODEL (unscored: "
            + ", ".join(missing_judge)
            + ")"
        )
        return 2
    if unscored and not allow_unjudged:
        print("eval gate FAILED: unscored cases: " + ", ".join(unscored))
        return 2
    return 0 if rate >= args.min_score and denominator else 1


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--cases",
        nargs="+",
        default=DEFAULT_CASES,
        help="JSONL dataset(s); default: the deterministic sets",
    )
    parser.add_argument("--min-score", type=float, default=0.8)
    parser.add_argument(
        "--allow-unjudged",
        action="store_true",
        help="local dev only: skip (do not fail) cases that cannot be scored",
    )
    args = parser.parse_args(argv)
    raise SystemExit(asyncio.run(_run(args)))


if __name__ == "__main__":
    main()
