#!/usr/bin/env python3
"""Seed controlled defects into copies of clean units; write the labelled eval set.

  seed.py [--clean-dir DIR] [--seed S] [--per-defect K] [--set-id ID] [--gate gates/feature.v1.yaml] [--root DIR]

Output (per set):  metrics/kev-eval/sets/<set-id>/units.json   (decide.py `--units` format)
                   metrics/kev-eval/sets/<set-id>/labels.json  (ground truth + skipped transforms)
Every clean unit is kept as a negative; each defect type is applied to up to K applicable units
(shuffled by seed). A transform that cannot apply is skipped and the reason recorded.

Groups: `clean`; `seeded` (checklist defects: expect_fail = gate question ids that should fail);
`out_of_checklist` (expected: none; no question in the gate is designed to catch it).
`tolerated` = other questions that may legitimately also fail (not counted as false positives).
"""
from __future__ import annotations

import argparse
import ast
import random
import re
import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from kev_eval import common as c  # noqa: E402


class Skip(Exception):
    """Transform not applicable; message = why."""


# ----------------------------------------------------------- STEPS surgery ---
_HDR = re.compile(r"^# (\S+\.py)::(\w+)$", re.M)


def split_sections(steps: str) -> list[tuple[str, str]]:
    """[(header_line, source)]; sources are standalone function definitions."""
    ms = list(_HDR.finditer(steps))
    out = []
    for i, m in enumerate(ms):
        end = ms[i + 1].start() if i + 1 < len(ms) else len(steps)
        out.append((m.group(0), steps[m.end():end].strip("\n")))
    return out


def join_sections(secs: list[tuple[str, str]]) -> str:
    return "\n\n".join(f"{h}\n{s}" for h, s in secs)


def _is_then(fn) -> bool:
    for d in fn.decorator_list:
        if isinstance(d, ast.Call) and isinstance(d.func, (ast.Name, ast.Attribute)):
            nm = d.func.id if isinstance(d.func, ast.Name) else d.func.attr
            if nm == "then":
                return True
    return False


def _root_name(node) -> str:
    while True:
        if isinstance(node, ast.Call):
            node = node.func
        elif isinstance(node, ast.Attribute):
            if node.attr.startswith("assert_") or node.attr in ("called", "should"):
                return "assert"
            node = node.value
        elif isinstance(node, ast.Name):
            return node.id
        else:
            return ""


def _assert_stmts(fn) -> list:
    """Top-most assertion statements in fn: `assert ...`, `expect(...)...`, `x.assert_*()`."""
    found = []

    def visit(n):
        for ch in ast.iter_child_nodes(n):
            if isinstance(ch, ast.Assert):
                found.append(ch)
            elif isinstance(ch, ast.Expr) and isinstance(ch.value, ast.Call) and _root_name(ch.value.func) in ("expect", "assert"):
                found.append(ch)
            elif isinstance(ch, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)):
                continue
            else:
                visit(ch)

    visit(fn)
    return found


def _parse_fn(src: str):
    try:
        mod = ast.parse(src)
    except SyntaxError:
        return None
    fns = [n for n in mod.body if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))]
    return fns[0] if fns else None


def then_assertions_count(steps: str) -> int:
    n = 0
    for _, src in split_sections(steps):
        fn = _parse_fn(src)
        if fn is not None and _is_then(fn):
            n += len(_assert_stmts(fn))
    return n


def rewrite_then(steps: str, first_text: str | None, rest: str = "pass  # removed") -> tuple[str, int]:
    """In every @then function: first assertion statement -> first_text (None -> `rest`), others -> rest.
    Returns (new steps, number of assertion statements touched)."""
    secs, touched = [], 0
    for hdr, src in split_sections(steps):
        fn = _parse_fn(src)
        if fn is None or not _is_then(fn):
            secs.append((hdr, src))
            continue
        stmts = sorted(_assert_stmts(fn), key=lambda s: s.lineno)
        lines = src.splitlines()
        for i, st in reversed(list(enumerate(stmts))):
            indent = " " * st.col_offset
            txt = first_text if (i == 0 and first_text is not None) else rest
            lines[st.lineno - 1:st.end_lineno] = [indent + txt]
            touched += 1
        secs.append((hdr, "\n".join(lines)))
    return join_sections(secs), touched


