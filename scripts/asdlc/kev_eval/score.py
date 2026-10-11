#!/usr/bin/env python3
"""Score the A/B/C arms against the seeded ground truth and suggest per-question thresholds.

  score.py --set ID [--engine mock|kev|<label>] [--gate G] [--max-miss 0.05] [--max-false-fail 0.05]
Reads  metrics/kev-eval/sets/<set>/labels.json and the runs under metrics/kev-eval/runs/<set>/
Writes metrics/kev-eval/<set>/report.md + report.json

Truth for question q on a unit: positive iff q in labels.expect_fail. A flag on a `tolerated` question is ignored
(neither TP nor FP). Out-of-checklist units have no positive question: any non-tolerated flag there is a false alarm,
and "caught" counts flags on tolerated questions (or, for A, free-text `other` findings).
"""
from __future__ import annotations

import argparse
import json
import statistics
import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from kev_eval import common as c  # noqa: E402


# --------------------------------------------------------------- loading ---
def load_preds(set_id: str, engine: str, root: Path, gate: dict) -> dict:
    """-> {arm: {"pred": {unit: {flag, uncertain, pending, other}}, "meta": {...}, "dir": Path}}"""
    arms = {}
    qids = [q["id"] for q in gate["questions"]]
    cdir = c.run_dir(set_id, "C", engine, root)
    if (cdir / "decisions.json").exists():
        doc = c.read_json(cdir / "decisions.json")
        pred = {}
        for u in doc["units"]:
            if u["result"] == "ENGINE_DOWN" or not u["questions"]:
                continue
            qs = u["questions"]
            pred[u["id"]] = {"flag": {q for q, v in qs.items() if v["verdict"] == "fail"},
                             "uncertain": {q for q, v in qs.items() if v["verdict"] == "uncertain"},
                             "pending": set(), "other": []}
        arms["C"] = {"pred": pred, "doc": doc, "dir": cdir, "meta": _meta(cdir)}
    bdir = c.run_dir(set_id, "B", engine, root)
    if (bdir / "decisions.json").exists():
        doc = c.read_json(bdir / "decisions.json")
        esc = c.read_json(bdir / "escalations.json") if (bdir / "escalations.json").exists() else []
        final = c.read_json(bdir / "final.json") if (bdir / "final.json").exists() else None
        pred = {}
        for u in doc["units"]:
            if u["result"] == "ENGINE_DOWN" and not final:
                continue
            qs = u["questions"]
            if not qs and not (final and u["id"] in final):
                continue  # whole-unit escalation with no final.json entry: not scored (as arm C)
            flag = {q for q, v in qs.items() if v["verdict"] == "fail"}
            pend = {q for q, v in qs.items() if v["verdict"] == "uncertain"}
            if final and u["id"] in final:
                fq = final[u["id"]]["questions"]
                flag = {q for q, v in fq.items() if v["verdict"] == "fail"}
                pend = {q for q, v in fq.items() if v["verdict"] == "pending"}
            pred[u["id"]] = {"flag": flag, "uncertain": set(), "pending": pend, "other": []}
        arms["B"] = {"pred": pred, "doc": doc, "dir": bdir, "meta": _meta(bdir), "escalations": esc,
                     "llm_ingested": final is not None}
    adir = c.run_dir(set_id, "A", "llm", root)
    if (adir / "findings.json").exists():
        f = c.read_json(adir / "findings.json")
        pred = {u: {"flag": set(v["findings"]), "uncertain": set(), "pending": set(), "other": v.get("other", [])}
                for u, v in f.items()}
        idx = c.read_json(adir / "prompts_index.json") if (adir / "prompts_index.json").exists() else {}
        arms["A"] = {"pred": pred, "dir": adir, "meta": _meta(adir), "prompts": idx}
    return arms


def _meta(d: Path) -> dict:
    return c.read_json(d / "meta.json") if (d / "meta.json").exists() else {}


# -------------------------------------------------------------- metrics ---
def prf(tp: int, fp: int, fn: int) -> dict:
    p = tp / (tp + fp) if tp + fp else None
    r = tp / (tp + fn) if tp + fn else None
    f1 = 2 * p * r / (p + r) if p and r else (0.0 if (p is not None and r is not None) else None)
    return {"tp": tp, "fp": fp, "fn": fn, "precision": p, "recall": r, "f1": f1}


