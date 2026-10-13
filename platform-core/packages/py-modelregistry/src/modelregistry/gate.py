"""Pure publish-gate decision. No I/O."""
from __future__ import annotations

import math
from collections.abc import Mapping
from dataclasses import dataclass

from .policy import Policy

TOL = 1e-9


@dataclass(frozen=True)
class Decision:
    version: int
    outcome: str  # promotable | rejected | promoted
    reason: str


def _fin(metrics: Mapping[str, float], key: str) -> float | None:
    v = metrics.get(key)
    if v is None or isinstance(v, bool):
        return None
    try:
        f = float(v)
    except (TypeError, ValueError):
        return None
    return f if math.isfinite(f) else None


def decide(
    policy: Policy,
    candidate: Mapping[str, float],
    champion: Mapping[str, float] | None,
    candidate_protocol: str | None,
    champion_protocol: str | None,
) -> tuple[str, str]:
    """Return (outcome, reason); outcome is `promotable` or `rejected`.

    `champion` is None when the model has no champion.
    """
    required = [policy.primary, *policy.guards, *policy.floors]
    for key in dict.fromkeys(required):
        if key not in candidate or candidate[key] is None:
            return "rejected", f"candidate metric {key} is missing"
        if _fin(candidate, key) is None:
            return "rejected", f"candidate metric {key} is not finite"

    for key, floor in policy.floors.items():
        value = float(candidate[key])
        if value < floor:
            return "rejected", f"floor violated: {key} {value:g} < {floor:g}"

    if champion is None:
        return "promotable", "no champion; floors met"

    if champion_protocol is None or candidate_protocol != champion_protocol:
        old = champion_protocol if champion_protocol is not None else "none"
        new = candidate_protocol if candidate_protocol is not None else "none"
        return "promotable", f"eval_protocol changed: {old} -> {new}; judged on floors only"

    cand = float(candidate[policy.primary])
    champ = _fin(champion, policy.primary)
    notes: list[str] = []
    if champ is None:
        return "promotable", f"champion {policy.primary} missing; judged on floors only"
    if champ <= 0:
        if not cand > champ:
            return "rejected", (
                f"{policy.primary} {cand:g} does not exceed champion {champ:g} "
                "(relative improvement not computed, champion <= 0)"
            )
        notes.append(f"{policy.primary} improved over non-positive champion {champ:g}")
    else:
        rel = (cand - champ) / champ
        need = policy.min_relative_improvement
        if rel < need - TOL:
            return "rejected", (
                f"{policy.primary} improvement {rel:.4g} < required {need:g} "
                f"(candidate {cand:g}, champion {champ:g})"
            )
        notes.append(f"{policy.primary} improvement {rel:.4g} >= required {need:g}")

    for key, ratio in policy.guards.items():
        champ_g = _fin(champion, key)
        if champ_g is None:
            notes.append(f"guard {key} skipped (missing on champion)")
            continue
        cand_g = float(candidate[key])
        ok = cand_g / champ_g >= ratio - TOL if champ_g > 0 else cand_g >= champ_g
        if not ok:
            return "rejected", (
                f"guard violated: {key} {cand_g:g} < {ratio:g} x champion {champ_g:g}"
            )
    return "promotable", "; ".join(notes)
