import json
import sys
import types
from pathlib import Path

import pytest
import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "devenv"))
import agora_env as ae  # noqa: E402

SERVICES = {
    "postgres": {"image": "postgres:16-alpine", "ports": [{"published": "5432", "target": 5432}]},
    "redpanda": {
        "image": "redpandadata/redpanda:v24.3.1",
        "ports": [{"published": "19092", "target": 19092}],
        "command": ["redpanda", "start", "--advertise-kafka-addr=internal://redpanda:9092,external://localhost:19092"],
    },
    "otel-collector": {"image": "otel/opentelemetry-collector-contrib:0.104.0"},
    "team-identity-migrate": {"image": "team-identity:local", "depends_on": {"postgres": {}}},
    "team-identity": {
        "image": "team-identity:local",
        "build": {"context": "/r/team-identity"},
        "container_name": "team-identity-svc",
        "depends_on": {"team-identity-migrate": {}},
        "environment": {"KAFKA_BROKERS": "redpanda:9092", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector:4317"},
    },
    "team-order": {
        "image": "team-order:local",
        "build": {"context": "/r/team-order"},
        "container_name": "team-order-svc",
        "environment": {"UPSTREAM_IDENTITY_ADDR": "team-identity-svc:50053"},
    },
    "team-sharing": {"image": "team-sharing:local", "build": {"context": "/r/team-sharing"}, "container_name": "team-sharing-svc"},
    "team-gateway": {
        "image": "team-gateway:local",
        "build": {"context": "/r/team-gateway"},
        "container_name": "team-gateway-svc",
        "ports": [{"published": "8080", "target": 8080}],
        "environment": {
            "JWKS_URL": "http://team-identity-svc:50063/.well-known/jwks.json",
            "UPSTREAM_ORDER_ADDR": "team-order-svc:50055",
            "UPSTREAM_SHARING_ADDR": "team-sharing-svc:50065",
            "TRUSTED_PROXIES": "team-frontend-svc",
            "CORS_ORIGINS": "http://localhost:3000",
            "JAEGER_UI_URL": "http://localhost:16686",
        },
    },
}


def test_slug_drops_branch_prefix_and_normalises():
    assert ae.slug("feat/Q7_Q37") == "q7-q37"
    assert ae.slug("asdlc/q7-referral-welcome") == "q7-referral-welcome"


def test_closure_follows_depends_on_and_env_hosts_but_not_gateway_fanout():
    got = ae.closure(SERVICES, {"team-gateway", "team-sharing"})
    assert got == {"team-gateway", "team-sharing", "team-identity", "team-identity-migrate", "postgres", "redpanda"}


def test_closure_follows_owner_upstreams_and_optional_hosts_on_request():
    assert "team-identity" in ae.closure(SERVICES, {"team-order"})
    assert "otel-collector" in ae.closure(SERVICES, {"team-identity"}, with_optional=True)


def test_feature_port_offsets_and_stays_valid():
    assert ae.feature_port(8080, 0) == 38080
    assert ae.feature_port(50060, 0) == 50160
    assert ae.feature_port(8080, 1) == 39080


def test_override_renames_containers_keeps_aliases_and_retags_owners():
    pm = ae.port_map(SERVICES, 0)
    ov = ae.make_override("q7", SERVICES, ["team-identity"], pm)["services"]
    assert ov["team-identity"]["container_name"] == "q7-team-identity-svc"
    assert ov["team-identity"]["networks"]["default"]["aliases"] == ["team-identity-svc"]
    assert ov["team-identity"]["image"] == "team-identity:q7"
    assert ov["team-identity-migrate"]["image"] == "team-identity:q7"
    assert "image" not in ov["team-order"]


def test_override_remaps_ports_redpanda_and_localhost_env():
    pm = ae.port_map(SERVICES, 0)
    ov = ae.make_override("q7", SERVICES, [], pm)["services"]
    assert ov["team-gateway"]["ports"].value == ["127.0.0.1:38080:8080"]
    assert "environment" not in ov["team-gateway"]
    with_fe = {**SERVICES, "team-frontend": {"image": "team-frontend:local", "ports": [{"published": "3000", "target": 3000}]}}
    ov_fe = ae.make_override("q7", with_fe, [], ae.port_map(with_fe, 0))["services"]
    assert ov_fe["team-gateway"]["environment"] == {"CORS_ORIGINS": "http://localhost:33000"}
    assert any("external://127.0.0.1:49092" in c for c in ov["redpanda"]["command"])
    assert not any("external://localhost" in c for c in ov["redpanda"]["command"])
    dumped = yaml.dump(ae.make_override("q7", SERVICES, [], pm))
    assert "!override" in dumped


def test_rewrite_local_only_touches_published_ports():
    assert ae.rewrite_local("http://localhost:8080,http://localhost:1234", {8080: 38080}) == "http://localhost:38080,http://localhost:1234"


def test_owners_from_slices_skips_contract_owner(tmp_path):
    p = tmp_path / "slices.yaml"
    p.write_text("slices:\n  - {id: a, owner: platform-core}\n  - {id: b, owner: team-order}\n  - {id: c, owner: team-order}\n")
    assert ae.owners_from_slices(p) == ["team-order"]


def test_k8s_value_maps_container_names_redis_and_public_urls():
    v = ae.k8s_value("http://team-identity-svc:50063/x,redis:6379,redpanda.marketplace.svc:9092,http://localhost:3000", SERVICES, "fe.local", "api.local")
    assert v == "http://team-identity:50063/x,valkey:6379,redpanda:9092,http://fe.local"


def test_main_build_passes_frontend_build_args(monkeypatch):
    calls = []
    monkeypatch.setattr(ae, "image_id", lambda ref: None)
    monkeypatch.setattr(ae, "gen_stubs", lambda src, name: None)
    monkeypatch.setattr(ae, "compose", lambda st, *args, **kw: calls.append(args))
    services = {
        "team-frontend": {"image": "team-frontend:local", "build": {"context": "/r/team-frontend"}},
        "team-sharing": SERVICES["team-sharing"],
    }
    st = {"env": ae.MAIN, "src": "/r"}
    ae.build(st, services, ["team-frontend", "team-sharing"])
    fe = next(c for c in calls if c[-1] == "team-frontend")
    assert "NEXT_PUBLIC_GTM_ID=GTM-E2E0001" in fe
    assert "NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080" in fe
    assert next(c for c in calls if c[-1] == "team-sharing") == ("build", "team-sharing")


def test_frontend_build_args_carry_the_site_url():
    assert ae.frontend_args("team-frontend", {3000: 33000})["NEXT_PUBLIC_SITE_URL"] == "http://localhost:33000"
    assert ae.frontend_args("team-frontend", {})["NEXT_PUBLIC_SITE_URL"] == "http://localhost:3000"


def _exports(monkeypatch, tmp_path, env, ports):
    monkeypatch.setattr(ae, "config", lambda src, overlays: SERVICES)
    monkeypatch.setattr(ae, "state_dir", lambda e: tmp_path / e)
    st = {"env": env, "project": env, "src": str(tmp_path), "ports": ports, "owners": []}
    return ae.e2e_exports(st)


def test_feature_env_exports_ipv4_kafka_app_urls_and_redis_port(monkeypatch, tmp_path):
    ports = {"19092": 49092, "3000": 33000, "3001": 33001, "3002": 33002, "6379": 36379, "8080": 38080}
    e = _exports(monkeypatch, tmp_path, "q7", ports)
    assert e["KAFKA_BROKERS"] == "127.0.0.1:49092"
    assert e["SELLER_BASE_URL"] == "http://localhost:33001"
    assert e["ADMIN_BASE_URL"] == "http://localhost:33002"
    assert e["FSM_REDIS_PORT"] == "36379"


def test_main_env_keeps_default_redis_port_for_fsm(monkeypatch, tmp_path):
    e = _exports(monkeypatch, tmp_path, ae.MAIN, {})
    assert "FSM_REDIS_PORT" not in e
    assert e["ADMIN_BASE_URL"] == "http://localhost:3002"


def test_promote_builds_job_only_images_that_exist_locally():
    services = {
        "team-sharing": SERVICES["team-sharing"],
        "featurestore-job": {"image": "platform-featurestore:local", "build": {"context": "/r/platform-featurestore"}},
        "featurestore-dataset": {"image": "platform-featurestore:local"},
        "team-frontend-admin": {"image": "team-frontend-admin:local", "build": {"context": "/r/team-frontend-admin"}},
    }
    owners = ["team-sharing", "featurestore-job", "team-frontend-admin"]
    have = {"platform-featurestore"}
    got = ae.deployed_owners(services, owners, existing={"team-sharing"}, has_image=lambda img: img in have)
    assert got == ["featurestore-job", "team-sharing"]


def test_down_removes_leftover_job_containers_and_volumes(monkeypatch, tmp_path):
    calls = []
    monkeypatch.setattr(ae, "STATE_ROOT", tmp_path)
    monkeypatch.setattr(ae, "env_images", lambda env: [])

    def fake_out(cmd, cwd=None):
        calls.append(cmd)
        if cmd[:3] == ["docker", "ps", "-aq"]:
            return "c1\nc2\n"
        if cmd[:3] == ["docker", "volume", "ls"]:
            return "recs_data\n"
        return ""

    monkeypatch.setattr(ae, "out", fake_out)
    monkeypatch.setattr(ae, "run", lambda cmd, **kw: calls.append(cmd))
    ae.cmd_down(type("A", (), {"env": "recs-attrs"})())
    label = "label=com.docker.compose.project=recs-attrs"
    assert ["docker", "ps", "-aq", "--filter", label] in calls
    assert ["docker", "rm", "-f", "c1", "c2"] in calls
    assert ["docker", "volume", "rm", "recs_data"] in calls


def _promote_env(monkeypatch, tmp_path, running, migrate_rc=0, build_fails=False):
    monkeypatch.setattr(ae, "STATE_ROOT", tmp_path)
    (tmp_path / "main").mkdir()
    (tmp_path / "main" / "state.json").write_text(json.dumps({"env": "main", "deployed": "a" * 40}))
    src = tmp_path / "src"
    (src / "team-identity").mkdir(parents=True)
    services = {
        "postgres": SERVICES["postgres"],
        "redpanda": SERVICES["redpanda"],
        "redis": {"image": "redis:7"},
        "team-identity-migrate": {"image": "team-identity:local"},
        "team-identity": {"image": "team-identity:local", "build": {"context": str(src / "team-identity")}},
    }
    existing = {"postgres", "redpanda", "redis", "team-identity-migrate", "team-identity"}
    calls = []

    def fake_out(cmd, cwd=None):
        if cmd[:3] == ["git", "rev-parse", "origin/main"]:
            return "b" * 40 + "\n"
        if cmd[:3] == ["docker", "ps", "-a"]:
            return "\n".join(existing)
        if cmd[:2] == ["docker", "ps"]:
            return "\n".join(running)
        return ""

    def fake_run(cmd, cwd=None, check=True, capture=False, env=None):
        calls.append(cmd)
        rc = migrate_rc if "team-identity-migrate" in cmd else 0
        if check and rc:
            raise SystemExit(1)
        return types.SimpleNamespace(returncode=rc, stdout="", stderr="")

    def fake_build(st, svcs, owners):
        if build_fails:
            raise SystemExit("agora-env: failed: docker compose build")
        return []

    monkeypatch.setattr(ae, "out", fake_out)
    monkeypatch.setattr(ae, "run", fake_run)
    monkeypatch.setattr(ae, "export_main", lambda sha: src)
    monkeypatch.setattr(ae, "config", lambda s, o: services)
    monkeypatch.setattr(ae, "changed_files", lambda old, new: ["team-identity/main.go"])
    monkeypatch.setattr(ae, "image_id", lambda ref: "sha256:x")
    monkeypatch.setattr(ae, "build", fake_build)
    monkeypatch.setattr(ae, "drop_unused", lambda ids: None)
    args = types.SimpleNamespace(ref="origin/main", since=None, overlay=None, services=None, dry_run=False)
    return args, calls, tmp_path / "main" / "state.json"


def test_promote_keeps_deployed_when_build_fails(monkeypatch, tmp_path):
    args, _, state = _promote_env(monkeypatch, tmp_path, running={"postgres", "redpanda", "redis"}, build_fails=True)
    with pytest.raises(SystemExit):
        ae.cmd_promote(args)
    assert json.loads(state.read_text())["deployed"] == "a" * 40


def test_promote_fails_and_keeps_deployed_when_a_migrate_fails(monkeypatch, tmp_path):
    args, _, state = _promote_env(monkeypatch, tmp_path, running={"postgres", "redpanda", "redis"}, migrate_rc=1)
    with pytest.raises(SystemExit) as e:
        ae.cmd_promote(args)
    assert "team-identity-migrate" in str(e.value)
    assert json.loads(state.read_text())["deployed"] == "a" * 40


def test_promote_refuses_when_infra_is_down(monkeypatch, tmp_path):
    args, calls, state = _promote_env(monkeypatch, tmp_path, running=set())
    with pytest.raises(SystemExit) as e:
        ae.cmd_promote(args)
    msg = str(e.value)
    assert "postgres" in msg and "redis" in msg and "redpanda" in msg and "start" in msg
    assert not any("team-identity-migrate" in c for c in calls)
    assert json.loads(state.read_text())["deployed"] == "a" * 40


def test_promote_records_deployed_on_success(monkeypatch, tmp_path):
    args, _, state = _promote_env(monkeypatch, tmp_path, running={"postgres", "redpanda", "redis", "team-identity"})
    ae.cmd_promote(args)
    assert json.loads(state.read_text())["deployed"] == "b" * 40
