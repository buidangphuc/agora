import json
from argparse import Namespace

from scripts.run_eval import DEFAULT_CASES, _run
from tests.factories import build_test_settings

JUDGE_CASES = "evals/completions_judge.jsonl"


def _args(cases=DEFAULT_CASES, min_score=0.8, allow_unjudged=False) -> Namespace:
    return Namespace(cases=cases, min_score=min_score, allow_unjudged=allow_unjudged)


def _report(capsys) -> dict:
    lines = [ln for ln in capsys.readouterr().out.splitlines() if ln.startswith("{")]
    return json.loads(lines[-1])


async def test_eval_gate_passes_on_default_deterministic_datasets(capsys):
    exit_code = await _run(_args(), settings=build_test_settings())
    report = _report(capsys)

    assert exit_code == 0
    assert report["pass_rate"] == 1.0
    assert report["unscored"] == []
    assert report["cases"] == report["scored"] == report["passed"] == 13


async def test_default_datasets_include_the_chat_resilience_cases(capsys):
    await _run(
        _args(cases=["evals/chat_resilience.jsonl"]), settings=build_test_settings()
    )
    report = _report(capsys)

    assert report["cases"] >= 6
    assert report["failures"] == []


async def test_eval_gate_fails_below_threshold():
    exit_code = await _run(_args(min_score=1.01), settings=build_test_settings())
    assert exit_code == 1


async def test_judge_case_without_a_judge_exits_nonzero_and_names_the_cases(capsys):
    exit_code = await _run(
        _args(cases=[JUDGE_CASES]), settings=build_test_settings(JUDGE_CHAT_MODEL="")
    )
    out = capsys.readouterr().out
    report = json.loads([ln for ln in out.splitlines() if ln.startswith("{")][-1])

    assert exit_code == 2
    assert report["unscored"] == ["judge-verbatim", "judge-no-hallucination"]
    assert "judge-verbatim" in out and "JUDGE_CHAT_MODEL" in out


async def test_unscored_cases_count_as_failed_in_the_pass_rate(capsys):
    # 13 deterministic passes + 2 unscored judge cases => 13/15, not 13/13.
    exit_code = await _run(
        _args(cases=[*DEFAULT_CASES, JUDGE_CASES], min_score=0.0),
        settings=build_test_settings(),
    )
    report = _report(capsys)

    assert exit_code == 2  # unscored judge cases still fail the gate
    assert report["pass_rate"] == round(13 / 15, 4)
    assert report["unscored"] == ["judge-verbatim", "judge-no-hallucination"]


async def test_allow_unjudged_is_a_local_opt_out_that_skips_them(capsys):
    exit_code = await _run(
        _args(cases=[*DEFAULT_CASES, JUDGE_CASES], allow_unjudged=True),
        settings=build_test_settings(),
    )
    report = _report(capsys)

    assert exit_code == 0
    assert report["pass_rate"] == 1.0
    assert report["allow_unjudged"] is True


async def test_unknown_evaluator_is_unscored_and_fails(tmp_path, capsys):
    cases = tmp_path / "x.jsonl"
    cases.write_text(
        json.dumps(
            {
                "id": "weird",
                "input": {"messages": [{"role": "user", "content": "hi"}]},
                "expected": "echo: hi",
                "metadata": {"evaluator": "nope"},
            }
        )
        + "\n"
    )

    exit_code = await _run(_args(cases=[str(cases)]), settings=build_test_settings())

    assert exit_code == 2
    assert _report(capsys)["unscored"] == ["weird"]


async def test_a_broken_resilience_expectation_fails_the_gate(tmp_path, capsys):
    # Guards against a vacuous chat eval: a wrong expectation must be caught.
    case = {
        "id": "wrong-expectation",
        "input": {
            "chain": ["a", "b"],
            "scripts": {
                "a": [{"fail_at": 0, "status": 429}],
                "b": [{"chunks": ["hi"]}],
            },
            "requests": [{"message": "hello"}],
        },
        "expected": "reply=hi err=none calls=a",  # really a,b
        "metadata": {"evaluator": "exact", "target": "chat_resilience"},
    }
    path = tmp_path / "c.jsonl"
    path.write_text(json.dumps(case) + "\n")

    exit_code = await _run(_args(cases=[str(path)]), settings=build_test_settings())

    assert exit_code == 1
    assert "wrong-expectation" in _report(capsys)["failures"][0]


async def test_report_lists_the_chat_resilience_cases_as_passed(capsys):
    exit_code = await _run(_args(), settings=build_test_settings())
    out = capsys.readouterr().out

    assert exit_code == 0
    for case_id in (
        "chat-primary-429-falls-back",
        "chat-all-targets-fail",
        "chat-break-after-first-chunk-no-mixed-model",
    ):
        assert f"PASS {case_id}" in out


async def test_eval_extra_cases_env_adds_a_dataset(tmp_path, monkeypatch, capsys):
    extra = tmp_path / "extra.jsonl"
    extra.write_text(
        json.dumps(
            {
                "id": "extra-judge",
                "input": {"messages": [{"role": "user", "content": "hi"}]},
                "expected": "be nice",
                "metadata": {"evaluator": "judge"},
            }
        )
        + "\n"
    )
    monkeypatch.setenv("EVAL_EXTRA_CASES", str(extra))

    exit_code = await _run(_args(), settings=build_test_settings(JUDGE_CHAT_MODEL=""))
    out = capsys.readouterr().out
    report = _report_from(out)

    assert exit_code == 2
    assert report["unscored"] == ["extra-judge"]
    assert report["pass_rate"] < 1.0
    assert "UNSCORED extra-judge" in out
    assert "extra-judge" in out and "JUDGE_CHAT_MODEL" in out


def _report_from(out: str) -> dict:
    return json.loads([ln for ln in out.splitlines() if ln.startswith("{")][-1])
