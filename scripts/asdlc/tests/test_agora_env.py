import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "devenv"))
import agora_env as ae  # noqa: E402

SERVICES = {
    "postgres": {"image": "postgres:16-alpine", "ports": [{"published": "5432", "target": 5432}]},
    "redpanda": {
        "image": "redpandadata/redpanda:v24.2.7",
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
    assert any("external://localhost:49092" in c for c in ov["redpanda"]["command"])
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
