#!/usr/bin/env python3
"""Cross-change review for an ASDLC initiative (plan section 13, step 3b / W2-6).

Usage:
  python scripts/asdlc/cross_change_check.py --slices plans/<id>/slices.yaml
         [--changes-dir openspec/changes] [--proto-dir platform-core/packages/proto]
         [--root <polyrepo root>] [--json]

Rules (each violation prints `rule: message`; exit 1 if any):
  schema     slices.yaml shape; each slice has openspec/changes/<id>/ and its
             proposal.md contains `initiative: <initiative>`.
  dag        depends_on ids exist; no cycles. Prints a build order (info, not a
             violation): topological, platform-core-owned contract slices first.
  contract   every `uses` (`Service.Rpc` or `<topic>/<Event>`) exists in the
             current proto tree, or in `provides` of a transitive dependency, or
             in the proto delta of a transitive dependency's change.
             Proto delta = `rpc X(` (attributed to the nearest preceding
             `service Y {`) and `message X {` found in ANY .md/.proto file of the
             change (fenced snippets included). Event topics are not verifiable;
             only the Event message name is checked.
  ownership  capability C -> repos whose FEATURES.yaml mention it (`capability: C`
             field, a tag equal to C, or "capability C" in the summary). A slice
             touching C whose owner is not among them is flagged. A delta scenario
             whose title matches features in >1 FEATURES.yaml (or repeated in two
             slices) is ambiguous. Scenarios with no FEATURES match yet are fine
             (spec phase, before spec-to-e2e).
  conflict   no two slices ADD/MODIFY/REMOVE the same requirement name in the
             same capability.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
CONTRACT_OWNER = "platform-core"
OPS = ("ADDED", "MODIFIED", "REMOVED")


def _norm(s: str) -> str:  # same normalisation as platform-e2e/scripts/spec_sync.py
    return re.sub(r"[^a-z0-9]+", " ", s.lower()).strip()


# ---------------------------------------------------------------- proto parsing
_SERVICE = re.compile(r"^\s*service\s+(\w+)\s*\{", re.M)
_RPC = re.compile(r"^\s*rpc\s+(\w+)\s*\(", re.M)
_MESSAGE = re.compile(r"^\s*message\s+(\w+)\s*\{", re.M)


def parse_proto_text(text: str) -> tuple[set[str], set[str]]:
    """Return ({'Service.Rpc'}, {'MessageName'}) found in text."""
    rpcs: set[str] = set()
    services = [(m.start(), m.group(1)) for m in _SERVICE.finditer(text)]
    for m in _RPC.finditer(text):
        owner = [n for pos, n in services if pos < m.start()]
        if owner:
            rpcs.add(f"{owner[-1]}.{m.group(1)}")
    return rpcs, {m.group(1) for m in _MESSAGE.finditer(text)}


def proto_tree(proto_dir: Path) -> tuple[set[str], set[str]]:
    rpcs: set[str] = set()
    msgs: set[str] = set()
    for f in proto_dir.rglob("*.proto") if proto_dir.exists() else []:
        r, m = parse_proto_text(f.read_text("utf-8"))
        rpcs |= r
        msgs |= m
    return rpcs, msgs


def change_proto_delta(change_dir: Path) -> tuple[set[str], set[str]]:
    rpcs: set[str] = set()
    msgs: set[str] = set()
    for f in change_dir.rglob("*") if change_dir.exists() else []:
        if f.suffix in (".md", ".proto") and f.is_file():
            r, m = parse_proto_text(f.read_text("utf-8"))
            rpcs |= r
            msgs |= m
    return rpcs, msgs


# ---------------------------------------------------------------- spec parsing
_SCEN = re.compile(r"^####\s+Scenario:\s*(.+?)\s*$", re.M)
_REQ = re.compile(r"^###\s+Requirement:\s*(.+?)\s*$", re.M)
_OPHEAD = re.compile(r"^##\s+(ADDED|MODIFIED|REMOVED|RENAMED)\s+Requirements\s*$", re.M)


def change_specs(change_dir: Path) -> dict[str, dict]:
    """capability -> {'scenarios': [title], 'requirements': [name]} from delta specs."""
    base = change_dir / "specs"
    out: dict[str, dict] = {}
    for spec in sorted(base.rglob("spec.md")) if base.exists() else []:
        cap = spec.parent.relative_to(base).as_posix()
        text = spec.read_text("utf-8")
        heads = list(_OPHEAD.finditer(text))
        reqs = []
        for m in _REQ.finditer(text):
            prior = [h for h in heads if h.start() < m.start()]
            op = prior[-1].group(1) if prior else "ADDED"  # bare `## Requirements` = ADDED
            if op in OPS:
                reqs.append(m.group(1))
        d = out.setdefault(cap, {"scenarios": [], "requirements": []})
        d["scenarios"] += _SCEN.findall(text)
        d["requirements"] += reqs
    return out


def features_index(root: Path) -> tuple[dict[str, set[str]], dict[str, set[str]]]:
    """(capability -> repos, normalised feature title -> repos) from <root>/*/FEATURES.yaml."""
    cap_repos: dict[str, set[str]] = defaultdict(set)
    title_repos: dict[str, set[str]] = defaultdict(set)
    for manifest in sorted(root.glob("*/FEATURES.yaml")):
        data = yaml.safe_load(manifest.read_text("utf-8")) or {}
        repo = data.get("repo") or manifest.parent.name
        for feat in data.get("features") or []:
            if feat.get("title"):
                title_repos[_norm(feat["title"])].add(repo)
            if feat.get("capability"):
                cap_repos[str(feat["capability"])].add(repo)
            for t in feat.get("tags") or []:
                cap_repos[str(t)].add(repo)
            for m in re.finditer(r"capability[:\s]+`?([\w.-]+)", str(feat.get("summary", "")), re.I):
                cap_repos[m.group(1).rstrip(".")].add(repo)
    return cap_repos, title_repos


