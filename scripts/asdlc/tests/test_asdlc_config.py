import copy
import sys
from pathlib import Path

import pytest
import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
import asdlc_config as c  # noqa: E402


@pytest.fixture
def raw():
    return yaml.safe_load(c.DEFAULT_PATH.read_text(encoding="utf-8"))


def test_real_file_is_valid(raw):
    assert c.validate(raw) == []


def test_load_merges_active_profile():
    cfg = c.load()
    assert cfg["profile"] == "solo" and "profiles" not in cfg
    assert cfg["wip_limit"] >= 1
    assert c.load("team")["profile"] == "team"


def test_unknown_profile_rejected():
    with pytest.raises(ValueError):
        c.load("nope")


@pytest.mark.parametrize("val", [0.5, 1, 0.3, "0.9", True])
def test_bad_threshold(raw, val):
    raw["thresholds"]["noul"] = val
    assert any("thresholds.noul" in e for e in c.validate(raw))


def test_unknown_kev_mode(raw):
    raw["profiles"]["team"]["kev_mode"] = "block"
    assert any("kev_mode" in e for e in c.validate(raw))


def test_mismatched_profile_keys(raw):
    del raw["profiles"]["team"]["approvals"]["high"]
    errs = c.validate(raw)
    assert any("key set differs" in e for e in errs)


def test_extra_key_in_one_profile(raw):
    raw["profiles"]["solo"]["extra"] = 1
    assert any("key set differs" in e for e in c.validate(copy.deepcopy(raw)))


def test_gate_without_reviewers(raw):
    raw["profiles"]["solo"]["reviewers"]["spec"] = []
    assert any("reviewers.spec" in e for e in c.validate(raw))


def test_route_hits():
    assert c.route(["team-payment/svc/pay.go"]) == {"payment"}
    assert c.route(["platform-core/packages/proto/a/v1/a.proto"]) == {"proto"}
    assert {"auth", "pii"} <= c.route(["team-identity/main.go"])
    assert "auth" in c.route(["team-x/internal/interceptor/jwt.go"])
    assert "pii" in c.route(["web/src/address_form.ts"])
    assert c.route(["README.md", "team-listing/main.go"]) == set()


def test_route_release_and_case_insensitive():
    assert "release" in c.route(["platform-gitops/envs/prod/app.yaml"])
    assert "release" in c.route(["deploy/images/push-images.sh"])
    assert "auth" in c.route(["team-x/JWT_Verify.go"])
    assert "auth" in c.route(["team-x/ScopeGate.go"])
    assert "release" in c.route(["./Deploy/Images/x.sh"])


def test_route_strips_only_literal_leading_dot_slash():
    cfg = c.load()
    cfg["routing"] = {"auth": [".hidden/*"], "pii": ["a/*"], "payment": ["x"], "proto": ["y"], "release": ["z"]}
    assert c.route([".hidden/f"], cfg) == {"auth"}      # lstrip("./") would have eaten the dot
    assert c.route(["./a/f"], cfg) == {"pii"}