def per_question(pred: dict, labels: dict, qids: list[str]) -> dict:
    out = {}
    for q in qids:
        tp = fp = fn = 0
        for uid, p in pred.items():
            m = labels["units"].get(uid)
            if m is None:
                continue
            pos = q in m["expect_fail"]
            flagged = q in p["flag"]
            if pos:
                tp, fn = tp + flagged, fn + (not flagged)
            elif flagged and q not in m["tolerated"]:
                fp += 1
        out[q] = prf(tp, fp, fn)
    return out


def per_standard(pred: dict, labels: dict) -> dict:
    stds: dict[str, set] = {}
    for m in labels["units"].values():
        if m["group"] == "seeded":
            stds.setdefault(m["standard"], set()).update(m["expect_fail"])
    out = {}
    for std, qs in sorted(stds.items()):
        tp = fp = fn = 0
        for uid, p in pred.items():
            m = labels["units"].get(uid)
            if m is None:
                continue
            hit = bool(p["flag"] & qs)
            if m["group"] == "seeded" and m["standard"] == std:
                tp, fn = tp + hit, fn + (not hit)
            elif hit and not (set(m["expect_fail"]) & qs) and not (p["flag"] & qs) <= set(m["tolerated"]):
                fp += 1
        r = prf(tp, fp, fn)
        r["questions"] = sorted(qs)
        r["defects"] = sorted({m["defect"] for m in labels["units"].values() if m["standard"] == std})
        out[std] = r
    return out


def per_defect(pred: dict, labels: dict) -> dict:
    out = {}
    for did in sorted({m["defect"] for m in labels["units"].values() if m["defect"]}):
        ids = [u for u, m in labels["units"].items() if m["defect"] == did and u in pred]
        if not ids:
            continue
        grp = labels["units"][ids[0]]["group"]
        if grp == "seeded":
            hit = sum(1 for u in ids if pred[u]["flag"] & set(labels["units"][u]["expect_fail"]))
            out[did] = {"group": grp, "n": len(ids), "caught": hit, "rate": hit / len(ids)}
        else:
            tol = sum(1 for u in ids if (pred[u]["flag"] & set(labels["units"][u]["tolerated"])) or pred[u]["other"])
            alarm = sum(1 for u in ids if pred[u]["flag"] - set(labels["units"][u]["tolerated"]))
            out[did] = {"group": grp, "n": len(ids), "caught": tol, "rate": tol / len(ids),
                        "any_flag": sum(1 for u in ids if pred[u]["flag"] or pred[u]["other"]) / len(ids),
                        "false_alarm": alarm / len(ids)}
    return out


def unit_level(pred: dict, labels: dict) -> dict:
    """Any flag at all vs 'unit has a seeded in-checklist defect'."""
    tp = fp = fn = 0
    for uid, p in pred.items():
        m = labels["units"].get(uid)
        if not m or m["group"] == "out_of_checklist":
            continue
        pos = m["group"] == "seeded"
        flagged = bool(p["flag"] - ({q for q in m["tolerated"]} if not pos else set()))
        if pos:
            hit = bool(p["flag"] & set(m["expect_fail"]))
            tp, fn = tp + hit, fn + (not hit)
        elif flagged:
            fp += 1
    return prf(tp, fp, fn)


def out_of_checklist(pred: dict, labels: dict) -> dict:
    ids = [u for u, m in labels["units"].items() if m["group"] == "out_of_checklist" and u in pred]
    if not ids:
        return {"n": 0, "catch_rate": None, "false_alarm_rate": None}
    caught = sum(1 for u in ids if (pred[u]["flag"] & set(labels["units"][u]["tolerated"])) or pred[u]["other"])
    alarm = sum(1 for u in ids if pred[u]["flag"] - set(labels["units"][u]["tolerated"]))
    return {"n": len(ids), "catch_rate": caught / len(ids), "false_alarm_rate": alarm / len(ids),
            "any_flag_rate": sum(1 for u in ids if pred[u]["flag"] or pred[u]["other"]) / len(ids)}


# ----------------------------------------------------------- threshold sweep ---
def _decide_side(q: dict, raw) -> tuple[str, float] | None:
    """-> (side 'pass'|'fail', strength in [0.5,1]) or None. strength is what the threshold is compared to."""
    if not isinstance(raw, dict):
        return None
    try:
        if q["type"] == "noul":
            p_pass = float(raw["yes"]) if q.get("pass_when", True) else 1 - float(raw["yes"])
            return ("pass", p_pass) if p_pass >= 0.5 else ("fail", 1 - p_pass)
        conf = float(raw["confidence"])
        if q["type"] == "choice":
            return ("pass" if raw["choice"] in q["pass_values"] else "fail"), conf
        names = list(q["levels"])
        idx = raw["score"] if isinstance(raw.get("score"), int) else names.index(raw["level"])
        return ("pass" if idx >= names.index(q["pass_min"]) else "fail"), conf
    except (KeyError, TypeError, ValueError):
        return None