# ---------------------------------------------------------------- the checks
def find_cycle(deps: dict[str, list[str]]) -> list[str] | None:
    state: dict[str, int] = {}
    stack: list[str] = []

    def dfs(n: str) -> list[str] | None:
        state[n] = 1
        stack.append(n)
        for d in deps.get(n, []):
            if d not in deps:
                continue
            if state.get(d) == 1:
                return stack[stack.index(d):] + [d]
            if d not in state:
                c = dfs(d)
                if c:
                    return c
        stack.pop()
        state[n] = 2
        return None

    for n in deps:
        if n not in state:
            c = dfs(n)
            if c:
                return c
    return None


def build_order(slices: dict[str, dict]) -> list[str]:
    """Kahn; among ready slices, contract owner (platform-core) first, then id."""
    deps = {i: set(s["depends_on"]) & set(slices) for i, s in slices.items()}
    done: list[str] = []
    while len(done) < len(slices):
        ready = sorted(
            (i for i in slices if i not in done and deps[i] <= set(done)),
            key=lambda i: (slices[i]["owner"] != CONTRACT_OWNER, i),
        )
        if not ready:
            break
        done.append(ready[0])
    return done


def transitive(sid: str, deps: dict[str, list[str]]) -> set[str]:
    seen: set[str] = set()
    todo = list(deps.get(sid, []))
    while todo:
        d = todo.pop()
        if d in seen or d not in deps:
            continue
        seen.add(d)
        todo += deps[d]
    return seen


