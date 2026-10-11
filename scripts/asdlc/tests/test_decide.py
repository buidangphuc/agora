import copy
import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
import decide  # noqa: E402
from engines import MockEngine  # noqa: E402

FX = HERE / "fixtures" / "decide"
GATE = FX / "gate.v1.yaml"
UNITS = json.loads((FX / "units.json").read_text())
ANS = json.loads((FX / "answers.json").read_text())


def run(units=None, answers=None, mode="enforce"):
    return decide.run(GATE, units or UNITS, MockEngine(answers or ANS), mode)


def unit(doc, uid):
    return next(u for u in doc["units"] if u["id"] == uid)


def test_decision_table_rows():
    d = run()
    assert unit(d, "s_pass")["result"] == "PASS"
    assert unit(d, "s_block")["result"] == "BLOCK"
    assert unit(d, "s_unc")["result"] == "ESCALATE_LLM"
    assert unit(d, "s_unc")["escalations"] == ["checks_store"]
    assert unit(d, "s_fail")["result"] == "FAIL"
    h = unit(d, "s_human")
    assert h["result"] == "PASS" and h["needs_human"] and "auth" in h["routing"]
    assert d["result"] == "BLOCK" and d["needs_human"]
    assert d["escalations"] == [{"unit": "s_unc", "questions": ["checks_store"]}]


def test_engine_down_row():
    a = copy.deepcopy(ANS)
    a["s_pass"] = {"__down__": True}
    d = run(answers=a)
    assert unit(d, "s_pass")["result"] == "ENGINE_DOWN"
    assert d["result"] == "ENGINE_DOWN" and d["fallback"] == "llm_reviewer"


def test_advisory_vs_enforce():
    assert unit(run(mode="advisory"), "s_block")["result"] == "WOULD_BLOCK"
    assert run(mode="advisory")["result"] == "WOULD_BLOCK"
    assert unit(run(mode="enforce"), "s_block")["result"] == "BLOCK"


def test_pass_when_false():
    q = {"type": "noul", "pass_when": False, "threshold": 0.8}
    assert decide.evaluate(q, {"yes": 0.05})[0] == "pass"
    assert decide.evaluate(q, {"yes": 0.95})[0] == "fail"
    assert decide.evaluate(q, {"yes": 0.5})[0] == "uncertain"


def test_score_pass_min():
    q = {"type": "score", "levels": {"none": "a", "partial": "b", "full": "c"}, "pass_min": "partial", "min_confidence": 0.6}
    assert decide.evaluate(q, {"score": 0, "level": "none", "confidence": 0.9})[0] == "fail"
    assert decide.evaluate(q, {"score": 1, "confidence": 0.9})[0] == "pass"
    assert decide.evaluate(q, {"level": "full", "confidence": 0.9})[0] == "pass"
    assert decide.evaluate(q, {"score": 2, "confidence": 0.3})[0] == "uncertain"


def test_choice_low_confidence_uncertain():
    q = {"type": "choice", "criteria": {"real": "", "mock": ""}, "pass_values": ["real"], "min_confidence": 0.6}
    assert decide.evaluate(q, {"choice": "real", "confidence": 0.5})[0] == "uncertain"


def test_oversize_unit_escalates_without_engine_call():
    big = [{"id": "s_big", "parts": {"SPEC": "x" * 4000}, "paths": []}]
    d = run(units=big, answers={})  # engine has no answers: would raise if called
    u = unit(d, "s_big")
    assert u["result"] == "ESCALATE_LLM" and "too large" in u["note"]
    assert d["escalations"][0]["unit"] == "s_big"


def test_deterministic_and_cli(tmp_path):
    outs = []
    for i in range(2):
        out = tmp_path / str(i)
        decide.main(["--gate", str(GATE), "--units", str(FX / "units.json"), "--engine", "mock",
                     "--answers", str(FX / "answers.json"), "--mode", "advisory", "--out", str(out)])
        outs.append(((out / "decisions.json").read_bytes(), (out / "decisions.md").read_bytes()))
    assert outs[0] == outs[1]
    # replay of the first run reproduces the same decisions
    out = tmp_path / "r"
    decide.main(["--gate", str(GATE), "--units", str(FX / "units.json"), "--engine", "replay",
                 "--answers", str(tmp_path / "0" / "decisions.json"), "--mode", "advisory", "--out", str(out)])
    a = json.loads((tmp_path / "0" / "decisions.json").read_text())
    b = json.loads((out / "decisions.json").read_text())
    b["engine"] = a["engine"]
    assert a == b


def test_default_mode_reads_kev_mode(monkeypatch):
    import asdlc_config
    monkeypatch.setattr(asdlc_config, "load", lambda *a, **k: {"kev_mode": "enforce"})
    assert decide.default_mode() == "enforce"
    monkeypatch.setattr(asdlc_config, "load", lambda *a, **k: {"kev_mode": "advisory"})
    assert decide.default_mode() == "advisory"


def test_config_error_warns_and_falls_back(monkeypatch, capsys):
    import asdlc_config

    def boom(*a, **k):
        raise ValueError("bad cfg")
    monkeypatch.setattr(asdlc_config, "load", boom)
    decide._WARNED.clear()
    assert decide.default_mode() == "advisory"
    assert "WARNING" in capsys.readouterr().err
    assert decide.route(["team-x/JWT.go", "deploy/images/a.sh"]) == ["auth", "release"]


def test_route_uses_config_globs():
    assert "auth" in decide.route(["team-x/internal/scope_gate.go"])
    assert "release" in decide.route(["platform-gitops/envs/prod/a.yaml"])


def test_kev_engine_passes_timeout_consistent_with_wait(monkeypatch):
    import subprocess
    from engines import KevEngine
    seen = {}

    class R:
        returncode, stdout, stderr = 0, '{"answers": {}}', ""

    def fake(cmd, **kw):
        seen["cmd"], seen["wait"] = cmd, kw["timeout"]
        return R()
    monkeypatch.setattr(subprocess, "run", fake)
    KevEngine(kev_path="k.py", timeout=120).ask({"a": "x"}, {}, "t")
    assert seen["cmd"][seen["cmd"].index("--timeout") + 1] == "120"
    assert seen["wait"] == 130
    KevEngine(kev_path="k.py").ask({"a": "x"}, {}, "t")
    assert seen["cmd"][seen["cmd"].index("--timeout") + 1] == "300" and seen["wait"] == 310