def grid_for(q: dict) -> list[float]:
    if q["type"] == "noul":
        return [round(0.5 + 0.01 * i, 2) for i in range(50)] + [0.995]
    return [round(0.01 * i, 2) for i in range(100)]


def sweep(q: dict, raws: dict, labels: dict, max_miss: float, max_ff: float) -> dict:
    """raws: {unit: raw answer}. Threshold t = `threshold` (noul) or `min_confidence` (choice/score)."""
    pts = []
    for uid, raw in raws.items():
        m = labels["units"].get(uid)
        sd = _decide_side(q, raw)
        if m is None or sd is None:
            continue
        pos = q["id"] in m["expect_fail"]
        if not pos and q["id"] in m["tolerated"]:
            continue
        pts.append((pos, sd[0], sd[1]))
    npos = sum(1 for p in pts if p[0])
    nneg = len(pts) - npos
    key = "threshold" if q["type"] == "noul" else "min_confidence"
    curve = []
    for t in grid_for(q):
        tp = fp = miss = esc = fn_unc = 0
        for pos, side, strength in pts:
            decided = strength >= t
            if not decided:
                esc += 1
                fn_unc += pos
            elif side == "fail":
                tp, fp = tp + pos, fp + (not pos)
            else:
                miss += pos
        curve.append({key: t, "recall": tp / npos if npos else None,
                      "precision": tp / (tp + fp) if tp + fp else None,
                      "miss_rate": miss / npos if npos else None,
                      "false_fail_rate": fp / nneg if nneg else None,
                      "escalation_rate": esc / len(pts) if pts else None,
                      "recall_with_ideal_llm": (tp + fn_unc) / npos if npos else None})
    cur = q.get("threshold") if q["type"] == "noul" else q.get("min_confidence")
    res = {"key": key, "current": cur, "positives": npos, "negatives": nneg, "curve": curve,
           "suggested": None, "feasible": None}
    if npos == 0 or not pts:
        res["note"] = "no positive examples in the set for this question: keep the current value"
        return res
    ok = [p for p in curve if (p["miss_rate"] or 0) <= max_miss and (p["false_fail_rate"] or 0) <= max_ff]
    if ok:
        best = min(ok, key=lambda p: (p["escalation_rate"], -p[key]))  # ties: most conservative (highest) value
        res["feasible"] = True
    else:
        best = min(curve, key=lambda p: ((p["miss_rate"] or 0) + (p["false_fail_rate"] or 0), p["escalation_rate"]))
        res["feasible"] = False
    res["suggested"] = best[key]
    res["at_suggested"] = best
    return res


def verdict_for(sw: dict, min_pos: int = 3) -> str:
    if sw["positives"] < min_pos:
        return "insufficient positives"
    if not sw["feasible"]:
        return "keep LLM"
    esc = sw["at_suggested"]["escalation_rate"]
    return "Kev (candidate for enforce)" if esc <= 0.25 else "Kev + LLM escalation"


# ------------------------------------------------------------------ report ---
def pct(x) -> str:
    return "n/a" if x is None else f"{x * 100:.0f}%"


def fmt(x) -> str:
    return "n/a" if x is None else f"{x:.2f}"


def latency(arm: dict) -> dict:
    t = [u["timing_ms"] for u in arm.get("doc", {}).get("units", []) if u.get("timing_ms") is not None]
    if not t:
        return {"calls": 0}
    t.sort()
    return {"calls": len(t), "total_s": round(sum(t) / 1000, 2), "mean_ms": round(statistics.mean(t), 1),
            "p50_ms": round(statistics.median(t), 1), "p95_ms": round(t[min(len(t) - 1, int(0.95 * len(t)))], 1),
            "gpu_hours_est": round(sum(t) / 3.6e6, 4)}


