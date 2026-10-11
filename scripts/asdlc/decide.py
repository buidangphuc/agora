#!/usr/bin/env python3
"""Decision engine: gate questions + state units -> decisions.json / decisions.md.

  decide.py --gate gates/feature.v1.yaml --units units.json --engine kev|mock|replay \
            [--answers answers.json | --replay decisions.json] [--mode advisory|enforce] --out DIR [--stamp]

Rules (gates/SCHEMA.md): ENGINE_DOWN > BLOCK > ESCALATE_LLM > FAIL > PASS; NEEDS_HUMAN is a separate flag.
Advisory mode writes BLOCK as WOULD_BLOCK. Output is deterministic (sorted keys, no timestamps unless --stamp).
"""
from __future__ import annotations

import argparse
import fnmatch
import json
import math
import sys
import time
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parent))
from engines import EngineDown, EngineError, KevEngine, MockEngine, ReplayEngine, build_questions  # noqa: E402

PRECEDENCE = ["ENGINE_DOWN", "BLOCK", "ESCALATE_LLM", "FAIL", "PASS"]
HUMAN_TAGS = {"auth", "payment", "pii", "release"}
DEFAULT_ROUTES = {
    "auth": ["*auth*", "*jwt*", "*jwks*", "*login*", "*scope*"],
    "payment": ["*payment*", "*billing*", "*checkout*"],
    "pii": ["*pii*", "*personal*", "*gdpr*"],
    "proto": ["*.proto"],
    "release": ["platform-gitops/envs/prod/*", "deploy/images/*"],
}


def est_tokens(parts: dict) -> int:
    return math.ceil(sum(len(v) for v in parts.values()) / 4)


_WARNED: set[str] = set()


def _warn_once(msg: str) -> None:
    if msg not in _WARNED:
        _WARNED.add(msg)
        print(f"decide.py: WARNING: {msg}", file=sys.stderr)


def _load_config() -> dict:
    sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
    import asdlc_config  # type: ignore
    return asdlc_config.load()


def route(paths: list[str]) -> list[str]:
    """Routing flags from changed paths. asdlc.config.yaml `routing` is the single source;
    DEFAULT_ROUTES is only a fallback when the config fails to load."""
    try:
        cfg = _load_config()
        import asdlc_config  # type: ignore
        return sorted(asdlc_config.route(paths, cfg))
    except Exception as e:  # noqa: BLE001
        _warn_once(f"config unavailable ({e!r}); using built-in routing globs")
    flags = set()
    for p in paths:
        pl = p.lower()
        for flag, pats in DEFAULT_ROUTES.items():
            if any(fnmatch.fnmatchcase(pl, g) for g in pats):
                flags.add(flag)
    return sorted(flags)


def default_mode() -> str:
    try:
        m = _load_config().get("kev_mode")
        if m in ("advisory", "enforce"):
            return m
    except Exception as e:  # noqa: BLE001
        _warn_once(f"cannot read kev_mode from config ({e!r}); defaulting to advisory")
    return "advisory"


def evaluate(q: dict, ans) -> tuple[str, str]:
    """-> (verdict pass|fail|uncertain, reason)."""
    if not isinstance(ans, dict):
        return "uncertain", "no answer"
    t = q["type"]
    try:
        if t == "noul":
            p_yes = float(ans["yes"])
            p = p_yes if q.get("pass_when", True) else 1 - p_yes
            thr = float(q["threshold"])
            if p >= thr:
                return "pass", f"p={p:.4f} >= {thr}"
            if p <= 1 - thr:
                return "fail", f"p={p:.4f} <= {1 - thr:.4g}"
            return "uncertain", f"p={p:.4f} in ({1 - thr:.4g}, {thr})"
        conf = float(ans["confidence"])
        mc = float(q.get("min_confidence", 0))
        if t == "choice":
            ch = ans["choice"]
            if ch not in q["criteria"]:
                return "uncertain", f"unknown choice {ch!r}"
            if conf < mc:
                return "uncertain", f"{ch} confidence {conf:.4f} < {mc}"
            return ("pass" if ch in q["pass_values"] else "fail"), f"{ch} conf={conf:.4f}"
        if t == "score":
            names = list(q["levels"])
            if isinstance(ans.get("score"), int) and 0 <= ans["score"] < len(names):
                idx = ans["score"]
            elif ans.get("level") in names:
                idx = names.index(ans["level"])
            elif ans.get("level") in list(q["levels"].values()):
                idx = list(q["levels"].values()).index(ans["level"])
            else:
                return "uncertain", f"unresolvable level {ans.get('level')!r}"
            if conf < mc:
                return "uncertain", f"{names[idx]} confidence {conf:.4f} < {mc}"
            need = names.index(q["pass_min"])
            return ("pass" if idx >= need else "fail"), f"{names[idx]} (>= {q['pass_min']}?) conf={conf:.4f}"
    except (KeyError, TypeError, ValueError) as e:
        return "uncertain", f"malformed answer: {e!r}"
    return "uncertain", f"unknown type {t!r}"


def unit_result(verdicts: dict, qs: dict, mode: str) -> str:
    if any(v == "fail" and qs[i]["severity"] == "veto" for i, v in verdicts.items()):
        return "BLOCK" if mode == "enforce" else "WOULD_BLOCK"
    if any(v == "uncertain" for v in verdicts.values()):
        return "ESCALATE_LLM"
    if any(v == "fail" for v in verdicts.values()):
        return "FAIL"
    return "PASS"


def rank(result: str) -> int:
    r = "BLOCK" if result == "WOULD_BLOCK" else result
    return PRECEDENCE.index(r)


