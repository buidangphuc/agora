import json
import sys
import textwrap
from pathlib import Path

import pytest
import yaml

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parents[1]))
from kev_eval import build_set, common as c, mock_answers, run, score, seed  # noqa: E402

GATE_YAML = """
gate: feature
version: 1
state_unit: scenario
max_state_tokens: 8000
questions:
  - id: assertion_kind
    source: "Meszaros"
    type: choice
    severity: veto
    instructions: "What do the Then assertions check?"
    criteria: {real: "state", proxy: "status only", mock: "mock called", none: "nothing"}
    pass_values: [real]
    min_confidence: 0.6
  - id: promise_match
    source: "ISO 29119-4"
    type: score
    severity: veto
    instructions: "Coverage of the promise"
    levels: {unrelated: "x", partial: "y", main_outcome: "z", full: "w"}
    pass_min: main_outcome
    min_confidence: 0.6
  - id: permission_probe
    source: "ASVS"
    type: choice
    severity: veto
    instructions: "permission"
    criteria: {not_applicable: "na", cross_user_probe: "probe", same_user_only: "same"}
    pass_values: [not_applicable, cross_user_probe]
    min_confidence: 0.6
  - id: invalid_partition_assert
    source: "ISO"
    type: choice
    severity: normal
    instructions: "invalid"
    criteria: {not_applicable: "na", specific: "s", generic: "g"}
    pass_values: [not_applicable, specific]
    min_confidence: 0.6
  - id: boundary_values
    source: "ISO"
    type: choice
    severity: normal
    instructions: "boundary"
    criteria: {not_applicable: "na", both_sides: "b", missing: "m"}
    pass_values: [not_applicable, both_sides]
    min_confidence: 0.6
  - id: mystery_guest
    source: "tsDetect"
    type: noul
    severity: normal
    instructions: "mystery guest?"
    pass_when: false
    threshold: 0.85
  - id: code_beyond_tests
    source: "Google"
    type: choice
    severity: normal
    instructions: "extra behaviour?"
    criteria: {none: "n", incidental: "i", untested_behaviour: "u"}
    pass_values: [none, incidental]
    min_confidence: 0.6
"""
GATE = yaml.safe_load(GATE_YAML)

STEPS_API = textwrap.dedent('''\
    # a_steps.py::do_it
    @when("the buyer calls the api with 300 items")
    def do_it(world):
        world.state.resp = call(items=300)


    # a_steps.py::check
    @then("the request is rejected with 403")
    def check(world):
        resp = world.state.resp
        assert resp.status_code == 403, resp.text
        assert resp.json()["error"] == "forbidden"
        assert not store.exists(resp.json()["id"])''')

STEPS_UI = textwrap.dedent('''\
    # u_steps.py::open_page
    @when("the buyer opens the page")
    def open_page(world):
        world.page.goto("/x")


    # u_steps.py::see
    @then("the page shows the new value")
    def see(world):
        expect(world.page.get_by_text("saved")).to_be_visible()
        assert world.page.title() == "Done"''')

STEPS_HELPER_ONLY = textwrap.dedent('''\
    # h_steps.py::chk
    @then("all good")
    def chk(world):
        _helper(world)''')


def clean_unit(uid, steps, spec, feature="Scenario: x\n  Given a\n  Then b"):
    return {"id": uid, "change": "c-" + uid, "paths": ["f.feature"],
            "parts": {"SPEC": spec, "FEATURE": feature, "STEPS": steps,
                      "IMPL": build_set.synth_impl(uid, spec)}}


def synthetic_clean():
    spec_deny = "#### Scenario: Other user is denied\n- **WHEN** a buyer calls with 300 items\n- **THEN** the call is rejected with 403"
    spec_ok = "#### Scenario: Value saved\n- **WHEN** the buyer opens the page\n- **THEN** the page shows the new value, at most 40 characters"
    return [clean_unit("u1", STEPS_API, spec_deny),
            clean_unit("u2", STEPS_UI, spec_ok),
            clean_unit("u3", STEPS_UI.replace("saved", "saved2"), spec_ok),
            clean_unit("u4", STEPS_API, spec_deny.replace("300", "5")),
            clean_unit("u5", STEPS_HELPER_ONLY, spec_ok)]


# ------------------------------------------------------------------- seed ---
def by_defect(units, labels, d):
    return {m["base"]: next(u for u in units if u["id"] == i)
            for i, m in labels["units"].items() if m["defect"] == d}


