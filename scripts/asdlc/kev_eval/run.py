#!/usr/bin/env python3
"""Run the A/B/C arms of the Kev experiment on a labelled set (plan section 14).

  run.py --set ID --arm C --engine mock|kev [--answers mock.json] [--gate gates/feature.v1.yaml]
  run.py --set ID --arm B --engine mock|kev [--answers mock.json] [--llm-answers llm.json]
  run.py --set ID --arm A --emit-prompts
  run.py --set ID --arm A --ingest findings.json

C = Kev only (decide.py per unit).   B = Kev, uncertain questions escalate: written to escalations.json for the
orchestrator's LLM subagent; its answers come back through --llm-answers (no LLM call happens here).
A = LLM full-scan reviewer: --emit-prompts writes one bundle per unit, --ingest records its findings.
Outputs: metrics/kev-eval/runs/<set>/<arm>-<engine|llm>[-label]/ (decisions.json, meta.json, raw/...).
"""
from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parents[0]))
from kev_eval import common as c  # noqa: E402
import decide  # noqa: E402
from engines import KevEngine, MockEngine, build_questions  # noqa: E402


def load_gate(root: Path, gate: str) -> tuple[dict, Path]:
    gp = Path(gate)
    gp = gp if gp.is_absolute() else root / gp
    return yaml.safe_load(gp.read_text(encoding="utf-8")), gp


def load_set(set_id: str, root: Path) -> tuple[list[dict], dict]:
    d = c.set_dir(set_id, root)
    return c.read_json(d / "units.json"), c.read_json(d / "labels.json")


# ------------------------------------------------------------------ prompts ---
def render_questions(questions: list[dict]) -> str:
    L = []
    for q in questions:
        L.append(f"- `{q['id']}` ({q['type']}, severity {q['severity']}; source: {q['source']})")
        L.append(f"  Question: {q['instructions']}")
        if q["type"] == "noul":
            L.append(f"  Pass when the answer is {'YES' if q.get('pass_when', True) else 'NO'}.")
        elif q["type"] == "choice":
            for k, v in q["criteria"].items():
                L.append(f"    - {k}: {v}")
            L.append(f"  Pass values: {', '.join(q['pass_values'])}")
        else:
            for k, v in q["levels"].items():
                L.append(f"    - {k}: {v}")
            L.append(f"  Pass when level >= {q['pass_min']}")
    return "\n".join(L)


def render_parts(parts: dict) -> str:
    return "\n\n".join(f"## {k}\n```\n{v}\n```" for k, v in parts.items())


def prompt_full_scan(unit: dict, questions: list[dict]) -> str:
    return (
        "You are a test reviewer. Review the ONE e2e scenario below (spec, gherkin, step code, optional implementation hunk). "
        "Read everything, do not guess from titles.\n\n"
        "Report as JSON only: {\"findings\": [<question ids whose PASS condition is NOT met>], "
        "\"other\": [<short free-text problems you found that no question covers>]}.\n\n"
        f"### Checklist\n{render_questions(questions)}\n\n### Unit `{unit['id']}`\n{render_parts(unit['parts'])}\n"
    )


def prompt_escalation(unit: dict, questions: list[dict]) -> str:
    return (
        "You are a test reviewer answering ONLY the questions below about ONE e2e scenario. A cheaper model was unsure.\n\n"
        "Report as JSON only: {\"<question id>\": \"pass\" | \"fail\"} (fail = the PASS condition is NOT met).\n\n"
        f"### Questions\n{render_questions(questions)}\n\n### Unit `{unit['id']}`\n{render_parts(unit['parts'])}\n"
    )


# --------------------------------------------------------------- helpers ---
def _label(arm: str, engine: str, label: str | None) -> str:
    return engine + (f"-{label}" if label else "")


def verdict_from_llm(q: dict, ans) -> str | None:
    """Normalise an LLM answer to 'pass'|'fail' (None if unusable)."""
    if isinstance(ans, dict):
        ans = ans.get("verdict", ans.get("answer"))
    if isinstance(ans, bool):
        return "pass" if ans == q.get("pass_when", True) else "fail" if q["type"] == "noul" else None
    if isinstance(ans, str):
        a = ans.strip().lower()
        if a in ("pass", "fail"):
            return a
        if q["type"] == "noul" and a in ("yes", "no"):
            return "pass" if (a == "yes") == q.get("pass_when", True) else "fail"
        if q["type"] == "choice" and ans in q["criteria"]:
            return "pass" if ans in q["pass_values"] else "fail"
        if q["type"] == "score" and ans in q["levels"]:
            names = list(q["levels"])
            return "pass" if names.index(ans) >= names.index(q["pass_min"]) else "fail"
    return None


