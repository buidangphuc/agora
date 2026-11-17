"""Eval gate for the completions surface.

Runs the JSONL dataset against the real CompletionService (echo handler by
default — swap in your product handler when you fork) and FAILS (exit != 0)
when the pass rate drops below --min-score.

Per-case evaluator selection via metadata.evaluator: exact | contains |
judge. Judge cases need JUDGE_CHAT_MODEL (+ the [ai] extra); without it they
are SKIPPED so the gate stays hermetic in CI.

    python -m scripts.run_eval                    # default dataset + 0.8 gate
    python -m scripts.run_eval --cases evals/x.jsonl --min-score 0.9
"""

from __future__ import annotations

import argparse
import asyncio
import json
import re

from app.core.config import get_settings
from app.core.redaction import RedactionPolicy
from app.modules.ai.evals.evaluators import (
    CallableEvaluator,
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
from app.modules.business.ai_assistant.adapters.listing import ListingFacts
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.copilot_llm import (
    claims_availability,
    promises_delivery,
    says_out_of_stock,
)
from app.modules.business.ai_assistant.grounding import GroundedCards
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ProductCard,
    ShoppingAssistantRequest,
)
from app.modules.business.ai_assistant.shopping import (
    MockAssistantReplier,
    ShoppingAssistantService,
)
from app.modules.business.completions.handlers.echo import EchoCompletionHandler
from app.modules.business.completions.schemas import CompletionRequest
from app.modules.business.completions.service import CompletionService

DEFAULT_CASES = "evals/completions_smoke.jsonl"

# The eval target — swap the handler for your product's when you fork.
_completion_service = CompletionService(EchoCompletionHandler())


class _CaseGrounding:
    def __init__(self, cards: list[ProductCard]) -> None:
        self._cards = cards

    async def ground(self, query: str) -> GroundedCards:
        return GroundedCards(cards=self._cards)


async def _assistant_target(case: EvalCase) -> EvalTargetResult:
    cards = [ProductCard(**c) for c in case.input.get("cards", [])]
    service = ShoppingAssistantService(
        _CaseGrounding(cards),  # type: ignore[arg-type]
        MockAssistantReplier(),
        redaction=RedactionPolicy(mode="redacted"),
    )
    answer = await service.answer(
        ShoppingAssistantRequest(message=case.input["message"])
    )
    return EvalTargetResult(
        output={
            "reply_text": answer.reply_text,
            "cards": [c.model_dump() for c in answer.product_cards],
            "offered": [c.model_dump() for c in cards],
        }
    )


class _CaseListing:
    def __init__(self, facts: ListingFacts | None) -> None:
        self._facts = facts

    async def get(self, listing_id: str, *, timeout_s: float) -> ListingFacts | None:
        return self._facts


def _build_copilot(listing: _CaseListing) -> ChatCopilotService:
    return ChatCopilotService(
        redaction=RedactionPolicy(mode="redacted"), listing=listing
    )


async def _copilot_target(case: EvalCase) -> EvalTargetResult:
    raw = case.input.get("facts")
    facts = ListingFacts(**raw) if raw else None
    copilot = _build_copilot(_CaseListing(facts))
    response = await copilot.suggest(
        ChatCopilotRequest(
            buyer_message=case.input["message"],
            seller_id=case.input["seller_id"],
            listing_id=case.input["listing_id"],
        )
    )
    return EvalTargetResult(
        output={
            "drafts": response.quick_replies,
            "facts": raw,
            "seller_id": case.input["seller_id"],
        }
    )


def _is_copilot_grounded(output: dict, expected: object) -> bool:
    facts = output["facts"]
    in_stock = bool(
        facts
        and facts.get("published")
        and facts.get("seller_id") == output["seller_id"]
        and facts.get("stock", 0) > 0
    )
    for draft in output["drafts"]:
        if promises_delivery(draft):
            return False
        if claims_availability(draft) and not in_stock:
            return False
        if in_stock and says_out_of_stock(draft):
            return False
    return True


def _is_grounded(output: dict, expected: object) -> bool:
    cards = [ProductCard(**c) for c in output["cards"]]
    if cards != [ProductCard(**c) for c in output["offered"]]:
        return False
    text = output["reply_text"]
    if not cards:
        return "**" not in text
    titles = {c.title.casefold() for c in cards}
    prices = {str(c.price) for c in cards}
    for bold in re.findall(r"\*\*(.+?)\*\*", text):
        key = bold.strip().casefold()
        digits = re.sub(r"[.,đ₫]|vnđ|vnd", "", key)
        if key not in titles and digits not in prices:
            return False
    return cards[0].title in text


async def _target(case: EvalCase) -> EvalTargetResult:
    if case.metadata.get("evaluator") == "grounded":
        return await _assistant_target(case)
    if case.metadata.get("evaluator") == "copilot_grounded":
        return await _copilot_target(case)
    result = await _completion_service.complete(CompletionRequest(**case.input))
    return EvalTargetResult(output=result.content)


def _build_evaluators(settings=None) -> dict[str, Evaluator | None]:
    settings = settings or get_settings()
    judge: Evaluator | None = None
    if settings.JUDGE_CHAT_MODEL:
        judge = build_llm_judge(settings)
    return {
        "exact": ExactMatchEvaluator(),
        "contains": ContainsEvaluator(),
        "grounded": CallableEvaluator(name="grounded", evaluate=_is_grounded),
        "copilot_grounded": CallableEvaluator(
            name="copilot_grounded", evaluate=_is_copilot_grounded
        ),
        "judge": judge,
    }


async def _run(args: argparse.Namespace, settings=None) -> int:
    cases = load_jsonl_cases(args.cases)
    evaluators = _build_evaluators(settings)

    scored = passed = skipped = 0
    failures: list[str] = []
    for case in cases:
        kind = case.metadata.get("evaluator", "exact")
        evaluator = evaluators.get(kind)
        if evaluator is None:
            skipped += 1
            continue
        target_result = await _target(case)
        scores = await evaluate_one(
            output=target_result.output,
            expected=case.expected,
            evaluators=[evaluator],
        )
        for score in scores:
            scored += 1
            if score.passed:
                passed += 1
            else:
                failures.append(f"{case.id} [{score.name}] {score.comment or ''}")

    rate = (passed / scored) if scored else 0.0
    print(
        json.dumps(
            {
                "cases": len(cases),
                "scored": scored,
                "passed": passed,
                "skipped_judge": skipped,
                "pass_rate": round(rate, 4),
                "min_score": args.min_score,
                "failures": failures,
            },
            sort_keys=True,
            ensure_ascii=False,
        )
    )
    return 0 if rate >= args.min_score else 1


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--cases", default=DEFAULT_CASES)
    parser.add_argument("--min-score", type=float, default=0.8)
    args = parser.parse_args(argv)
    raise SystemExit(asyncio.run(_run(args)))


if __name__ == "__main__":
    main()
