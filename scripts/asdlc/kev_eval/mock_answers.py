#!/usr/bin/env python3
"""Mock-engine answers derived from labels with controllable noise (for the harness tests and dry runs).

  mock_answers.py --set ID [--noise 0.1] [--flip 0.0] [--seed 1] [--gate G] [--out FILE]

For every unit/question the "perfect" answer is: fail iff the question is in the unit's `expect_fail`.
`noise` (sd of |gaussian|) pulls the probability/confidence toward 0.5 (-> uncertain zone, escalation);
`flip` is the probability that the answer is simply wrong (a confident miss / false alarm).
Out-of-checklist units get all-pass answers (a checklist engine is blind to them).
"""
from __future__ import annotations

import argparse
import random
import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from kev_eval import common as c  # noqa: E402


def _answer(q: dict, should_fail: bool, rng: random.Random, noise: float, flip: float):
    if flip and rng.random() < flip:
        should_fail = not should_fail
    d = abs(rng.gauss(0, noise)) if noise else 0.0
    t = q["type"]
    if t == "noul":
        p_pass = max(0.001, min(0.999, (0.03 + d) if should_fail else (0.97 - d)))
        p_yes = p_pass if q.get("pass_when", True) else 1 - p_pass
        return {"yes": round(p_yes, 4)}
    conf = round(max(0.05, min(0.99, 0.92 - d)), 4)
    if t == "choice":
        opts = list(q["criteria"])
        bad = [o for o in opts if o not in q["pass_values"]]
        ch = (bad[0] if bad else opts[0]) if should_fail else q["pass_values"][0]
        return {"choice": ch, "confidence": conf, "probabilities": {ch: conf}}
    names = list(q["levels"])
    idx = 0 if should_fail else len(names) - 1
    return {"score": idx, "level": names[idx], "confidence": conf, "probabilities": {names[idx]: conf}}


def make_answers(labels: dict, gate: dict, noise: float = 0.0, flip: float = 0.0, seed: int = 1) -> dict:
    out = {}
    for uid, meta in sorted(labels["units"].items()):
        out[uid] = {}
        for q in gate["questions"]:
            rng = random.Random(c.stable_hash(str(seed), uid, q["id"]))
            out[uid][q["id"]] = _answer(q, q["id"] in meta.get("expect_fail", []), rng, noise, flip)
    return out


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default=str(c.ROOT))
    ap.add_argument("--set", required=True)
    ap.add_argument("--gate", default=c.DEFAULT_GATE)
    ap.add_argument("--noise", type=float, default=0.1)
    ap.add_argument("--flip", type=float, default=0.0)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--out")
    a = ap.parse_args(argv)
    root = Path(a.root)
    labels = c.read_json(c.set_dir(a.set, root) / "labels.json")
    gp = Path(a.gate)
    gate = yaml.safe_load((gp if gp.is_absolute() else root / gp).read_text(encoding="utf-8"))
    out = Path(a.out) if a.out else c.set_dir(a.set, root) / "mock-answers.json"
    c.write_json(out, make_answers(labels, gate, a.noise, a.flip, a.seed))
    print(out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