def make_engine(name: str, answers: str | None):
    if name == "mock":
        if not answers:
            raise SystemExit("--engine mock needs --answers FILE (see mock_answers.py)")
        return MockEngine.from_file(answers)
    return KevEngine()


def run_decisions(units, gate_path: Path, engine, mode: str) -> dict:
    return decide.run(gate_path, units, engine, mode)


def write_decisions(out: Path, doc: dict) -> None:
    c.write_json(out / "decisions.json", doc)
    (out / "decisions.md").write_text(decide.to_markdown(doc), encoding="utf-8")
    c.write_json(out / "raw" / "answers.json",
                 {u["id"]: {q: v["raw"] for q, v in u["questions"].items() if v.get("raw") is not None} for u in doc["units"]})


def meta_for(arm, engine, set_id, gate, doc, t0) -> dict:
    t = [u["timing_ms"] for u in doc["units"] if u.get("timing_ms") is not None]
    return {"arm": arm, "engine": engine, "set": set_id, "gate": gate["gate"], "gate_version": gate["version"],
            "units": len(doc["units"]), "engine_ms_total": round(sum(t), 1), "engine_calls": len(t),
            "wall_s": round(time.perf_counter() - t0, 2), "result": doc["result"],
            "engine_down_units": sum(1 for u in doc["units"] if u["result"] == "ENGINE_DOWN")}


# -------------------------------------------------------------------- arms ---
def run_c(a, root, units, gate, gate_path) -> int:
    out = c.run_dir(a.set, "C", _label("C", a.engine, a.label), root)
    t0 = time.perf_counter()
    doc = run_decisions(units, gate_path, make_engine(a.engine, a.answers), a.mode)
    write_decisions(out, doc)
    c.write_json(out / "meta.json", meta_for("C", a.engine, a.set, gate, doc, t0))
    print(f"C-{a.engine}: {doc['result']} -> {out}")
    return 3 if doc["result"] == "ENGINE_DOWN" else 0


def to_escalate(rec: dict, all_ids: list[str]) -> list[str]:
    """Questions that go to the LLM: every uncertain one (whatever the unit result), or all of them when the
    engine gave no answers (down / error / state too large)."""
    if not rec.get("questions"):
        return sorted(all_ids)
    return sorted(q for q, v in rec["questions"].items() if v["verdict"] == "uncertain")


def escalation_bundle(units_by_id, gate, rec) -> dict:
    qs = {q["id"]: q for q in gate["questions"]}
    ids = to_escalate(rec, list(qs))
    sub = [qs[i] for i in ids if i in qs]
    u = units_by_id[rec["id"]]
    prompt = prompt_escalation(u, sub)
    return {"unit": rec["id"], "questions": [q["id"] for q in sub], "reason": rec.get("note") or "uncertain",
            "tokens_est": c.est_tokens_text(prompt), "prompt": prompt,
            "wire_questions": build_questions(sub)}


def apply_llm(doc: dict, gate: dict, llm: dict) -> dict:
    """Merge Kev verdicts with escalated LLM verdicts -> {unit: {qid: {verdict, by}}} + per-unit result."""
    qs = {q["id"]: q for q in gate["questions"]}
    final = {}
    for u in doc["units"]:
        row = {}
        for qid in qs:
            kv = u["questions"].get(qid, {}).get("verdict")
            if kv in ("pass", "fail"):
                row[qid] = {"verdict": kv, "by": "kev"}
                continue
            if qid in to_escalate(u, list(qs)):
                lv = verdict_from_llm(qs[qid], (llm.get(u["id"]) or {}).get(qid))
                row[qid] = {"verdict": lv, "by": "llm"} if lv else {"verdict": "pending", "by": "llm"}
            else:
                row[qid] = {"verdict": "pending", "by": "kev"}
        verdicts = {q: v["verdict"] if v["verdict"] in ("pass", "fail") else "uncertain" for q, v in row.items()}
        res = decide.unit_result(verdicts, qs, doc["mode"]) if verdicts else u["result"]
        final[u["id"]] = {"questions": row, "result": res}
    return final