def map_then_assert_text(steps: str, fn_text) -> tuple[str, int]:
    """Apply fn_text(line)->line to every assertion statement of every @then function; count changes."""
    secs, changed = [], 0
    for hdr, src in split_sections(steps):
        fn = _parse_fn(src)
        if fn is None or not _is_then(fn):
            secs.append((hdr, src))
            continue
        lines = src.splitlines()
        for st in sorted(_assert_stmts(fn), key=lambda s: -s.lineno):
            old = "\n".join(lines[st.lineno - 1:st.end_lineno])
            new = fn_text(old)
            if new != old:
                changed += 1
                lines[st.lineno - 1:st.end_lineno] = new.split("\n")
        secs.append((hdr, "\n".join(lines)))
    return join_sections(secs), changed


# -------------------------------------------------------------- transforms ---
_DENY = re.compile(r"\b(401|403|404|409|422|429|denied|forbidden|unauthori[sz]ed|reject(?:ed|s)?|cannot|can't|not allowed|"
                   r"invalid|error|fails?|refus(?:ed|es)|blocked)\b", re.I)
_PERM = re.compile(r"\b(401|403|denied|forbidden|unauthori[sz]ed|another user|other user|someone else|not the owner|"
                   r"owner|permission|scope|access|cross-user|IDOR)\b", re.I)
_LIMIT = re.compile(
    r"(\d{1,4})\s*(?:characters|chars|items|bytes|entries|results|rows|files|attempts|requests|MB|KB|minutes|days|hours|"
    r"words|digits|products|events|per page)\b|"
    r"(?:at most|at least|no more than|up to|maximum of|minimum of|more than|fewer than|exceeds?|limit of|limit)\D{0,15}(\d{1,4})\b",
    re.I)
_WORDMAP = [
    (r"\b(401|403|404|409|422|429|400)\b", "200"),
    (r"\bhttpx\.codes\.(?:UNAUTHORIZED|FORBIDDEN|NOT_FOUND|CONFLICT|BAD_REQUEST|UNPROCESSABLE_ENTITY|TOO_MANY_REQUESTS)\b", "httpx.codes.OK"),
    (r"\bdenied\b", "allowed"), (r"\bforbidden\b", "allowed"), (r"\bunauthori[sz]ed\b", "authorized"),
    (r"\brejected\b", "accepted"), (r"\brejects\b", "accepts"), (r"\bcannot\b", "can"), (r"\binvalid\b", "valid"),
    (r"\bnot allowed\b", "allowed"), (r"\bblocked\b", "allowed"), (r"\bfails\b", "succeeds"), (r"\bfailed\b", "succeeded"),
]


def _unit_text(u: dict) -> str:
    return "\n".join(u["parts"].get(k, "") for k in ("SPEC", "FEATURE", "STEPS"))


def _apply_wordmap(text: str) -> tuple[str, int]:
    n = 0
    for pat, rep in _WORDMAP:
        text, k = re.subn(pat, rep, text, flags=re.I)
        n += k
    return text, n


def t_proxy_status(u, rng):
    steps = u["parts"]["STEPS"]
    if then_assertions_count(steps) == 0:
        raise Skip("no assert/expect statement inside @then steps")
    ui = bool(re.search(r"\bexpect\(|world\.page|\.locator\(", steps))
    repl = 'expect(world.page.locator("body")).to_be_visible()' if ui else "assert resp.status_code == 200"
    new, n = rewrite_then(steps, repl)
    if all(len(re.findall(r"status_code|\.status\b|status_of|codes\.", ln)) and "==" in ln for ln in _asserts_text(steps)):
        raise Skip("clean unit already asserts only status codes")
    return {"STEPS": new}, f"{n} assertion statement(s) replaced by `{repl}`"


def _asserts_text(steps: str) -> list[str]:
    out = []
    for _, src in split_sections(steps):
        fn = _parse_fn(src)
        if fn is not None and _is_then(fn):
            lines = src.splitlines()
            for st in _assert_stmts(fn):
                out.append("\n".join(lines[st.lineno - 1:st.end_lineno]))
    return out


def t_mock_only(u, rng):
    steps = u["parts"]["STEPS"]
    if then_assertions_count(steps) == 0:
        raise Skip("no assert/expect statement inside @then steps")
    new, n = rewrite_then(steps, "mock_client.assert_called_once()")
    return {"STEPS": new}, f"{n} assertion statement(s) replaced by a mock-called check"


