#!/usr/bin/env python3
"""Validate gate question sets (gates/<gate>.v<N>.yaml) against gates/SCHEMA.md.

Usage: check_gates.py [FILE ...]      (default: every gates/*.v*.yaml in the repo)
Exit 0 when every file is valid, 1 otherwise (errors printed as `path: message`).
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
GATES = {"spec", "task", "feature", "release"}
STATE_UNITS = {"scenario", "task", "change"}
TYPES = {"noul", "choice", "score"}
SEVERITIES = {"veto", "normal"}
TOP_REQUIRED = {"gate", "version", "state_unit", "max_state_tokens", "questions"}
TOP_ALLOWED = TOP_REQUIRED | {"pins"}
Q_COMMON = {"id", "source", "type", "severity", "instructions"}
Q_TYPED = {
    "noul": {"pass_when", "threshold"},
    "choice": {"criteria", "pass_values", "min_confidence"},
    "score": {"levels", "pass_min", "min_confidence"},
}
ID_RE = re.compile(r"^[a-z][a-z0-9]*(_[a-z0-9]+)*$")
FILE_RE = re.compile(r"^(?P<gate>[a-z]+)\.v(?P<version>[1-9][0-9]*)\.yaml$")


class _UniqueKeyLoader(yaml.SafeLoader):
    """SafeLoader that rejects duplicate mapping keys (PyYAML silently keeps the last)."""


def _construct_mapping(loader, node, deep=False):
    seen = set()
    for key_node, _ in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in seen:
            raise yaml.constructor.ConstructorError(
                None, None, f"duplicate key {key!r}", key_node.start_mark)
        seen.add(key)
    return loader.construct_mapping(node, deep)


_UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _construct_mapping)


def _nonempty_str(v) -> bool:
    return isinstance(v, str) and v.strip() != ""


def _is_prob(v) -> bool:
    return isinstance(v, (int, float)) and not isinstance(v, bool) and 0.5 < v < 1


def _check_options(qid: str, field: str, opts, errors: list[str]) -> bool:
    if not isinstance(opts, dict) or len(opts) < 2:
        errors.append(f"{qid}: {field} must be a mapping with at least 2 options")
        return False
    for k, desc in opts.items():
        if not (isinstance(k, str) and ID_RE.match(k)):
            errors.append(f"{qid}: {field} option {k!r} is not snake_case")
        if not _nonempty_str(desc):
            errors.append(f"{qid}: {field} option {k!r} has no description")
    return True


def check_question(q, errors: list[str]) -> str | None:
    if not isinstance(q, dict):
        errors.append("question is not a mapping")
        return None
    qid = q.get("id")
    label = qid if isinstance(qid, str) else "<no id>"
    if not (isinstance(qid, str) and ID_RE.match(qid)):
        errors.append(f"{label}: id must be snake_case")
    for key in sorted(Q_COMMON - q.keys()):
        errors.append(f"{label}: missing required key {key!r}")
    for key in ("source", "instructions"):
        if key in q and not _nonempty_str(q[key]):
            errors.append(f"{label}: {key} must be a non-empty string")
    if "severity" in q and q["severity"] not in SEVERITIES:
        errors.append(f"{label}: severity {q['severity']!r} not in {sorted(SEVERITIES)}")
    qtype = q.get("type")
    if qtype not in TYPES:
        if "type" in q:
            errors.append(f"{label}: type {qtype!r} not in {sorted(TYPES)}")
        return qid if isinstance(qid, str) else None

    typed = Q_TYPED[qtype]
    for key in sorted(typed - q.keys()):
        errors.append(f"{label}: {qtype} question missing {key!r}")
    for key in sorted(q.keys() - Q_COMMON - typed):
        errors.append(f"{label}: key {key!r} not allowed for type {qtype}")

    if qtype == "noul":
        if "pass_when" in q and not isinstance(q["pass_when"], bool):
            errors.append(f"{label}: pass_when must be true or false")
        if "threshold" in q and not _is_prob(q["threshold"]):
            errors.append(f"{label}: threshold must be in (0.5, 1)")
    else:
        if "min_confidence" in q and not _is_prob(q["min_confidence"]):
            errors.append(f"{label}: min_confidence must be in (0.5, 1)")
    if qtype == "choice" and _check_options(label, "criteria", q.get("criteria"), errors):
        pv = q.get("pass_values")
        if not isinstance(pv, list) or not pv:
            errors.append(f"{label}: pass_values must be a non-empty list")
        else:
            bad = [v for v in pv if v not in q["criteria"]]
            if bad:
                errors.append(f"{label}: pass_values {bad} not in criteria")
            if len(set(pv)) != len(pv):
                errors.append(f"{label}: pass_values has duplicates")
            if set(pv) >= set(q["criteria"]):
                errors.append(f"{label}: every criterion passes; question can never fail")
    if qtype == "score" and _check_options(label, "levels", q.get("levels"), errors):
        if "pass_min" in q and q["pass_min"] not in q["levels"]:
            errors.append(f"{label}: pass_min {q['pass_min']!r} not in levels")
        elif q.get("pass_min") == next(iter(q["levels"])):
            errors.append(f"{label}: pass_min is the lowest level; question can never fail")
    return qid if isinstance(qid, str) else None


def check_data(data, filename: str) -> list[str]:
    errors: list[str] = []
    if not isinstance(data, dict):
        return ["top level must be a mapping"]
    for key in sorted(TOP_REQUIRED - data.keys()):
        errors.append(f"missing required key {key!r}")
    for key in sorted(data.keys() - TOP_ALLOWED):
        errors.append(f"unknown top-level key {key!r}")

    gate, version = data.get("gate"), data.get("version")
    if "gate" in data and gate not in GATES:
        errors.append(f"gate {gate!r} not in {sorted(GATES)}")
    if "version" in data and not (isinstance(version, int) and not isinstance(version, bool)
                                  and version >= 1):
        errors.append("version must be an integer >= 1")
    if "state_unit" in data and data["state_unit"] not in STATE_UNITS:
        errors.append(f"state_unit {data['state_unit']!r} not in {sorted(STATE_UNITS)}")
    mst = data.get("max_state_tokens")
    if "max_state_tokens" in data and not (isinstance(mst, int) and not isinstance(mst, bool)
                                           and mst > 0):
        errors.append("max_state_tokens must be a positive integer")
    pins = data.get("pins", {})
    if not isinstance(pins, dict) or not all(_nonempty_str(v) for v in pins.values()):
        errors.append("pins must map names to non-empty version strings")
        pins = {}

    expected = f"{gate}.v{version}.yaml"
    if filename != expected:
        errors.append(f"file name must be {expected!r} (gate + version)")

    questions = data.get("questions")
    if "questions" in data and (not isinstance(questions, list) or not questions):
        errors.append("questions must be a non-empty list")
        questions = []
    seen: set[str] = set()
    for q in questions or []:
        qid = check_question(q, errors)
        if qid is None:
            continue
        if qid in seen:
            errors.append(f"{qid}: duplicate question id")
        seen.add(qid)
        src = q.get("source") if isinstance(q, dict) else None
        if isinstance(src, str) and "ASVS" in src and "asvs" not in pins:
            errors.append(f"{qid}: cites ASVS but pins.asvs is not set")
    return errors


def check_file(path: Path) -> list[str]:
    if not FILE_RE.match(path.name):
        return [f"file name {path.name!r} does not match <gate>.v<N>.yaml"]
    try:
        data = yaml.load(path.read_text(encoding="utf-8"), Loader=_UniqueKeyLoader)
    except (yaml.YAMLError, OSError) as exc:
        return [f"cannot parse: {exc}"]
    return check_data(data, path.name)


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    files = [Path(a) for a in args] or sorted((REPO / "gates").glob("*.v*.yaml"))
    if not files:
        print("check_gates: no gate files found", file=sys.stderr)
        return 1
    failed = False
    for path in files:
        errs = check_file(path)
        for e in errs:
            print(f"{path}: {e}")
        failed = failed or bool(errs)
    if not failed:
        print(f"check_gates: {len(files)} file(s) OK")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