def check(slices_path: Path, changes_dir: Path, proto_dir: Path, root: Path) -> tuple[list[tuple[str, str]], list[str]]:
    v: list[tuple[str, str]] = []

    def bad(rule: str, msg: str) -> None:
        v.append((rule, msg))

    try:
        doc = yaml.safe_load(slices_path.read_text("utf-8")) or {}
    except (OSError, yaml.YAMLError) as e:
        return [("schema", f"cannot read {slices_path}: {e}")], []
    initiative = doc.get("initiative")
    if not isinstance(initiative, str) or not initiative:
        bad("schema", "missing `initiative`")
    raw = doc.get("slices")
    if not isinstance(raw, list) or not raw:
        bad("schema", "`slices` must be a non-empty list")
        return v, []

    slices: dict[str, dict] = {}
    for i, s in enumerate(raw):
        if not isinstance(s, dict) or not isinstance(s.get("id"), str):
            bad("schema", f"slices[{i}]: missing string `id`")
            continue
        sid = s["id"]
        if sid in slices:
            bad("schema", f"duplicate slice id `{sid}`")
            continue
        if not isinstance(s.get("owner"), str) or not s["owner"]:
            bad("schema", f"{sid}: missing `owner`")
        for key in ("depends_on", "uses", "provides"):
            if not isinstance(s.get(key, []), list):
                bad("schema", f"{sid}: `{key}` must be a list")
        slices[sid] = {
            "owner": s.get("owner") or "",
            "depends_on": list(s.get("depends_on") or []),
            "uses": list(s.get("uses") or []),
            "provides": list(s.get("provides") or []),
        }
        cdir = changes_dir / sid
        if not cdir.is_dir():
            bad("schema", f"{sid}: no change directory {cdir}")
        else:
            prop = cdir / "proposal.md"
            text = prop.read_text("utf-8") if prop.exists() else ""
            m = re.search(r"^\s*(?:[-*]\s*)?\**initiative\**\s*:\s*\**\s*([\w.-]+)", text, re.M)
            if not m:
                bad("schema", f"{sid}: proposal.md has no `initiative: {initiative}`")
            elif initiative and m.group(1) != initiative:
                bad("schema", f"{sid}: proposal.md says initiative `{m.group(1)}`, expected `{initiative}`")

    # DAG
    deps = {i: s["depends_on"] for i, s in slices.items()}
    for sid, ds in deps.items():
        for d in ds:
            if d not in slices:
                bad("dag", f"{sid}: depends_on unknown slice `{d}`")
    cyc = find_cycle(deps)
    if cyc:
        bad("dag", "cycle: " + " -> ".join(cyc))
    order = [] if cyc else build_order(slices)

    # contract
    cur_rpcs, cur_msgs = proto_tree(proto_dir)
    delta = {sid: change_proto_delta(changes_dir / sid) for sid in slices}
    for sid, s in slices.items():
        up = transitive(sid, deps)
        up_rpcs = set().union(*(delta[u][0] for u in up)) if up else set()
        up_msgs = set().union(*(delta[u][1] for u in up)) if up else set()
        up_prov = {p for u in up for p in slices[u]["provides"]}
        for use in s["uses"]:
            if "/" in use:
                topic, _, ev = use.partition("/")
                ok = bool(topic) and (ev in cur_msgs or ev in up_msgs or use in up_prov)
            else:
                ok = use in cur_rpcs or use in up_rpcs or use in up_prov
            if not ok:
                bad("contract", f"{sid}: uses `{use}` which is neither in the proto tree nor provided by a dependency")

    # ownership + conflict
    cap_repos, title_repos = features_index(root)
    seen_scen: dict[str, str] = {}
    seen_req: dict[tuple[str, str], str] = {}
    for sid, s in slices.items():
        for cap, d in change_specs(changes_dir / sid).items():
            repos = cap_repos.get(cap)
            if repos and s["owner"] not in repos:
                bad("ownership", f"{sid}: owner `{s['owner']}` touches capability `{cap}` held by {sorted(repos)}")
            for title in d["scenarios"]:
                n = _norm(title)
                hit = title_repos.get(n, set())
                if len(hit) > 1:
                    bad("ownership", f"{sid}: scenario `{title}` matches FEATURES.yaml in {sorted(hit)} (must be exactly one)")
                if n in seen_scen and seen_scen[n] != sid:
                    bad("ownership", f"scenario `{title}` appears in slices {seen_scen[n]} and {sid}")
                seen_scen.setdefault(n, sid)
            for req in d["requirements"]:
                key = (cap, _norm(req))
                if key in seen_req and seen_req[key] != sid:
                    bad("conflict", f"requirement `{req}` in capability `{cap}` modified by both {seen_req[key]} and {sid}")
                seen_req.setdefault(key, sid)
    return v, order


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slices", required=True, type=Path)
    ap.add_argument("--root", type=Path, default=ROOT)
    ap.add_argument("--changes-dir", type=Path)
    ap.add_argument("--proto-dir", type=Path)
    ap.add_argument("--json", action="store_true")
    a = ap.parse_args(argv)
    changes = a.changes_dir or a.root / "openspec" / "changes"
    proto = a.proto_dir or a.root / "platform-core" / "packages" / "proto"
    viol, order = check(a.slices, changes, proto, a.root)
    if a.json:
        print(json.dumps({"ok": not viol, "build_order": order,
                          "violations": [{"rule": r, "message": m} for r, m in viol]}, indent=2))
    else:
        for r, m in viol:
            print(f"{r}: {m}")
        if order:
            print("build order: " + " -> ".join(order))
        if not viol:
            print("cross_change_check: OK")
    return 1 if viol else 0


if __name__ == "__main__":
    sys.exit(main())