def test_seed_transforms_and_labels():
    units, labels = seed.seed(synthetic_clean(), 1, per_defect=10, gate=GATE)
    L = labels["units"]
    # every clean unit is kept as a negative
    assert sum(1 for m in L.values() if m["group"] == "clean") == 5
    # proxy: first assert of every @then -> status code, rest removed
    px = by_defect(units, labels, "proxy_status_only")
    assert "u5" not in px  # helper-only unit: no assertion to rewrite
    api = px["u1"]["parts"]["STEPS"]
    assert "assert resp.status_code == 200" in api and "403" not in api.split("def check")[1]
    ui = px["u2"]["parts"]["STEPS"]
    assert 'expect(world.page.locator("body")).to_be_visible()' in ui and "get_by_text" not in ui
    # mock-only
    mk = by_defect(units, labels, "mock_only")
    assert "mock_client.assert_called_once()" in mk["u1"]["parts"]["STEPS"]
    # no-test: Then bodies emptied
    nt = by_defect(units, labels, "scenario_no_test")["u1"]["parts"]["STEPS"]
    assert "pass  # TODO" in nt and "assert" not in nt.split("def check")[1]
    # removed error case: 403 -> 200 inside the Then assertions; spec untouched; permission variant label
    re_ = by_defect(units, labels, "removed_error_case")
    assert "403" not in re_["u1"]["parts"]["STEPS"].split("def check")[1]
    assert "== 200" in re_["u1"]["parts"]["STEPS"] and re_["u1"]["parts"]["SPEC"] == synthetic_clean()[0]["parts"]["SPEC"]
    assert "u2" not in re_  # not an error case
    assert L["u1~removed_error_case"]["expect_fail"] == ["permission_probe"]
    # boundary: 300 -> 150 in feature/steps
    bv = by_defect(units, labels, "removed_boundary_case")
    assert "150 items" in bv["u1"]["parts"]["STEPS"] and "300" not in bv["u1"]["parts"]["STEPS"]
    assert L["u1~removed_boundary_case"]["expect_fail"] == ["boundary_values"]
    # code beyond tests / off-by-one touch IMPL only
    cb = by_defect(units, labels, "code_beyond_tests")["u1"]["parts"]
    assert cb["STEPS"] == synthetic_clean()[0]["parts"]["STEPS"] and cb["IMPL"] != synthetic_clean()[0]["parts"]["IMPL"]
    ob = by_defect(units, labels, "off_by_one_impl")["u1"]["parts"]["IMPL"]
    assert "len(req.items) >= MAX_ITEMS" in ob
    # out-of-checklist group: expected none, no question expected to fail
    for d in ("wrong_field_assertion", "off_by_one_impl", "hardcoded_test_data_leak"):
        for i, m in L.items():
            if m["defect"] == d:
                assert m["group"] == "out_of_checklist" and m["expected"] == "none" and m["expect_fail"] == []
    assert "_value" in by_defect(units, labels, "wrong_field_assertion")["u1"]["parts"]["STEPS"]
    # skipped transforms carry a reason
    assert any(s["base"] == "u5" and s["defect"] == "proxy_status_only" and "no assert" in s["reason"] for s in labels["skipped"])
    assert labels["counts"]["units_total"] == len(units)


def test_seed_is_deterministic_and_capped():
    a = seed.seed(synthetic_clean(), 7, per_defect=2, gate=GATE)
    b = seed.seed(synthetic_clean(), 7, per_defect=2, gate=GATE)
    assert a == b
    for d in seed.DEFECTS:
        assert a[1]["counts"][d] <= 2


# -------------------------------------------------------------- build_set ---
def make_repo(tmp: Path):
    arch = tmp / "openspec/changes/archive/2026-01-01-demo/specs/cap"
    arch.mkdir(parents=True)
    (arch / "spec.md").write_text(textwrap.dedent("""\
        ## ADDED Requirements

        ### Requirement: Revoke

        #### Scenario: A session is revoked
        - **WHEN** the buyer revokes
        - **THEN** the token gets 401 within 5 seconds

        #### Scenario: Not covered anywhere
        - **WHEN** x
        - **THEN** y
        """))
    (tmp / "team-x").mkdir()
    (tmp / "team-x/FEATURES.yaml").write_text(yaml.safe_dump({"repo": "team-x", "features": [
        {"id": "a.b", "status": "automated", "covered_by": "auth/rev.feature::A session is revoked"}]}))
    e2e = tmp / "platform-e2e/tests/e2e"
    (e2e / "features/auth").mkdir(parents=True)
    (e2e / "step_definitions").mkdir(parents=True)
    (e2e / "features/auth/rev.feature").write_text(
        "@auth\nFeature: Rev\n\n  Scenario: A session is revoked\n    Given a buyer on 2 devices\n"
        "    When the buyer revokes\n    Then the token gets 401 within 5 seconds\n")
    (e2e / "step_definitions/rev_steps.py").write_text(textwrap.dedent('''\
        from pytest_bdd import given, parsers, then, when

        @given(parsers.parse("a buyer on {n:d} devices"))
        def buyer(world, n):
            world.n = n

        @when("the buyer revokes")
        def revoke(world):
            world.r = 1

        @then(parsers.parse("the token gets 401 within {s:d} seconds"))
        def rejected(world, s):
            assert world.r == 1
        '''))