def build_report(set_id: str, engine: str, root: Path, gate: dict, max_miss: float, max_ff: float) -> dict:
    labels = c.read_json(c.set_dir(set_id, root) / "labels.json")
    qs = {q["id"]: q for q in gate["questions"]}
    qids = list(qs)
    arms = load_preds(set_id, engine, root, gate)
    rep = {"set": set_id, "engine": engine, "gate": gate["gate"], "gate_version": gate["version"],
           "counts": labels["counts"], "arms": {}, "thresholds": {}, "conclusions": {}}
    for name, arm in arms.items():
        pred = arm["pred"]
        r = {"units_scored": len(pred), "per_question": per_question(pred, labels, qids),
             "per_standard": per_standard(pred, labels), "per_defect": per_defect(pred, labels),
             "unit_level": unit_level(pred, labels), "out_of_checklist": out_of_checklist(pred, labels)}
        if name == "C":
            r["uncertain_rate"] = (sum(len(p["uncertain"]) for p in pred.values()) / (len(pred) * len(qids))) if pred else None
            r["latency"] = latency(arm)
            r["llm_tokens_est"] = 0
        if name == "B":
            esc = arm.get("escalations", [])
            n_eval = len(pred)
            r["escalation"] = {"units": len(esc), "unit_rate": len(esc) / n_eval if n_eval else None,
                               "questions": sum(len(e["questions"]) for e in esc),
                               "question_rate": sum(len(e["questions"]) for e in esc) / (n_eval * len(qids)) if n_eval else None}
            r["llm_answers_ingested"] = arm["llm_ingested"]
            r["pending_questions"] = sum(len(p["pending"]) for p in pred.values())
            r["llm_tokens_est"] = sum(e["tokens_est"] for e in esc)
            r["latency"] = latency(arm)
        if name == "A":
            idx = arm.get("prompts", {})
            r["llm_tokens_est"] = sum(v["tokens_est"] for k, v in idx.items() if k in pred) if idx else None
            r["other_findings"] = sum(1 for p in pred.values() if p["other"])
        rep["arms"][name] = r
    if "A" in rep["arms"] and "B" in rep["arms"] and rep["arms"]["A"].get("llm_tokens_est"):
        rep["arms"]["B"]["llm_tokens_vs_A"] = rep["arms"]["B"]["llm_tokens_est"] / rep["arms"]["A"]["llm_tokens_est"]
    # thresholds from raw Kev answers
    src = arms.get("C") or arms.get("B")
    if src:
        raw = c.read_json(src["dir"] / "raw" / "answers.json")
        for q in qids:
            sw = sweep(qs[q], {u: a.get(q) for u, a in raw.items()}, labels, max_miss, max_ff)
            rep["thresholds"][q] = sw
            rep["conclusions"][q] = verdict_for(sw)
    return rep


