"""Engine adapters for decide.py. ask(parts, questions, tag) -> {question_id: answer}.

Answer shapes (TypeSafe/Kev compact):
  noul   {"yes": p}
  choice {"choice": str, "confidence": float, "probabilities": {...}}
  score  {"score": int, "level": str, "confidence": float, "probabilities": {...}}
Changing engine never changes the gate files: build_questions() is the only translator.
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


class EngineDown(Exception):
    """Engine unreachable / timed out -> caller falls back to the LLM reviewer."""


class EngineError(Exception):
    """Engine reachable but the request or answer is unusable."""


def build_questions(questions: list[dict]) -> dict:
    """Gate question list -> Kev/TypeSafe `questions` object (SCHEMA.md wire format)."""
    out: dict = {}
    for q in questions:
        t = q["type"]
        item = {"type": t, "instructions": q["instructions"]}
        if t == "choice":
            item["criteria"] = dict(q["criteria"])
        elif t == "score":
            item["criteria"] = list(q["levels"].values())
        elif t != "noul":
            raise EngineError(f"unknown question type {t!r} ({q.get('id')})")
        out[q["id"]] = item
    return out


class Engine:
    name = "base"
    last_latency_ms = None

    def ask(self, parts: dict, questions: dict, tag: str) -> dict:
        raise NotImplementedError


class MockEngine(Engine):
    """Canned answers: {unit_id: {question_id: answer}}. Special unit values:
    {"__down__": true} -> EngineDown, {"__error__": "msg"} -> EngineError."""
    name = "mock"

    def __init__(self, answers: dict):
        self.answers = answers

    @classmethod
    def from_file(cls, path):
        return cls(json.loads(Path(path).read_text(encoding="utf-8")))

    def ask(self, parts, questions, tag):
        if tag not in self.answers:
            raise EngineError(f"mock: no canned answers for unit {tag!r}")
        a = self.answers[tag]
        if a.get("__down__"):
            raise EngineDown("mock: engine down")
        if "__error__" in a:
            raise EngineError(str(a["__error__"]))
        return {k: v for k, v in a.items() if k in questions}


class ReplayEngine(MockEngine):
    """Re-reads raw answers from a previous decisions.json."""
    name = "replay"

    @classmethod
    def from_file(cls, path):
        prev = json.loads(Path(path).read_text(encoding="utf-8"))
        ans = {}
        for u in prev.get("units", []):
            qs = u.get("questions", {})
            if u.get("result") == "ENGINE_DOWN":
                ans[u["id"]] = {"__down__": True}
            elif qs:
                ans[u["id"]] = {qid: q["raw"] for qid, q in qs.items() if q.get("raw") is not None}
        return cls(ans)


class KevEngine(Engine):
    """Shells out to kev.py. Exit codes: 3 ENGINE_DOWN, 5 timeout, 4 bad request, 6 server error, 2 usage."""
    name = "kev"

    SLACK_S = 10  # subprocess wait = kev --timeout + this, so kev reports its own TIMEOUT (exit 5) first

    def __init__(self, kev_path=None, timeout=None):
        """timeout = seconds passed to `kev.py ask --timeout` (default env ASDLC_KEV_TIMEOUT or 300)."""
        self.kev = kev_path or os.environ.get("KEV_PY") or str(Path.home() / "tools/kev/kev.py")
        self.kev_timeout = float(timeout if timeout is not None else os.environ.get("ASDLC_KEV_TIMEOUT", 300))
        self.timeout = self.kev_timeout + self.SLACK_S

    def ask(self, parts, questions, tag):
        with tempfile.TemporaryDirectory() as td:
            qf = Path(td) / "q.json"
            qf.write_text(json.dumps(questions, ensure_ascii=False), encoding="utf-8")
            cmd = [sys.executable, self.kev, "ask", "--questions", f"@{qf}", "--tag", tag,
                   "--timeout", f"{self.kev_timeout:g}"]
            for i, (name, text) in enumerate(parts.items()):
                pf = Path(td) / f"part{i}.txt"
                pf.write_text(text, encoding="utf-8")
                cmd += ["--part", f"{name}=@{pf}"]
            try:
                r = subprocess.run(cmd, capture_output=True, text=True, timeout=self.timeout)
            except subprocess.TimeoutExpired as e:
                raise EngineDown(f"kev: timeout after {self.timeout}s") from e
            except OSError as e:
                raise EngineDown(f"kev: cannot run {self.kev}: {e}") from e
        out = r.stdout.strip().splitlines()
        last = out[-1] if out else ""
        if r.returncode in (3, 5):
            raise EngineDown(_err(last, r.stderr, r.returncode))
        if r.returncode != 0:
            raise EngineError(_err(last, r.stderr, r.returncode))
        try:
            data = json.loads(last)
            self.last_latency_ms = data.get("latency_ms")
            return data["answers"]
        except (ValueError, KeyError) as e:
            raise EngineError(f"kev: unparseable output: {last[:200]!r}") from e


def _err(last, stderr, code):
    try:
        return f"kev exit {code}: {json.loads(last).get('error')}"
    except (ValueError, AttributeError):
        return f"kev exit {code}: {(last or stderr).strip()[:300]}"