def test_build_set_maps_spec_to_feature_and_steps(tmp_path):
    make_repo(tmp_path)
    out = tmp_path / "metrics/kev-eval/states/clean"
    s1 = build_set.build(tmp_path, None, 1, out)
    assert s1["selected"] == 1 and s1["exclusions"]["no_feature_match"] == 1
    u = build_set.load_clean(out)[0]
    assert set(u["parts"]) == {"SPEC", "FEATURE", "STEPS", "IMPL"}
    assert "Scenario: A session is revoked" in u["parts"]["FEATURE"]
    assert "def rejected" in u["parts"]["STEPS"] and "def buyer" in u["parts"]["STEPS"]
    assert u["feature_id"] == "a.b" and u["impl_origin"] == "synthetic"
    before = (out / f"{u['id']}.json").read_text()
    build_set.build(tmp_path, None, 1, out)
    assert (out / f"{u['id']}.json").read_text() == before  # deterministic
    # a Then step that hides its assertion in a helper is excluded unless asked
    (tmp_path / "platform-e2e/tests/e2e/step_definitions/rev_steps.py").write_text(
        (tmp_path / "platform-e2e/tests/e2e/step_definitions/rev_steps.py").read_text().replace("assert world.r == 1", "helper(world)"))
    assert build_set.build(tmp_path, None, 1, out)["selected"] == 0
    assert build_set.build(tmp_path, None, 1, out, require_assertions=False)["selected"] == 1


# ------------------------------------------------------------ end-to-end ---
@pytest.fixture()
def env(tmp_path):
    units, labels = seed.seed(synthetic_clean(), 3, per_defect=10, gate=GATE)
    labels["gate"] = "gate.yaml"
    (tmp_path / "gate.yaml").write_text(GATE_YAML)
    seed.write_set("syn", units, labels, tmp_path)
    return tmp_path, units, labels


def cli(root, *args):
    return run.main(["--root", str(root), "--set", "syn", *args])


def rep(root, engine="mock", **kw):
    r = score.build_report("syn", engine, root, GATE, kw.get("max_miss", 0.05), kw.get("max_ff", 0.05))
    return r


def test_perfect_mock_gives_perfect_scores_and_blind_out_of_checklist(env):
    root, units, labels = env
    ans = mock_answers.make_answers(labels, GATE, noise=0.0)
    c.write_json(root / "ans.json", ans)
    assert cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json")) == 0
    r = rep(root)
    pq = r["arms"]["C"]["per_question"]
    for q, m in pq.items():
        if m["tp"] + m["fn"]:
            assert m["recall"] == 1.0 and m["precision"] == 1.0 and m["f1"] == 1.0, q
    assert r["arms"]["C"]["unit_level"]["recall"] == 1.0
    oc = r["arms"]["C"]["out_of_checklist"]
    assert oc["n"] > 0 and oc["catch_rate"] == 0.0 and oc["false_alarm_rate"] == 0.0
    assert r["arms"]["C"]["llm_tokens_est"] == 0
    assert score.to_markdown(r).startswith("# Kev eval report")


def test_recall_precision_math_with_known_errors(env):
    root, units, labels = env
    ans = mock_answers.make_answers(labels, GATE, noise=0.0)
    pos = [u for u, m in labels["units"].items() if "assertion_kind" in m["expect_fail"]]
    neg = [u for u, m in labels["units"].items() if m["group"] == "clean"]
    P, N = len(pos), len(neg)
    # one miss (positive answered `real`) and two false alarms (clean answered `proxy`)
    ans[pos[0]]["assertion_kind"] = {"choice": "real", "confidence": 0.9}
    for u in neg[:2]:
        ans[u]["assertion_kind"] = {"choice": "proxy", "confidence": 0.9}
    c.write_json(root / "ans.json", ans)
    cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json"))
    m = rep(root)["arms"]["C"]["per_question"]["assertion_kind"]
    tp, fp, fn = P - 1, 2, 1
    assert (m["tp"], m["fp"], m["fn"]) == (tp, fp, fn)
    assert m["recall"] == pytest.approx(tp / (tp + fn)) and m["precision"] == pytest.approx(tp / (tp + fp))
    assert m["f1"] == pytest.approx(2 * tp / (2 * tp + fp + fn))
    # a flag on a tolerated question is not a false positive
    tol = next(u for u, mm in labels["units"].items() if "promise_match" in mm["tolerated"] and "promise_match" not in mm["expect_fail"])
    ans[tol]["promise_match"] = {"score": 0, "level": "unrelated", "confidence": 0.9}
    c.write_json(root / "ans.json", ans)
    cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json"), "--rerun")
    assert rep(root)["arms"]["C"]["per_question"]["promise_match"]["fp"] == 0