def to_markdown(rep: dict) -> str:
    L = [f"# Kev eval report: set `{rep['set']}`, engine `{rep['engine']}`, gate `{rep['gate']}` v{rep['gate_version']}", "",
         f"Units: {json.dumps(rep['counts'], sort_keys=True)}", ""]
    arms = rep["arms"]
    names = [a for a in ("A", "B", "C") if a in arms]
    if not names:
        return "\n".join(L + ["No runs found.", ""])
    L += ["## Per question (recall / precision / F1)", "",
          "| Question | " + " | ".join(f"{a} R / P / F1" for a in names) + " |", "|---|" + "---|" * len(names)]
    for q in rep["thresholds"] or next(iter(arms.values()))["per_question"]:
        row = []
        for a in names:
            m = arms[a]["per_question"].get(q, {})
            row.append(f"{pct(m.get('recall'))} / {pct(m.get('precision'))} / {fmt(m.get('f1'))} (n+={m.get('tp', 0) + m.get('fn', 0)})")
        L.append(f"| `{q}` | " + " | ".join(row) + " |")
    L += ["", "## Per standard ID (unit recall / precision)", "",
          "| Standard | Questions | " + " | ".join(a for a in names) + " |", "|---|---|" + "---|" * len(names)]
    stds = sorted({s for a in names for s in arms[a]["per_standard"]})
    for s in stds:
        qs_ = next((arms[a]["per_standard"][s]["questions"] for a in names if s in arms[a]["per_standard"]), [])
        L.append(f"| {s} | {', '.join(qs_)} | " + " | ".join(
            f"{pct(arms[a]['per_standard'].get(s, {}).get('recall'))} / {pct(arms[a]['per_standard'].get(s, {}).get('precision'))}"
            for a in names) + " |")
    L += ["", "## Seeded defects caught, by defect type", "", "| Defect | Group | " + " | ".join(names) + " |", "|---|---|" + "---|" * len(names)]
    dids = sorted({d for a in names for d in arms[a]["per_defect"]})
    for d in dids:
        grp = next(arms[a]["per_defect"][d]["group"] for a in names if d in arms[a]["per_defect"])
        L.append(f"| {d} | {grp} | " + " | ".join(
            (f"{arms[a]['per_defect'][d]['caught']}/{arms[a]['per_defect'][d]['n']}" if d in arms[a]["per_defect"] else "n/a")
            for a in names) + " |")
    L += ["", "## Out-of-checklist group (expected: none)", "", "| Arm | n | caught (tolerated flag / `other`) | false alarm | any flag |", "|---|---|---|---|---|"]
    for a in names:
        o = arms[a]["out_of_checklist"]
        L.append(f"| {a} | {o['n']} | {pct(o['catch_rate'])} | {pct(o['false_alarm_rate'])} | {pct(o.get('any_flag_rate'))} |")
    L += ["", "## Cost and speed", ""]
    if "C" in arms:
        lt = arms["C"]["latency"]
        L.append(f"- Kev (C): {lt.get('calls', 0)} call(s), total {lt.get('total_s', 'n/a')} s, mean {lt.get('mean_ms', 'n/a')} ms, "
                 f"p95 {lt.get('p95_ms', 'n/a')} ms, ~{lt.get('gpu_hours_est', 'n/a')} GPU-h; uncertain answers {pct(arms['C'].get('uncertain_rate'))}; LLM tokens 0.")
    if "B" in arms:
        e = arms["B"]["escalation"]
        L.append(f"- B escalation: {e['units']} unit(s) = {pct(e['unit_rate'])} of units, {e['questions']} question(s) = {pct(e['question_rate'])}; "
                 f"LLM tokens ~{arms['B']['llm_tokens_est']}" + (f" ({pct(arms['B'].get('llm_tokens_vs_A'))} of A)" if "llm_tokens_vs_A" in arms["B"] else "")
                 + ("" if arms["B"]["llm_answers_ingested"] else "; LLM answers NOT ingested yet, B == Kev-only"))
    if "A" in arms:
        L.append(f"- A: LLM tokens ~{arms['A']['llm_tokens_est']} (chars/4 of the bundles); free-text `other` findings on {arms['A']['other_findings']} unit(s).")
    L += ["", "## Thresholds from the precision/recall sweep (Kev raw answers)", "",
          f"Constraint: silent miss <= max_miss and false fail <= max_false_fail; minimise escalation. `current` is the value in the gate file.", "",
          "| Question | key | current | suggested | feasible | miss | false fail | escalation | recall+ideal LLM | Verdict |",
          "|---|---|---|---|---|---|---|---|---|---|"]
    for q, sw in rep["thresholds"].items():
        at = sw.get("at_suggested") or {}
        L.append(f"| `{q}` | {sw['key']} | {sw['current']} | {sw['suggested'] if sw['suggested'] is not None else 'keep'} | "
                 f"{sw['feasible']} | {pct(at.get('miss_rate'))} | {pct(at.get('false_fail_rate'))} | {pct(at.get('escalation_rate'))} | "
                 f"{pct(at.get('recall_with_ideal_llm'))} | {rep['conclusions'][q]} |")
    L += ["", "Verdict rule: >= 3 positives; feasible and escalation <= 25% -> Kev; feasible otherwise -> Kev + LLM escalation; infeasible -> keep LLM.", ""]
    return "\n".join(L)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default=str(c.ROOT))
    ap.add_argument("--set", required=True)
    ap.add_argument("--engine", default="kev")
    ap.add_argument("--gate")
    ap.add_argument("--max-miss", type=float, default=0.05)
    ap.add_argument("--max-false-fail", type=float, default=0.05)
    a = ap.parse_args(argv)
    root = Path(a.root)
    labels = c.read_json(c.set_dir(a.set, root) / "labels.json")
    gp = Path(a.gate or labels.get("gate") or c.DEFAULT_GATE)
    gate = yaml.safe_load((gp if gp.is_absolute() else root / gp).read_text(encoding="utf-8"))
    rep = build_report(a.set, a.engine, root, gate, a.max_miss, a.max_false_fail)
    out = c.eval_dir(root) / a.set
    c.write_json(out / "report.json", rep)
    (out / "report.md").write_text(to_markdown(rep), encoding="utf-8")
    print(f"report -> {out / 'report.md'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
