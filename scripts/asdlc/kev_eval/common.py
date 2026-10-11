"""Shared helpers for the Kev A/B/C eval harness (plan section 14, task W3-6).

Parsers for OpenSpec archived specs, gherkin .feature files and pytest-bdd step
definitions; paths; deterministic hashing; small stats helpers.
"""
from __future__ import annotations

import ast
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
EVAL_DIR = "metrics/kev-eval"
DEFAULT_GATE = "gates/feature.v1.yaml"
E2E_REL = "platform-e2e/tests/e2e"

ARMS = ("A", "B", "C")


def eval_dir(root: Path = ROOT) -> Path:
    return root / EVAL_DIR


def set_dir(set_id: str, root: Path = ROOT) -> Path:
    return eval_dir(root) / "sets" / set_id


def run_dir(set_id: str, arm: str, engine: str, root: Path = ROOT) -> Path:
    return eval_dir(root) / "runs" / set_id / f"{arm}-{engine}"


def write_json(path: Path, obj) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, indent=2, sort_keys=True, ensure_ascii=False) + "\n", encoding="utf-8")


def read_json(path: Path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def stable_hash(*parts: str) -> int:
    h = hashlib.sha256("\x1f".join(parts).encode("utf-8")).hexdigest()
    return int(h[:12], 16)


def slug(s: str, n: int = 60) -> str:
    return re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-")[:n] or "x"


def est_tokens_text(*texts: str) -> int:
    return -(-sum(len(t) for t in texts) // 4)


# ---------------------------------------------------------------- OpenSpec ---
_SCEN_RE = re.compile(r"^####\s+Scenario:\s*(.+?)\s*$", re.M)


def spec_scenarios(spec_md: str) -> list[dict]:
    """[{name, text}] for every `#### Scenario:` block (text = block incl. heading)."""
    ms = list(_SCEN_RE.finditer(spec_md))
    out = []
    for i, m in enumerate(ms):
        end = len(spec_md)
        nxt = re.search(r"^#{1,4}\s", spec_md[m.end():], re.M)
        if nxt:
            end = m.end() + nxt.start()
        out.append({"name": m.group(1).strip(), "text": spec_md[m.start():end].strip()})
    return out


def requirement_of(spec_md: str, pos_name: str) -> str:
    """Name of the `### Requirement:` heading preceding the scenario named pos_name."""
    m = re.search(r"^####\s+Scenario:\s*" + re.escape(pos_name) + r"\s*$", spec_md, re.M)
    if not m:
        return ""
    reqs = list(re.finditer(r"^###\s+Requirement:\s*(.+?)\s*$", spec_md[:m.start()], re.M))
    return reqs[-1].group(1) if reqs else ""


# ----------------------------------------------------------------- gherkin ---
_STEP_KW = ("Given", "When", "Then", "And", "But", "*")


def parse_feature(text: str) -> list[dict]:
    """[{name, tags, steps:[(kw, text)], text, outline, background}] per Scenario / Scenario Outline.

    `text` is the verbatim scenario block (directly preceding tag and comment lines included).
    """
    lines = text.splitlines()
    heads = [i for i, ln in enumerate(lines) if re.match(r"\s*Scenario(?: Outline)?:", ln)]
    feat_tags = []
    for ln in lines:
        st = ln.strip()
        if st.startswith("Feature:"):
            break
        if st.startswith("@"):
            feat_tags += st.split()
    bg: list[tuple[str, str]] = []
    in_bg = False
    for ln in lines[: heads[0] if heads else len(lines)]:
        st = ln.strip()
        if st.startswith("Background:"):
            in_bg = True
        elif in_bg and st.split(" ", 1)[0] in _STEP_KW:
            bg.append((st.split(" ", 1)[0], st.split(" ", 1)[1].strip() if " " in st else ""))
    starts = []
    for h in heads:
        j = h
        while j > 0 and lines[j - 1].strip().startswith(("@", "#")):
            j -= 1
        starts.append(j)
    out = []
    for n, h in enumerate(heads):
        end = starts[n + 1] if n + 1 < len(heads) else len(lines)
        m = re.match(r"\s*Scenario(?: Outline)?:\s*(.*)$", lines[h])
        tags = list(feat_tags)
        for ln in lines[starts[n]:h]:
            if ln.strip().startswith("@"):
                tags += ln.split()
        steps = []
        for ln in lines[h + 1:end]:
            st = ln.strip()
            kw = st.split(" ", 1)[0]
            if kw in _STEP_KW:
                steps.append((kw, st.split(" ", 1)[1].strip() if " " in st else ""))
        out.append({"name": m.group(1).strip(), "tags": tags, "steps": steps, "background": list(bg),
                    "outline": "Outline" in lines[h],
                    "text": "\n".join(lines[starts[n]:end]).rstrip()})
    return out


def norm_title(s: str) -> str:
    return " ".join(re.sub(r"[^a-z0-9 ]+", " ", s.lower()).split())


# -------------------------------------------------------- pytest-bdd steps ---
def _pattern_to_regex(pat: str, kind: str):
    if kind == "re":
        try:
            return re.compile(pat)
        except re.error:
            return None
    # parse: {name}, {name:d}, {name:f}, {name:w}, ...
    out, i = "", 0
    for m in re.finditer(r"\{[^{}]*\}", pat):
        out += re.escape(pat[i:m.start()])
        spec = m.group(0)[1:-1]
        fmt = spec.split(":", 1)[1] if ":" in spec else ""
        out += {"d": r"[-+]?\d+", "f": r"[-+]?\d*\.?\d+", "w": r"\w+"}.get(fmt, r".+?")
        i = m.end()
    out += re.escape(pat[i:])
    return re.compile("^" + out + "$")


def _str_arg(node) -> tuple[str, str] | None:
    """-> (pattern, kind) from a decorator argument node."""
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value, "plain"
    if isinstance(node, ast.Call) and node.args:
        fn = node.func
        name = fn.attr if isinstance(fn, ast.Attribute) else getattr(fn, "id", "")
        a0 = node.args[0]
        if isinstance(a0, ast.Constant) and isinstance(a0.value, str) and name in ("parse", "re", "cfparse", "string"):
            return a0.value, ("re" if name == "re" else "parse" if name in ("parse", "cfparse") else "plain")
    return None


def load_step_defs(step_dir: Path) -> list[dict]:
    """Every @given/@when/@then in step_dir/*.py -> {kw, rx, file, func, source}."""
    defs = []
    for f in sorted(Path(step_dir).glob("*.py")):
        try:
            src = f.read_text(encoding="utf-8")
            tree = ast.parse(src)
        except (SyntaxError, OSError):
            continue
        lines = src.splitlines()
        for node in tree.body:
            if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                continue
            kws = []
            for d in node.decorator_list:
                if isinstance(d, ast.Call):
                    nm = d.func.id if isinstance(d.func, ast.Name) else getattr(d.func, "attr", "")
                    if nm in ("given", "when", "then") and d.args:
                        got = _str_arg(d.args[0])
                        if got:
                            kws.append((nm, got))
            if not kws:
                continue
            start = min([d.lineno for d in node.decorator_list] + [node.lineno]) - 1
            source = "\n".join(lines[start:node.end_lineno])
            for nm, (pat, kind) in kws:
                rx = _pattern_to_regex(pat, kind)
                if rx:
                    defs.append({"kw": nm, "rx": rx, "file": f.name, "func": node.name, "source": source})
    return defs


def resolve_steps(steps: list[tuple[str, str]], defs: list[dict]) -> tuple[list[dict], list[str]]:
    """Map gherkin steps to step-def functions. And/But inherit the previous keyword.
    -> (unique matched defs in step order, unmatched step texts)."""
    seen, matched, missing = set(), [], []
    prev = "given"
    for kw, text in steps:
        k = kw.lower()
        if k in ("and", "but", "*"):
            k = prev
        else:
            prev = k
        cands = [d for d in defs if d["rx"].match(text)]
        pick = next((d for d in cands if d["kw"] == k), cands[0] if cands else None)
        if pick is None:
            missing.append(f"{kw} {text}")
            continue
        key = (pick["file"], pick["func"])
        if key not in seen:
            seen.add(key)
            matched.append(pick)
    return matched, missing


# ------------------------------------------------------------- FEATURES.yaml ---
def load_covered_by(root: Path) -> dict[str, dict]:
    """{'<rel>.feature::<scenario>': {feature_id, repo, status}} from every <repo>/FEATURES.yaml."""
    import yaml
    out: dict[str, dict] = {}
    for f in sorted(root.glob("*/FEATURES.yaml")):
        try:
            doc = yaml.safe_load(f.read_text(encoding="utf-8")) or {}
        except yaml.YAMLError:
            continue
        for ft in doc.get("features") or []:
            cb = ft.get("covered_by")
            cbs = [cb] if isinstance(cb, str) else list(cb or [])
            for c in cbs:
                if isinstance(c, str) and "::" in c:
                    out[c.strip()] = {"feature_id": ft.get("id"), "repo": doc.get("repo", f.parent.name),
                                      "status": ft.get("status")}
    return out
