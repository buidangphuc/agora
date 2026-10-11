import copy
import sys
from pathlib import Path

import pytest
import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import check_gates

REPO = Path(__file__).resolve().parents[3]

VALID = {
    "gate": "feature",
    "version": 1,
    "pins": {"asvs": "4.0.3"},
    "state_unit": "scenario",
    "max_state_tokens": 8000,
    "questions": [
        {"id": "checks_store", "source": "ISO/IEC/IEEE 29119-4:2021 Equivalence partitioning",
         "type": "noul", "severity": "veto", "instructions": "Does it?",
         "pass_when": True, "threshold": 0.85},
        {"id": "assertion_kind", "source": "OWASP ASVS 4.0.3 V4.2.1",
         "type": "choice", "severity": "normal", "instructions": "What?",
         "criteria": {"real": "real outcome", "mock": "mock called"},
         "pass_values": ["real"], "min_confidence": 0.6},
        {"id": "promise_match", "source": "Google Engineering Practices - Tests",
         "type": "score", "severity": "veto", "instructions": "How well?",
         "levels": {"none": "nothing", "full": "everything"},
         "pass_min": "full", "min_confidence": 0.6},
    ],
}


def write(tmp_path, data, name="feature.v1.yaml"):
    p = tmp_path / name
    p.write_text(yaml.safe_dump(data, sort_keys=False), encoding="utf-8")
    return p


def errors_for(tmp_path, mutate, name="feature.v1.yaml"):
    data = copy.deepcopy(VALID)
    mutate(data)
    return check_gates.check_file(write(tmp_path, data, name))


def test_repo_gate_files_are_valid():
    files = sorted((REPO / "gates").glob("*.v*.yaml"))
    assert {f.name for f in files} >= {"feature.v1.yaml", "task.v1.yaml",
                                       "spec.v1.yaml", "release.v1.yaml"}
    for f in files:
        assert check_gates.check_file(f) == [], f.name
    assert check_gates.main([]) == 0


def test_valid_minimal_file(tmp_path):
    p = write(tmp_path, VALID)
    assert check_gates.check_file(p) == []
    assert check_gates.main([str(p)]) == 0


@pytest.mark.parametrize("mutate, needle", [
    (lambda d: d.pop("state_unit"), "missing required key 'state_unit'"),
    (lambda d: d.update(examples=[]), "unknown top-level key 'examples'"),
    (lambda d: d.update(gate="deploy"), "gate 'deploy' not in"),
    (lambda d: d.update(state_unit="file"), "state_unit 'file' not in"),
    (lambda d: d.update(max_state_tokens=0), "max_state_tokens"),
    (lambda d: d.update(questions=[]), "questions must be a non-empty list"),
    (lambda d: d["questions"][1].update(id="checks_store"), "duplicate question id"),
    (lambda d: d["questions"][0].update(id="ChecksStore"), "snake_case"),
    (lambda d: d["questions"][0].update(severity="high"), "severity 'high'"),
    (lambda d: d["questions"][0].update(type="bool"), "type 'bool'"),
    (lambda d: d["questions"][0].update(source=""), "source must be a non-empty string"),
    (lambda d: d["questions"][0].pop("source"), "missing required key 'source'"),
    (lambda d: d["questions"][0].update(threshold=0.5), "threshold must be in (0.5, 1)"),
    (lambda d: d["questions"][0].update(threshold=1.0), "threshold must be in (0.5, 1)"),
    (lambda d: d["questions"][0].update(pass_when="yes"), "pass_when must be true or false"),
    (lambda d: d["questions"][0].pop("threshold"), "noul question missing 'threshold'"),
    (lambda d: d["questions"][0].update(examples=["x"]), "key 'examples' not allowed"),
    (lambda d: d["questions"][1].update(pass_values=["fake"]), "not in criteria"),
    (lambda d: d["questions"][1].update(pass_values=["real", "mock"]), "can never fail"),
    (lambda d: d["questions"][1]["criteria"].update(mock=""), "has no description"),
    (lambda d: d["questions"][1].update(min_confidence=0.3), "min_confidence must be in"),
    (lambda d: d["questions"][2].update(pass_min="most"), "pass_min 'most' not in levels"),
    (lambda d: d["questions"][2].update(pass_min="none"), "can never fail"),
    (lambda d: d["questions"][2].update(criteria={}), "not allowed for type score"),
    (lambda d: d.pop("pins"), "cites ASVS but pins.asvs is not set"),
])
def test_invalid_files_are_reported(tmp_path, mutate, needle):
    errs = errors_for(tmp_path, mutate)
    assert any(needle in e for e in errs), errs


def test_file_name_must_match_gate_and_version(tmp_path):
    errs = errors_for(tmp_path, lambda d: d.update(version=2))
    assert any("file name must be 'feature.v2.yaml'" in e for e in errs)
    errs = check_gates.check_file(write(tmp_path, VALID, "feature.yaml"))
    assert errs and "does not match" in errs[0]


def test_duplicate_yaml_keys_rejected(tmp_path):
    p = tmp_path / "feature.v1.yaml"
    p.write_text("gate: feature\ngate: task\n", encoding="utf-8")
    errs = check_gates.check_file(p)
    assert errs and "duplicate key" in errs[0]


def test_main_exits_1_on_invalid(tmp_path, capsys):
    data = copy.deepcopy(VALID)
    data["questions"][0]["severity"] = "high"
    assert check_gates.main([str(write(tmp_path, data))]) == 1
    assert "severity 'high'" in capsys.readouterr().out
