"""Tests for per-kind minimum test evidence (features.py --test-levels)."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import features  # noqa: E402


def feat(**kw):
    base = {"id": "a.b", "status": "automated", "covered_by": "x.feature::S"}
    base.update(kw)
    return base


def errs(tmp_path, **kw):
    return features.test_level_errors("repo", feat(**kw), tmp_path)


def touch(tmp_path, rel):
    p = tmp_path / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("")


def test_default_kind_needs_unit(tmp_path):
    assert any("tests.unit" in e for e in errs(tmp_path))
    touch(tmp_path, "svc/u_test.go")
    assert errs(tmp_path, tests={"unit": "svc/u_test.go"}) == []


def test_non_automated_skipped(tmp_path):
    assert errs(tmp_path, status="planned", covered_by=None) == []


def test_authz_only_needs_e2e(tmp_path):
    assert errs(tmp_path, kind="authz") == []
    assert errs(tmp_path, kind="authz", covered_by=None)


def test_invariant_needs_unit_and_property(tmp_path):
    touch(tmp_path, "u.py")
    e = errs(tmp_path, kind="invariant", tests={"unit": "u.py"})
    assert len(e) == 1 and "tests.property" in e[0]


def test_contract_async_ml(tmp_path):
    assert any("tests.contract" in e for e in errs(tmp_path, kind="contract"))
    assert any("tests.integration" in e for e in errs(tmp_path, kind="async_event"))
    e = errs(tmp_path, kind="ml_pipeline")
    assert len(e) == 2


def test_dangling_path_and_name_suffix(tmp_path):
    touch(tmp_path, "t/it.py")
    assert errs(tmp_path, kind="async_event", tests={"integration": ["t/it.py::test_x"]}) == []
    e = errs(tmp_path, kind="async_event", tests={"integration": "t/missing.py"})
    assert any("path not found: t/missing.py" in x for x in e)


def test_schema_accepts_new_fields():
    v = features.load_schema_validator()
    doc = {
        "repo": "r",
        "features": [
            {
                "id": "a.b",
                "title": "t",
                "persona": "buyer",
                "entry_route": "/",
                "acceptance": ["g"],
                "status": "planned",
                "kind": "invariant",
                "tests": {"unit": "p", "property": ["q"]},
            }
        ],
    }
    assert list(v.iter_errors(doc)) == []
    doc["features"][0]["kind"] = "bogus"
    assert list(v.iter_errors(doc))