def t_removed_error(u, rng):
    txt = _unit_text(u)
    if not _DENY.search(u["parts"]["SPEC"] + "\n" + u["parts"]["FEATURE"]):
        raise Skip("scenario is not an error/permission case")
    if then_assertions_count(u["parts"]["STEPS"]) == 0:
        raise Skip("no assertion in @then steps to rewrite")

    def fn(line):
        return _apply_wordmap(line)[0]

    steps, ch = map_then_assert_text(u["parts"]["STEPS"], fn)
    if ch == 0:
        raise Skip("no denial/error literal inside the assertions")
    kind = "permission" if _PERM.search(u["parts"]["SPEC"] + u["parts"]["FEATURE"]) else "invalid_input"
    return {"STEPS": steps}, f"denial/error expectation in {ch} assertion(s) rewritten to the success path (scenario text unchanged)", {"variant": kind}


def t_removed_boundary(u, rng):
    spec = u["parts"]["SPEC"]
    n = None
    for m in _LIMIT.finditer(spec):
        k = int(m.group(1) or m.group(2))
        if k >= 3 and k not in _HTTP_CODES:
            n = k
            break
    if n is None:
        raise Skip("spec states no countable limit/range (HTTP status codes and values < 3 do not count)")
    mid = max(1, n // 2)
    pat = re.compile(rf"(?<![\w.]){n}(?![\w.])")
    feat, kf = pat.subn(str(mid), u["parts"]["FEATURE"])
    steps, ks = pat.subn(str(mid), u["parts"]["STEPS"])
    if kf + ks == 0:
        raise Skip(f"limit {n} does not appear in the feature or step text")
    return {"FEATURE": feat, "STEPS": steps}, f"boundary {n} replaced by mid-range {mid} ({kf + ks} occurrence(s))"


_HTTP_CODES = {100, 101, 200, 201, 202, 204, 301, 302, 304, 400, 401, 403, 404, 405, 409, 410, 412, 413, 415, 422, 429, 500, 501, 502, 503, 504}

_EXTRA = [
    '    if req.headers.get("x-debug") == "1":\n        return Response(200, store.dump_all())  # extra user-visible behaviour\n',
    '    if req.items and req.items[0].get("priority"):\n        store.notify_admins(record)  # extra side effect, no step exercises it\n',
    '    if req.query.get("export") == "csv":\n        return Response(200, to_csv(record))  # extra output format\n',
]


def t_code_beyond(u, rng):
    impl = u["parts"].get("IMPL")
    if not impl:
        raise Skip("unit has no IMPL hunk")
    extra = rng.choice(_EXTRA)
    new = impl.replace("    record = store.save(req)\n", "    record = store.save(req)\n" + extra)
    if new == impl:
        raise Skip("IMPL hunk has no insertion point")
    return {"IMPL": new}, "added an unexercised branch/side effect to IMPL"


def t_no_test(u, rng):
    steps = u["parts"]["STEPS"]
    secs, n = [], 0
    for hdr, src in split_sections(steps):
        fn = _parse_fn(src)
        if fn is not None and _is_then(fn):
            lines = src.splitlines()
            start = fn.body[0].lineno - 1
            doc = isinstance(fn.body[0], ast.Expr) and isinstance(getattr(fn.body[0], "value", None), ast.Constant)
            if doc and len(fn.body) > 1:
                start = fn.body[1].lineno - 1
            indent = " " * fn.body[0].col_offset
            lines[start:fn.end_lineno] = [indent + "pass  # TODO write the check"]
            src = "\n".join(lines)
            n += 1
        secs.append((hdr, src))
    if n == 0:
        raise Skip("no @then step to empty")
    return {"STEPS": join_sections(secs)}, f"{n} @then step body(ies) replaced by `pass`"


def t_wrong_field(u, rng):
    pat = re.compile(r"""(\[\s*["'])([A-Za-z_]\w{2,})(["']\s*\]|["']\s*\))|(\.get\(\s*["'])([A-Za-z_]\w{2,})(["'])""")

    def fn(line):
        done = [False]

        def rep(m):
            if done[0]:
                return m.group(0)
            done[0] = True
            if m.group(2):
                return f"{m.group(1)}{m.group(2)}_value{m.group(3)}"
            return f"{m.group(4)}{m.group(5)}_value{m.group(6)}"
        return pat.sub(rep, line)

    new, ch = map_then_assert_text(u["parts"]["STEPS"], fn)
    if ch == 0:
        raise Skip("no dict-key/`.get` access inside @then assertions")
    return {"STEPS": new}, f"{ch} assertion(s) now read a field name the spec never mentions"


def t_off_by_one(u, rng):
    impl = u["parts"].get("IMPL")
    if not impl:
        raise Skip("unit has no IMPL hunk")
    if not re.search(r"\b\d{1,4}\b", u["parts"]["SPEC"]):
        raise Skip("spec states no number, an off-by-one has no anchor")
    new = impl.replace("len(req.items) > MAX_ITEMS", "len(req.items) >= MAX_ITEMS")
    if new == impl:
        raise Skip("IMPL has no limit comparison")
    return {"IMPL": new}, "limit comparison `>` became `>=` in IMPL"


_LEAKS = [
    'ADMIN_TOKEN = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.c2lnbmF0dXJl"  # real-looking admin JWT',
    'TEST_EMAIL = "nguyen.van.a@gmail.com"  # real-looking personal email\nTEST_PHONE = "0912345678"',
    'DB_URL = "postgres://admin:Sup3rS3cret!@prod-db.internal:5432/app"  # real-looking credential',
]


def t_hardcoded_leak(u, rng):
    secs = split_sections(u["parts"]["STEPS"])
    if not secs:
        raise Skip("no step sections")
    leak = rng.choice(_LEAKS)
    hdr, src = secs[0]
    fn = _parse_fn(src)
    if fn is None:
        raise Skip("first step does not parse")
    lines = src.splitlines()
    first = min([d.lineno for d in fn.decorator_list] + [fn.lineno]) - 1
    lines[first:first] = leak.split("\n") + [""]
    secs[0] = (hdr, "\n".join(lines))
    return {"STEPS": join_sections(secs)}, "hardcoded secret/PII literal added to the step module"


# id, transform, group, standard id, standard ref, expected questions, tolerated
DEFECTS: dict[str, dict] = {
    "proxy_status_only": dict(fn=t_proxy_status, group="seeded", standard="MESZAROS-2007/proxy-assertion",
        ref="Meszaros xUnit Test Patterns 2007 (state vs behaviour verification); tsDetect assertion smells",
        expect=["assertion_kind"], tolerated=["promise_match"]),
    "mock_only": dict(fn=t_mock_only, group="seeded", standard="MESZAROS-2007/mock-only",
        ref="Meszaros xUnit Test Patterns 2007 (behaviour verification only); tsDetect",
        expect=["assertion_kind"], tolerated=["promise_match", "sut_not_bypassed"]),
    "removed_error_case": dict(fn=t_removed_error, group="seeded", standard="ISO29119-4/error-guessing+ASVS-4.0.3/V4",
        ref="ISO/IEC/IEEE 29119-4 error guessing; OWASP ASVS 4.0.3 V4 access control",
        expect=None, tolerated=["promise_match", "assertion_kind", "invalid_partition_assert", "permission_probe"]),
    "removed_boundary_case": dict(fn=t_removed_boundary, group="seeded", standard="ISO29119-4/BVA",
        ref="ISO/IEC/IEEE 29119-4 boundary value analysis", expect=["boundary_values"], tolerated=["promise_match"]),
    "code_beyond_tests": dict(fn=t_code_beyond, group="seeded", standard="GOOGLE-ENG/complexity",
        ref="Google Engineering Practices - code review: complexity / tests",
        expect=["code_beyond_tests"], tolerated=[]),
    "scenario_no_test": dict(fn=t_no_test, group="seeded", standard="ISO29119-4/scenario-testing",
        ref="ISO/IEC/IEEE 29119-4 scenario testing (every scenario needs a test that checks it)",
        expect=["assertion_kind", "promise_match"], tolerated=[]),
    "wrong_field_assertion": dict(fn=t_wrong_field, group="out_of_checklist", standard="none",
        ref="out of checklist: assertion reads a field the spec does not name", expect=[], tolerated=["promise_match"]),
    "off_by_one_impl": dict(fn=t_off_by_one, group="out_of_checklist", standard="none",
        ref="out of checklist: off-by-one in the implementation limit", expect=[], tolerated=[]),
    "hardcoded_test_data_leak": dict(fn=t_hardcoded_leak, group="out_of_checklist", standard="none",
        ref="out of checklist: secret/PII literal in test code (a secrets scanner's job)", expect=[],
        tolerated=["mystery_guest"]),
}


def seed(clean: list[dict], seed_: int, per_defect: int, gate: dict | None = None) -> tuple[list[dict], dict]:
    """-> (units, labels). Pure function (no I/O): used by the CLI and the tests."""
    qids = {q["id"] for q in (gate or {}).get("questions", [])}
    units, lab_units, skipped = [], {}, []
    order = sorted(clean, key=lambda u: u["id"])
    for u in order:
        units.append({"id": u["id"], "parts": u["parts"], "paths": u.get("paths", [])})
        lab_units[u["id"]] = {"base": u["id"], "group": "clean", "defect": None, "standard": None,
                              "expect_fail": [], "tolerated": [], "expected": "pass", "change": u.get("change")}
    counts = {d: 0 for d in DEFECTS}
    for did, spec in DEFECTS.items():
        rng = random.Random(c.stable_hash(str(seed_), did))
        pool = order[:]
        rng.shuffle(pool)
        for u in pool:
            if counts[did] >= per_defect:
                break
            r = random.Random(c.stable_hash(str(seed_), did, u["id"]))
            try:
                out = spec["fn"](u, r)
            except Skip as e:
                skipped.append({"base": u["id"], "defect": did, "reason": str(e)})
                continue
            patch, detail = out[0], out[1]
            extra = out[2] if len(out) > 2 else {}
            expect = spec["expect"]
            if expect is None:  # removed_error_case: permission vs invalid input
                expect = ["permission_probe"] if extra.get("variant") == "permission" else ["invalid_partition_assert"]
            if qids:
                expect = [q for q in expect if q in qids]
            uid = f"{u['id']}~{did}"
            parts = {**u["parts"], **patch}
            units.append({"id": uid, "parts": parts, "paths": u.get("paths", [])})
            lab_units[uid] = {"base": u["id"], "group": spec["group"], "defect": did, "standard": spec["standard"],
                              "standard_ref": spec["ref"], "expect_fail": sorted(expect),
                              "tolerated": sorted(set(spec["tolerated"]) - set(expect)),
                              "expected": "none" if spec["group"] == "out_of_checklist" else "fail",
                              "detail": detail, "change": u.get("change")}
            counts[did] += 1
    labels = {"seed": seed_, "per_defect": per_defect, "units": lab_units, "skipped": skipped,
              "counts": {"clean": len(order), **counts, "units_total": len(units), "skipped": len(skipped)},
              "gate_questions": sorted(qids)}
    return sorted(units, key=lambda x: x["id"]), labels


def write_set(set_id: str, units: list[dict], labels: dict, root: Path = c.ROOT) -> Path:
    d = c.set_dir(set_id, root)
    labels = {**labels, "set_id": set_id}
    c.write_json(d / "units.json", units)
    c.write_json(d / "labels.json", labels)
    return d


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", default=str(c.ROOT))
    ap.add_argument("--clean-dir")
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--per-defect", type=int, default=5)
    ap.add_argument("--set-id")
    ap.add_argument("--gate", default=c.DEFAULT_GATE)
    a = ap.parse_args(argv)
    root = Path(a.root)
    from kev_eval.build_set import load_clean
    clean = load_clean(Path(a.clean_dir) if a.clean_dir else c.eval_dir(root) / "states" / "clean")
    if not clean:
        print("no clean units: run build_set.py first", file=sys.stderr)
        return 2
    gp = Path(a.gate)
    gate = yaml.safe_load((gp if gp.is_absolute() else root / gp).read_text(encoding="utf-8")) if (gp if gp.is_absolute() else root / gp).exists() else None
    units, labels = seed(clean, a.seed, a.per_defect, gate)
    set_id = a.set_id or f"s{a.seed}-n{len(clean)}"
    labels["gate"] = a.gate
    d = write_set(set_id, units, labels, root)
    import json
    print(json.dumps({"set_id": set_id, "dir": str(d), **labels["counts"]}, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