def test_arm_b_escalates_uncertain_and_llm_answers_recover_recall(env):
    root, units, labels = env
    ans = mock_answers.make_answers(labels, GATE, noise=0.0)
    pos = [u for u, m in labels["units"].items() if "assertion_kind" in m["expect_fail"]]
    # Kev unsure about two positive units (confidence below min_confidence 0.6)
    for u in pos[:2]:
        ans[u]["assertion_kind"] = {"choice": "real", "confidence": 0.4}
    c.write_json(root / "ans.json", ans)
    cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json"))
    cli(root, "--arm", "B", "--engine", "mock", "--answers", str(root / "ans.json"))
    esc = c.read_json(c.run_dir("syn", "B", "mock", root) / "escalations.json")
    assert {e["unit"]: e["questions"] for e in esc} == {u: ["assertion_kind"] for u in pos[:2]}
    assert all(e["tokens_est"] > 0 and "assertion_kind" in e["prompt"] for e in esc)
    r = rep(root)
    assert r["arms"]["B"]["llm_answers_ingested"] is False
    assert r["arms"]["B"]["escalation"]["units"] == 2
    kev_only = r["arms"]["C"]["per_question"]["assertion_kind"]["recall"]
    assert kev_only < 1.0
    # the orchestrator's LLM answers the escalated question correctly (oracle derived from labels)
    llm = {e["unit"]: {q: "fail" for q in e["questions"]} for e in esc}
    c.write_json(root / "llm.json", llm)
    cli(root, "--arm", "B", "--engine", "mock", "--llm-answers", str(root / "llm.json"))
    r = rep(root)
    assert r["arms"]["B"]["llm_answers_ingested"] and r["arms"]["B"]["pending_questions"] == 0
    assert r["arms"]["B"]["per_question"]["assertion_kind"]["recall"] == 1.0
    assert r["arms"]["B"]["per_question"]["assertion_kind"]["precision"] == 1.0
    assert r["arms"]["B"]["llm_tokens_est"] == sum(e["tokens_est"] for e in esc)


def test_arm_a_prompts_and_ingest(env):
    root, units, labels = env
    assert cli(root, "--arm", "A", "--emit-prompts") == 0
    d = c.run_dir("syn", "A", "llm", root)
    idx = c.read_json(d / "prompts_index.json")
    assert len(idx) == len(units) and all((d / v["file"]).exists() and v["tokens_est"] > 0 for v in idx.values())
    text = (d / next(iter(idx.values()))["file"]).read_text()
    assert "assertion_kind" in text and "## STEPS" in text and '"findings"' in text
    findings = {u: {"findings": m["expect_fail"], "other": ["off-by-one in limit"] if m["defect"] == "off_by_one_impl" else []}
                for u, m in labels["units"].items()}
    findings[next(iter(findings))]["findings"].append("not_a_question")
    c.write_json(root / "a.json", findings)
    assert cli(root, "--arm", "A", "--ingest", str(root / "a.json")) == 0
    r = rep(root)
    a = r["arms"]["A"]
    assert a["unit_level"]["recall"] == 1.0 and a["llm_tokens_est"] == sum(v["tokens_est"] for v in idx.values())
    oc = a["out_of_checklist"]
    n_off = sum(1 for m in labels["units"].values() if m["defect"] == "off_by_one_impl")
    n_oc = sum(1 for m in labels["units"].values() if m["group"] == "out_of_checklist")
    assert oc["catch_rate"] == pytest.approx(n_off / n_oc)  # only the free-text `other` findings


def test_engine_down_is_reported_not_scored(env):
    root, units, labels = env
    ans = mock_answers.make_answers(labels, GATE)
    ans[units[0]["id"]] = {"__down__": True}
    c.write_json(root / "ans.json", ans)
    assert cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json")) == 3
    assert units[0]["id"] not in score.load_preds("syn", "mock", root, GATE)["C"]["pred"]