def run_b(a, root, units, gate, gate_path) -> int:
    out = c.run_dir(a.set, "B", _label("B", a.engine, a.label), root)
    cdir = c.run_dir(a.set, "C", _label("C", a.engine, a.label), root)
    t0 = time.perf_counter()
    if (out / "decisions.json").exists() and not a.rerun:
        doc = c.read_json(out / "decisions.json")
        print("B: reusing stored decisions (use --rerun to ask the engine again)")
    elif (cdir / "decisions.json").exists() and not a.rerun:
        doc = c.read_json(cdir / "decisions.json")
        print("B: reusing C's decisions (same input, same engine)")
        write_decisions(out, doc)
        c.write_json(out / "meta.json", c.read_json(cdir / "meta.json") | {"arm": "B", "reused_from": cdir.name})
    else:
        doc = run_decisions(units, gate_path, make_engine(a.engine, a.answers), a.mode)
        write_decisions(out, doc)
        c.write_json(out / "meta.json", meta_for("B", a.engine, a.set, gate, doc, t0))
    by_id = {u["id"]: u for u in units}
    esc = [escalation_bundle(by_id, gate, r) for r in doc["units"] if to_escalate(r, [q["id"] for q in gate["questions"]])]
    c.write_json(out / "escalations.json", esc)
    n_q = sum(len(e["questions"]) for e in esc)
    print(f"B-{a.engine}: {len(esc)} unit(s) / {n_q} question(s) to escalate -> {out / 'escalations.json'}")
    if a.llm_answers:
        llm = c.read_json(a.llm_answers)
        c.write_json(out / "llm_answers.json", llm)
        final = apply_llm(doc, gate, llm)
        c.write_json(out / "final.json", final)
        pend = sum(1 for u in final.values() for q in u["questions"].values() if q["verdict"] == "pending" and q["by"] == "llm")
        print(f"B-{a.engine}: LLM answers ingested, {pend} escalated question(s) still unanswered")
    return 0


def run_a(a, root, units, gate) -> int:
    out = c.run_dir(a.set, "A", "llm", root)
    if a.emit_prompts:
        pdir = out / "raw" / "prompts"
        pdir.mkdir(parents=True, exist_ok=True)
        idx = {}
        for u in sorted(units, key=lambda u: u["id"]):
            txt = prompt_full_scan(u, gate["questions"])
            f = pdir / f"{u['id']}.md"
            f.write_text(txt, encoding="utf-8")
            idx[u["id"]] = {"file": str(f.relative_to(out)), "chars": len(txt), "tokens_est": c.est_tokens_text(txt)}
        c.write_json(out / "prompts_index.json", idx)
        c.write_json(out / "answers.template.json", {i: {"findings": [], "other": []} for i in idx})
        print(f"A: {len(idx)} bundle(s) -> {pdir}; answer file shape: {out / 'answers.template.json'}")
    if a.ingest:
        raw = c.read_json(a.ingest)
        known = {q["id"] for q in gate["questions"]}
        findings, bad = {}, []
        for uid, v in raw.items():
            f = [q for q in (v.get("findings") or []) if q in known]
            bad += [q for q in (v.get("findings") or []) if q not in known]
            findings[uid] = {"findings": sorted(set(f)), "other": [str(x) for x in (v.get("other") or [])]}
        c.write_json(out / "findings.json", findings)
        c.write_json(out / "meta.json", {"arm": "A", "engine": "llm", "set": a.set, "gate": gate["gate"],
                                         "gate_version": gate["version"], "units": len(findings),
                                         "unknown_question_ids": sorted(set(bad))})
        print(f"A: findings for {len(findings)} unit(s) -> {out / 'findings.json'}"
              + (f" (ignored unknown ids: {sorted(set(bad))})" if bad else ""))
    if not (a.emit_prompts or a.ingest):
        print("arm A needs --emit-prompts and/or --ingest FILE", file=sys.stderr)
        return 2
    return 0


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default=str(c.ROOT))
    ap.add_argument("--set", required=True)
    ap.add_argument("--arm", choices=["A", "B", "C"], required=True)
    ap.add_argument("--engine", choices=["mock", "kev"], default="mock")
    ap.add_argument("--label", help="suffix for the run dir, e.g. kev4b for the Kev-4B side arm")
    ap.add_argument("--answers", help="mock engine answers (mock_answers.py)")
    ap.add_argument("--gate")
    ap.add_argument("--mode", choices=["advisory", "enforce"], default="advisory")
    ap.add_argument("--llm-answers", help="B: escalated LLM verdicts {unit: {qid: pass|fail}}")
    ap.add_argument("--emit-prompts", action="store_true")
    ap.add_argument("--ingest", help="A: LLM findings {unit: {findings: [qid], other: [text]}}")
    ap.add_argument("--rerun", action="store_true", help="ask the engine again even if decisions are stored")
    a = ap.parse_args(argv)
    root = Path(a.root)
    units, labels = load_set(a.set, root)
    gate, gate_path = load_gate(root, a.gate or labels.get("gate") or c.DEFAULT_GATE)
    if a.arm == "C":
        return run_c(a, root, units, gate, gate_path)
    if a.arm == "B":
        return run_b(a, root, units, gate, gate_path)
    return run_a(a, root, units, gate)


if __name__ == "__main__":
    sys.exit(main())
