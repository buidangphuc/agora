#!/usr/bin/env python3
"""Build clean eval units from archived OpenSpec changes (plan section 14).

  build_set.py [--root DIR] [--limit N] [--seed S] [--out metrics/kev-eval/states/clean]
               [--no-require-assertions] [--impl synthetic|none]

Unit = ONE spec scenario that an archived change added and that platform-e2e covers:
  SPEC    the `#### Scenario:` block from openspec/changes/archive/*/specs/*/spec.md
  FEATURE the gherkin Scenario (same title; FEATURES.yaml `covered_by` confirms the mapping)
  STEPS   source of the pytest-bdd step functions the scenario's steps resolve to
  IMPL    optional implementation hunk (default: a SYNTHETIC handler stub derived from the
          scenario, so `code_beyond_tests` has something to judge; origin is recorded)
Deterministic: candidates are sorted, then picked round-robin over changes with random.Random(seed).
"""
from __future__ import annotations

import argparse
import difflib
import json
import random
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from kev_eval import common as c  # noqa: E402

MAX_STEPS_CHARS = 12000  # keep units well under the gate's max_state_tokens (8000 tokens ~ 32k chars)


def _spec_number(spec_text: str) -> int | None:
    for m in re.finditer(r"\b(\d{1,4})\b", spec_text):
        n = int(m.group(1))
        if 2 <= n <= 1000:
            return n
    return None


def synth_impl(name: str, spec_text: str) -> str:
    """Synthetic implementation hunk. Clearly marked; used only so IMPL-dependent questions/defects have input."""
    n = _spec_number(spec_text)
    limit = n if n is not None else 10
    return (
        f"# synthetic implementation hunk (origin: kev_eval/build_set.py) for scenario: {name}\n"
        f"MAX_ITEMS = {limit}\n\n"
        "def handle(req, store):\n"
        "    if not req.principal:\n"
        "        raise Unauthorized()\n"
        "    if len(req.items) > MAX_ITEMS:\n"
        '        raise BadRequest("too many items")\n'
        "    record = store.save(req)\n"
        "    return Response(200, record)\n"
    )


def format_steps(defs: list[dict]) -> str:
    return "\n\n".join(f"# {d['file']}::{d['func']}\n{d['source']}" for d in defs)


def candidates(root: Path, require_assertions: bool, impl: str) -> tuple[list[dict], dict]:
    """All mappable units from archived changes. -> (units, exclusion counters)."""
    from kev_eval.seed import then_assertions_count  # local import: avoids a cycle at module load
    e2e = root / c.E2E_REL
    cov = c.load_covered_by(root)
    by_title: dict[str, list[str]] = {}
    for key in cov:
        by_title.setdefault(c.norm_title(key.split("::", 1)[1]), []).append(key)
    defs = c.load_step_defs(e2e / "step_definitions")
    feat_cache: dict[str, dict] = {}

    def feature_scenarios(rel: str) -> dict:
        if rel not in feat_cache:
            f = e2e / "features" / rel
            feat_cache[rel] = {c.norm_title(s["name"]): s for s in c.parse_feature(f.read_text(encoding="utf-8"))} if f.exists() else {}
        return feat_cache[rel]

    ex = {"spec_scenarios": 0, "no_feature_match": 0, "ambiguous_or_missing_scenario": 0, "steps_unresolved": 0,
          "no_steps": 0, "too_large": 0, "no_then_assertion": 0}
    units = []
    arch = root / "openspec/changes/archive"
    for ch in sorted(p for p in arch.iterdir() if p.is_dir()) if arch.exists() else []:
        change_txt = " ".join(p.read_text(encoding="utf-8", errors="ignore") for p in ch.glob("*.md"))
        for sp in sorted(ch.glob("specs/*/spec.md")):
            md = sp.read_text(encoding="utf-8")
            for sc in c.spec_scenarios(md):
                ex["spec_scenarios"] += 1
                nt = c.norm_title(sc["name"])
                keys = by_title.get(nt)
                if not keys:
                    close = difflib.get_close_matches(nt, list(by_title), 1, 0.9)
                    keys = by_title[close[0]] if close else None
                if not keys:
                    ex["no_feature_match"] += 1
                    continue
                keys = sorted(keys)
                hint = [k for k in keys if Path(k.split("::")[0]).stem in change_txt]
                key = (hint or keys)[0]
                rel, fname = key.split("::", 1)
                fs = feature_scenarios(rel).get(c.norm_title(fname))
                if fs is None:
                    ex["ambiguous_or_missing_scenario"] += 1
                    continue
                matched, missing = c.resolve_steps(fs["background"] + fs["steps"], defs)
                if missing:
                    ex["steps_unresolved"] += 1
                    continue
                if not matched:
                    ex["no_steps"] += 1
                    continue
                steps_txt = format_steps(matched)
                if len(steps_txt) > MAX_STEPS_CHARS:
                    ex["too_large"] += 1
                    continue
                if require_assertions and then_assertions_count(steps_txt) == 0:
                    ex["no_then_assertion"] += 1
                    continue
                uid = f"{ch.name}__{c.slug(sc['name'])}"
                parts = {"SPEC": sc["text"], "FEATURE": fs["text"], "STEPS": steps_txt}
                if impl == "synthetic":
                    parts["IMPL"] = synth_impl(sc["name"], sc["text"])
                info = cov[key]
                units.append({
                    "id": uid, "change": ch.name, "requirement": c.requirement_of(md, sc["name"]),
                    "scenario": sc["name"], "feature_id": info["feature_id"], "feature_status": info["status"],
                    "feature_file": f"{c.E2E_REL}/features/{rel}", "covered_by": key,
                    "step_files": sorted({d["file"] for d in matched}),
                    "paths": [f"{c.E2E_REL}/features/{rel}"] + [f"{c.E2E_REL}/step_definitions/{d['file']}" for d in matched],
                    "impl_origin": "synthetic" if impl == "synthetic" else None,
                    "parts": parts,
                })
    # unique ids (same change may repeat a scenario title across capabilities)
    seen: dict[str, int] = {}
    for u in units:
        n = seen.get(u["id"], 0)
        seen[u["id"]] = n + 1
        if n:
            u["id"] = f"{u['id']}-{n + 1}"
    return units, ex