def decide_unit(unit, gate, qs, questions_wire, engine, mode, stamp):
    uid = unit["id"]
    parts = unit.get("parts", {})
    rec = {"id": uid, "tokens_est": est_tokens(parts), "routing": route(unit.get("paths", [])),
           "questions": {}, "escalations": [], "note": None, "timing_ms": None}
    rec["needs_human"] = bool(set(rec["routing"]) & HUMAN_TAGS)  # proto routes to contract-boundary-reviewer, not a human
    t0 = time.perf_counter()
    if rec["tokens_est"] > gate["max_state_tokens"]:
        rec["result"] = "ESCALATE_LLM"
        rec["note"] = f"state too large ({rec['tokens_est']} > {gate['max_state_tokens']} tokens)"
        rec["escalations"] = sorted(qs)
        return rec
    try:
        answers = engine.ask(parts, questions_wire, uid)
        rec["timing_ms"] = engine.last_latency_ms
    except EngineDown as e:
        rec["result"], rec["note"] = "ENGINE_DOWN", str(e)
        return rec
    except EngineError as e:
        rec["result"], rec["note"] = "ESCALATE_LLM", f"engine error: {e}"
        rec["escalations"] = sorted(qs)
        return rec
    verdicts = {}
    for qid in sorted(qs):
        raw = answers.get(qid)
        v, why = evaluate(qs[qid], raw)
        verdicts[qid] = v
        rec["questions"][qid] = {"raw": raw, "verdict": v, "reason": why,
                                 "severity": qs[qid]["severity"], "source": qs[qid]["source"]}
    rec["result"] = unit_result(verdicts, qs, mode)
    if rec["result"] == "ESCALATE_LLM":
        rec["escalations"] = [i for i in sorted(verdicts) if verdicts[i] == "uncertain"]
    if stamp:
        rec["wall_ms"] = round((time.perf_counter() - t0) * 1000, 1)
    return rec


def run(gate_path, units, engine, mode, stamp=False):
    gate = yaml.safe_load(Path(gate_path).read_text(encoding="utf-8"))
    qs = {q["id"]: q for q in gate["questions"]}
    wire = build_questions(gate["questions"])
    recs = [decide_unit(u, gate, qs, wire, engine, mode, stamp) for u in sorted(units, key=lambda u: u["id"])]
    overall = "PASS"
    for r in recs:
        if rank(r["result"]) < rank(overall):
            overall = r["result"]
    if recs and overall == "PASS":
        pass
    # prefer WOULD_BLOCK label if any unit carries it and it is the worst class
    worst = min((rank(r["result"]) for r in recs), default=PRECEDENCE.index("PASS"))
    overall = PRECEDENCE[worst]
    if overall == "BLOCK" and mode != "enforce":
        overall = "WOULD_BLOCK"
    doc = {
        "gate": gate["gate"], "version": gate["version"], "mode": mode, "engine": engine.name,
        "result": overall,
        "needs_human": any(r["needs_human"] for r in recs),
        "fallback": "llm_reviewer" if overall == "ENGINE_DOWN" else None,
        "escalations": [{"unit": r["id"], "questions": r["escalations"]} for r in recs
                        if r["result"] == "ESCALATE_LLM"],
        "units": recs,
    }
    if stamp:
        doc["stamp"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return doc


def to_markdown(doc: dict) -> str:
    L = [f"### Decision gate `{doc['gate']}` v{doc['version']}: **{doc['result']}**"
         + (" (NEEDS_HUMAN)" if doc["needs_human"] else ""),
         f"mode `{doc['mode']}`, engine `{doc['engine']}`", ""]
    if doc["fallback"]:
        L += ["Engine unreachable: fall back to the LLM reviewer.", ""]
    L += ["| Unit | Result | Human | Veto/fail | Uncertain | Note |", "|---|---|---|---|---|---|"]
    for u in doc["units"]:
        qd = u["questions"]
        bad = ", ".join(i for i, q in qd.items() if q["verdict"] == "fail")
        unc = ", ".join(i for i, q in qd.items() if q["verdict"] == "uncertain") or ", ".join(u["escalations"])
        human = ",".join(u["routing"]) if u["needs_human"] else ""
        L.append(f"| {u['id']} | {u['result']} | {human} | {bad} | {unc} | {u['note'] or ''} |")
    return "\n".join(L) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--gate", required=True)
    ap.add_argument("--units", required=True)
    ap.add_argument("--engine", choices=["kev", "mock", "replay"], default="kev")
    ap.add_argument("--answers", help="mock: {unit_id: {qid: answer}}; replay: previous decisions.json")
    ap.add_argument("--mode", choices=["advisory", "enforce"])
    ap.add_argument("--out", required=True)
    ap.add_argument("--stamp", action="store_true", help="add timestamp and wall timings (non-deterministic)")
    a = ap.parse_args(argv)
    if a.engine in ("mock", "replay") and not a.answers:
        ap.error(f"--engine {a.engine} needs --answers FILE")
    engine = {"kev": lambda: KevEngine(), "mock": lambda: MockEngine.from_file(a.answers),
              "replay": lambda: ReplayEngine.from_file(a.answers)}[a.engine]()
    units = json.loads(Path(a.units).read_text(encoding="utf-8"))
    doc = run(a.gate, units, engine, a.mode or default_mode(), a.stamp)
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    (out / "decisions.json").write_text(json.dumps(doc, indent=2, sort_keys=True, ensure_ascii=False) + "\n", encoding="utf-8")
    (out / "decisions.md").write_text(to_markdown(doc), encoding="utf-8")
    print(f"{doc['gate']} v{doc['version']}: {doc['result']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
