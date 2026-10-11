#!/usr/bin/env python3
"""spec_lint: lint an OpenSpec change's delta (specs/** + proposal.md) -- never openspec/specs.

Usage: spec_lint.py <change-dir-or-id> [...] [--json] [--warn-only] [--legacy]

Rules (id, severity, source):
  SPEC-none            warning  change has no delta spec files
  REQ-scenario         error    ISO 29148 verifiable: every Requirement has >=1 Scenario
  SCN-when / SCN-then  error    EARS trigger/response: every Scenario has WHEN and THEN
  EARS-shall           error    EARS: requirement text uses SHALL/MUST
  THEN-vague           warning  ISO 29148 unambiguous: THEN has vague words / "should"
  ISO29148-singular    warning  ISO 29148 singular: one behaviour per THEN (two SHALLs joined by "and")
  ERR-scenario         error    ADDED requirement has >=1 error/negative scenario (ISO 29119-4 error guessing)
  PERM-scenario        warning  requirement about a user-owned resource has a permission scenario (OWASP ASVS V4)
  PROPOSAL-sections    error    proposal has NFR, Data classification, Threat model sections
                                (warning with --legacy, for changes created before the template)
Heuristic keyword sets are the *_RE constants below.
Exit 1 if any error (0 with --warn-only).
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass, asdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

VAGUE_RE = re.compile(
    r"\b(properly|correctly|appropriately|fast|quickly|user[- ]friendly|etc\.?|and/or|as needed|"
    r"seamless(ly)?|gracefully|reasonabl[ey]|efficient(ly)?)(?=\W|$)", re.I)
SHOULD_RE = re.compile(r"\bshould\b", re.I)
SHALL_RE = re.compile(r"\b(SHALL|MUST)\b")
TWO_SHALL_RE = re.compile(r"\b(shall|must)\b[^.;\n]*\band\b[^.;\n]*\b(shall|must)\b", re.I)
# error / negative scenario: heuristic keywords in scenario title/WHEN/THEN
NEG_RE = re.compile(
    r"\b(reject(s|ed)?|error|invalid|fail(s|ed|ure)?|den(y|ies|ied)|forbidden|unauthori[sz]ed|"
    r"missing|malformed|exceed(s|ed)?|not found|conflict|timeout|times? out|unavailable|"
    r"rolled back|rollback|blocked|refus(e|es|ed)|without|empty|expired|"
    r"4\d\d|5\d\d|unauthenticated|"
    r"(?:does|do|is|are|was|were|must|shall|will|can|could) not|never|no longer|cannot|can't)\b", re.I)
# requirement mentions an actor-owned resource -> needs a permission scenario
ACTOR_RE = re.compile(
    r"\b(user|users|seller|sellers|buyer|buyers|admin|admins|owner|owners|principal|account|customer)s?\b", re.I)
PERM_RE = re.compile(
    r"\b(403|401|forbidden|unauthori[sz]ed|unauthenticated|permission|scope|not (?:the )?owner|"
    r"another (?:user|seller|buyer)|other (?:user|seller|buyer)|someone else|ownership|"
    r"denied|rbac|role|anonymous|not logged in|without (?:a )?(?:token|login|auth\w*))\b", re.I)

SECTION_PATTERNS = {
    "Non-functional requirements": re.compile(r"non[- ]?functional|\bnfrs?\b", re.I),
    "Data classification": re.compile(r"data[- ]classification|classification of data", re.I),
    "Threat model": re.compile(r"threat[- ]model", re.I),
}


@dataclass
class Finding:
    path: str
    line: int
    rule: str
    severity: str
    source: str
    message: str


SOURCES = {
    "SPEC-none": "OpenSpec delta layout",
    "REQ-scenario": "ISO29148-verifiable",
    "SCN-when": "EARS-1",
    "SCN-then": "EARS-1",
    "EARS-shall": "EARS-1",
    "THEN-vague": "ISO29148-unambiguous",
    "ISO29148-singular": "ISO29148-singular",
    "ERR-scenario": "ISO29119-4-error-guessing",
    "PERM-scenario": "OWASP-ASVS-V4",
    "PROPOSAL-sections": "ISO29148-complete",
}


def rel(p: Path) -> str:
    try:
        return str(p.resolve().relative_to(ROOT))
    except ValueError:
        return str(p)


def resolve_change(arg: str) -> Path | None:
    p = Path(arg)
    if p.is_dir():
        return p
    base = ROOT / "openspec" / "changes"
    for cand in (base / arg, ):
        if cand.is_dir():
            return cand
    hits = sorted((base / "archive").glob(f"*-{arg}")) + sorted((base / "archive").glob(arg))
    return hits[0] if hits else None


@dataclass
class Scenario:
    title: str
    line: int
    when: list
    then: list
    text: str


@dataclass
class Requirement:
    title: str
    line: int
    section: str
    body: str
    scenarios: list


BULLET_RE = re.compile(r"^\s*[-*]\s+\**\s*(WHEN|THEN|AND|GIVEN|BUT)\b\**:?\s*(.*)$", re.I)


def parse_spec(text: str) -> list[Requirement]:
    reqs: list[Requirement] = []
    section = "ADDED"
    cur_req = cur_sc = None
    last_kind = None
    body: list[str] = []
    in_fence = False

    def close_req():
        if cur_req is not None:
            cur_req.body = "\n".join(body)

    for i, raw in enumerate(text.splitlines(), 1):
        if raw.strip().startswith("```"):
            in_fence = not in_fence
        if in_fence:
            continue
        m = re.match(r"^##\s+(ADDED|MODIFIED|REMOVED|RENAMED)\b", raw, re.I)
        if m:
            close_req(); cur_req = cur_sc = None; body = []
            section = m.group(1).upper()
            continue
        m = re.match(r"^###\s+Requirement:\s*(.*)$", raw)
        if m:
            close_req(); body = []
            cur_req = Requirement(m.group(1).strip(), i, section, "", [])
            reqs.append(cur_req); cur_sc = None; last_kind = None
            continue
        m = re.match(r"^####\s+Scenario:\s*(.*)$", raw)
        if m and cur_req is not None:
            cur_sc = Scenario(m.group(1).strip(), i, [], [], "")
            cur_req.scenarios.append(cur_sc); last_kind = None
            continue
        if re.match(r"^#{1,3}\s", raw) and not raw.startswith("###" + " Requirement"):
            cur_sc = None
            continue
        if cur_sc is not None:
            b = BULLET_RE.match(raw)
            if b:
                kind = b.group(1).upper()
                last_kind = "when" if kind in ("WHEN", "GIVEN") else ("then" if kind == "THEN" else last_kind)
                if kind == "AND" and last_kind is None:
                    last_kind = "when"
                tgt = cur_sc.when if last_kind == "when" else cur_sc.then
                tgt.append((i, b.group(2)))
            elif raw.strip() and last_kind and raw.startswith((" ", "\t")):
                tgt = cur_sc.when if last_kind == "when" else cur_sc.then
                if tgt:
                    ln, t = tgt[-1]
                    tgt[-1] = (ln, t + " " + raw.strip())
            cur_sc.text += raw + "\n"
        elif cur_req is not None:
            body.append(raw)
    close_req()
    return reqs


def lint_spec(path: Path) -> list[Finding]:
    out: list[Finding] = []
    p = rel(path)

    def add(line, rule, sev, msg):
        out.append(Finding(p, line, rule, sev, SOURCES[rule], msg))

    for r in parse_spec(path.read_text()):
        if r.section in ("REMOVED", "RENAMED"):
            continue
        name = f'Requirement "{r.title}"'
        if not SHALL_RE.search(r.title + "\n" + r.body):
            add(r.line, "EARS-shall", "error", f"{name}: text has no SHALL/MUST")
        if not r.scenarios:
            add(r.line, "REQ-scenario", "error", f"{name}: no '#### Scenario:' (not verifiable)")
        for s in r.scenarios:
            if not s.when:
                add(s.line, "SCN-when", "error", f'Scenario "{s.title}": missing WHEN')
            if not s.then:
                add(s.line, "SCN-then", "error", f'Scenario "{s.title}": missing THEN')
            for ln, t in s.then:
                for v in VAGUE_RE.finditer(t):
                    add(ln, "THEN-vague", "warning",
                        f'THEN uses vague word "{v.group(0)}"; state an observable, measurable outcome')
                if SHOULD_RE.search(t):
                    add(ln, "THEN-vague", "warning", 'THEN uses "should"; use SHALL/MUST or a plain observable fact')
                if TWO_SHALL_RE.search(t):
                    add(ln, "ISO29148-singular", "warning",
                        "THEN joins two SHALL/MUST with 'and'; split into one behaviour per THEN/scenario")
        if r.section == "ADDED" and r.scenarios:
            def blob(s): return " ".join([s.title] + [t for _, t in s.when + s.then])
            if not any(NEG_RE.search(blob(s)) for s in r.scenarios):
                add(r.line, "ERR-scenario", "error",
                    f"{name}: no error/negative scenario (keywords: reject, invalid, fail, 4xx/5xx, not, without...)")
            if ACTOR_RE.search(r.title + "\n" + r.body) and not any(PERM_RE.search(blob(s)) for s in r.scenarios):
                add(r.line, "PERM-scenario", "warning",
                    f"{name}: mentions a user/seller/buyer/admin/owner resource but has no permission scenario "
                    "(403/401/not the owner/scope...)")
    return out


def lint_proposal(path: Path, legacy: bool) -> list[Finding]:
    p = rel(path)
    if not path.exists():
        return [Finding(p, 1, "PROPOSAL-sections", "warning" if legacy else "error",
                        SOURCES["PROPOSAL-sections"], "proposal.md not found")]
    headings = [(i, l) for i, l in enumerate(path.read_text().splitlines(), 1) if re.match(r"^#{1,6}\s", l)]
    sev = "warning" if legacy else "error"
    out = []
    for name, pat in SECTION_PATTERNS.items():
        if not any(pat.search(h) for _, h in headings):
            out.append(Finding(p, 1, "PROPOSAL-sections", sev, SOURCES["PROPOSAL-sections"],
                               f'missing section "{name}"'))
    return out


def lint_change(d: Path, legacy: bool = False) -> list[Finding]:
    out = lint_proposal(d / "proposal.md", legacy)
    specs = sorted((d / "specs").rglob("*.md")) if (d / "specs").is_dir() else []
    if not specs:
        out.append(Finding(rel(d / "specs"), 1, "SPEC-none", "warning", SOURCES["SPEC-none"],
                           "no delta spec files under specs/"))
    for s in specs:
        out += lint_spec(s)
    return out


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("changes", nargs="+", help="change dir or id")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--warn-only", action="store_true", help="always exit 0")
    ap.add_argument("--legacy", action="store_true", help="PROPOSAL-sections becomes a warning")
    a = ap.parse_args(argv)
    findings: list[Finding] = []
    per = {}
    for c in a.changes:
        d = resolve_change(c)
        if d is None:
            print(f"spec_lint: change not found: {c}", file=sys.stderr)
            return 2
        f = lint_change(d, a.legacy)
        per[rel(d)] = {"errors": sum(x.severity == "error" for x in f),
                       "warnings": sum(x.severity == "warning" for x in f)}
        findings += f
    errors = sum(f.severity == "error" for f in findings)
    if a.json:
        print(json.dumps({"changes": per, "findings": [asdict(f) for f in findings], "errors": errors}, indent=2))
    else:
        for f in findings:
            print(f"{f.path}:{f.line} {f.rule} [{f.severity}] {f.message} ({f.source})")
        for k, v in per.items():
            print(f"# {k}: {v['errors']} error(s), {v['warnings']} warning(s)")
    return 0 if (a.warn_only or errors == 0) else 1


if __name__ == "__main__":
    sys.exit(main())