def _applicable(u: dict, defect: str) -> bool:
    from kev_eval import seed as sd
    try:
        sd.DEFECTS[defect]["fn"](u, random.Random(0))
        return True
    except sd.Skip:
        return False


def pick(units: list[dict], limit: int | None, seed: int, reserve: float = 0.25) -> list[dict]:
    """Deterministic and diverse. Rare defect classes (error/permission case, boundary) are scarce in a random
    sample, so `reserve` of the slots per class go to units whose text lets that transform apply (stratified,
    chosen from the spec/test text only); the rest is round-robin over changes."""
    if limit is None or limit >= len(units):
        return sorted(units, key=lambda u: u["id"])
    rng = random.Random(seed)
    chosen: list[dict] = []
    ids: set[str] = set()
    want = max(1, int(-(-limit * reserve // 1)))
    for defect in ("removed_error_case", "removed_boundary_case"):
        pool = [u for u in sorted(units, key=lambda u: u["id"]) if u["id"] not in ids and _applicable(u, defect)]
        rng.shuffle(pool)
        by_change: dict[str, dict] = {}
        for u in pool:
            by_change.setdefault(u["change"], u)  # at most one per change first, keeps diversity
        for u in list(by_change.values())[:want]:
            chosen.append(u)
            ids.add(u["id"])
    by_ch: dict[str, list[dict]] = {}
    for u in sorted(units, key=lambda u: u["id"]):
        if u["id"] not in ids:
            by_ch.setdefault(u["change"], []).append(u)
    order = sorted(by_ch, key=lambda ch: ch in {x["change"] for x in chosen})  # untouched changes first
    rng.shuffle(order)
    order.sort(key=lambda ch: ch in {x["change"] for x in chosen})
    for ch in order:
        rng.shuffle(by_ch[ch])
    chosen = chosen[:limit]
    while len(chosen) < limit and any(by_ch.values()):
        for ch in order:
            if by_ch[ch] and len(chosen) < limit:
                chosen.append(by_ch[ch].pop())
    return sorted(chosen, key=lambda u: u["id"])


def build(root: Path, limit: int | None, seed: int, out: Path, require_assertions: bool = True,
          impl: str = "synthetic") -> dict:
    units, ex = candidates(root, require_assertions, impl)
    chosen = pick(units, limit, seed)
    out.mkdir(parents=True, exist_ok=True)
    for old in out.glob("*.json"):
        old.unlink()
    for u in chosen:
        c.write_json(out / f"{u['id']}.json", u)
    summary = {"candidates": len(units), "selected": len(chosen), "changes": len({u['change'] for u in chosen}),
               "seed": seed, "limit": limit, "exclusions": ex, "out": str(out)}
    c.write_json(out / "_manifest.json", {**summary, "ids": [u["id"] for u in chosen]})
    return summary


def load_clean(dir_: Path) -> list[dict]:
    return [json.loads(p.read_text(encoding="utf-8")) for p in sorted(dir_.glob("*.json")) if not p.name.startswith("_")]


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default=str(c.ROOT))
    ap.add_argument("--limit", type=int)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--out")
    ap.add_argument("--no-require-assertions", action="store_true",
                    help="keep units whose Then steps hold no direct assert/expect statement")
    ap.add_argument("--impl", choices=["synthetic", "none"], default="synthetic")
    a = ap.parse_args(argv)
    root = Path(a.root)
    out = Path(a.out) if a.out else c.eval_dir(root) / "states" / "clean"
    s = build(root, a.limit, a.seed, out, not a.no_require_assertions, a.impl)
    print(json.dumps(s, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