# ------------------------------------------------------- threshold sweep ---
def mk_labels(pos, neg, q="mystery_guest"):
    u = {}
    for i in pos:
        u[i] = {"expect_fail": [q], "tolerated": []}
    for i in neg:
        u[i] = {"expect_fail": [], "tolerated": []}
    return {"units": u}


def test_threshold_sweep_noul():
    q = next(x for x in GATE["questions"] if x["id"] == "mystery_guest")  # pass_when false: p_pass = 1 - yes
    # positives (should fail) have p_pass 0.1 / 0.2 / 0.4 ; negatives 0.95 / 0.9 / 0.6
    pv = {"p1": 0.1, "p2": 0.2, "p3": 0.4, "n1": 0.95, "n2": 0.9, "n3": 0.6}
    raws = {k: {"yes": round(1 - v, 4)} for k, v in pv.items()}
    labels = mk_labels(["p1", "p2", "p3"], ["n1", "n2", "n3"])
    sw = score.sweep(q, raws, labels, max_miss=0.0, max_ff=0.0)
    at = {p["threshold"]: p for p in sw["curve"]}
    # t=0.85: p1 fails (p<=.15), p2 (.2) and p3 (.4) uncertain, n1/n2 pass, n3 uncertain
    a = at[0.85]
    assert a["recall"] == pytest.approx(1 / 3) and a["precision"] == 1.0 and a["miss_rate"] == 0.0
    assert a["false_fail_rate"] == 0.0 and a["escalation_rate"] == pytest.approx(3 / 6)
    assert a["recall_with_ideal_llm"] == 1.0
    # t=0.6: everything decided correctly -> zero escalation, zero miss, zero false fail
    b = at[0.6]
    assert (b["recall"], b["precision"], b["escalation_rate"], b["miss_rate"]) == (1.0, 1.0, 0.0, 0.0)
    # t=0.5 decides everything too, but 0.6 is the most conservative value with zero escalation
    assert sw["feasible"] is True and sw["suggested"] == 0.6 and sw["positives"] == 3


def test_threshold_sweep_choice_and_infeasible():
    q = next(x for x in GATE["questions"] if x["id"] == "assertion_kind")
    raws = {
        "p1": {"choice": "proxy", "confidence": 0.9}, "p2": {"choice": "proxy", "confidence": 0.7},
        "p3": {"choice": "real", "confidence": 0.995},  # confident miss above the whole grid: no threshold fixes it
        "n1": {"choice": "real", "confidence": 0.9}, "n2": {"choice": "real", "confidence": 0.5},
    }
    labels = mk_labels(["p1", "p2", "p3"], ["n1", "n2"], "assertion_kind")
    sw = score.sweep(q, raws, labels, 0.05, 0.05)
    assert sw["key"] == "min_confidence" and sw["positives"] == 3 and sw["feasible"] is False
    at = {p["min_confidence"]: p for p in sw["curve"]}
    assert at[0.8]["escalation_rate"] == pytest.approx(2 / 5)  # p2 (0.7) and n2 (0.5)
    assert at[0.8]["miss_rate"] == pytest.approx(1 / 3) and at[0.8]["recall"] == pytest.approx(1 / 3)
    assert score.verdict_for(sw) == "keep LLM"
    # without the confident miss a threshold exists
    del raws["p3"]
    labels["units"].pop("p3")
    sw = score.sweep(q, raws, labels, 0.05, 0.05)
    assert sw["feasible"] is True and sw["positives"] == 2
    assert score.verdict_for(sw) == "insufficient positives"


def test_report_suggests_thresholds_from_noisy_run(env):
    root, units, labels = env
    ans = mock_answers.make_answers(labels, GATE, noise=0.2, flip=0.0, seed=5)
    c.write_json(root / "ans.json", ans)
    cli(root, "--arm", "C", "--engine", "mock", "--answers", str(root / "ans.json"))
    r = rep(root)
    for q in ("assertion_kind", "boundary_values"):
        sw = r["thresholds"][q]
        assert sw["positives"] > 0 and sw["suggested"] is not None and len(sw["curve"]) == 100
    md = score.to_markdown(r)
    assert "## Thresholds" in md and "assertion_kind" in md
    assert score.main(["--root", str(root), "--set", "syn", "--engine", "mock"]) == 0
    assert (root / "metrics/kev-eval/syn/report.md").exists() and (root / "metrics/kev-eval/syn/report.json").exists()


def test_gate_file_is_untouched_by_the_harness(env):
    root, _, _ = env
    assert (root / "gate.yaml").read_text() == GATE_YAML
