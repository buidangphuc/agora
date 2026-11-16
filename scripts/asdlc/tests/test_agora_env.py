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


def test_closure_adds_the_init_job_of_every_service_it_starts():
    services = {
        **SERVICES,
        "minio": {"image": "minio/minio"},
        "minio-init": {"image": "minio/mc", "depends_on": {"minio": {}}},
        "mlflow": {"image": "mlflow", "depends_on": {"minio": {}}},
    }
    assert ae.closure(services, {"mlflow"}) == {"mlflow", "minio", "minio-init"}
    assert "minio-init" not in ae.closure(services, {"team-sharing"})


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


def test_merge_overlays_keeps_recorded_and_appends_new():
    assert ae.merge_overlays(None, None) == ae.FEATURE_OVERLAYS
    prev = [*ae.FEATURE_OVERLAYS, "platform-e2e/compose/modelserve.override.yaml"]
    got = ae.merge_overlays(prev, ["platform-e2e/compose/recs.override.yaml", "platform-e2e/compose/modelserve.override.yaml"])
    assert got == [*prev, "platform-e2e/compose/recs.override.yaml"]


def test_changed_builders_maps_files_to_build_dirs():
    files = ["team-sharing/internal/x.go", "docs/readme.md", "team-gateway/go.mod"]
    assert ae.changed_builders(SERVICES, "/r", files) == {"team-sharing", "team-gateway"}
    assert ae.changed_builders(SERVICES, "/r", ["openspec/x.md"]) == set()


def test_changed_builders_rebuilds_proto_consumers_on_contract_change(tmp_path):
    (tmp_path / "team-order").mkdir()
    (tmp_path / "team-order" / "buf.gen.yaml").write_text("version: v2\n")
    (tmp_path / "team-sharing").mkdir()
    services = {
        "team-order": {"image": "team-order:local", "build": {"context": str(tmp_path / "team-order")}},
        "team-sharing": {"image": "team-sharing:local", "build": {"context": str(tmp_path / "team-sharing")}},
    }
    got = ae.changed_builders(services, tmp_path, ["platform-core/packages/proto/platform/order/v1/order.proto"])
    assert got == {"team-order"}


def test_one_per_image_prefers_the_service_named_like_its_dir():
    services = {
        "team-order": {"image": "team-order:local", "build": {"context": "/r/team-order"}},
        "team-order-consumer": {"image": "team-order:local", "build": {"context": "/r/team-order"}},
        "team-sharing": SERVICES["team-sharing"],
    }
    assert ae.one_per_image(services, {"team-order-consumer", "team-order", "team-sharing"}, "/r") == ["team-order", "team-sharing"]


def _plan_args(**kw):
    base = {"src": "/r", "services": None, "slices": None, "with_": None, "with_optional": False}
    return types.SimpleNamespace(**{**base, **kw})


def test_plan_builds_branch_changes_without_slices(monkeypatch):
    monkeypatch.setattr(ae, "config", lambda src, overlays: SERVICES)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: ["team-sharing/main.go"])
    _, _, owners, run_set = ae.cmd_plan(_plan_args())
    assert owners == ["team-sharing"]
    assert {"team-sharing", "team-gateway", "team-identity", "postgres"} <= set(run_set)


def test_plan_adds_explicit_services_to_branch_changes(monkeypatch):
    monkeypatch.setattr(ae, "config", lambda src, overlays: SERVICES)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: ["team-sharing/main.go"])
    _, _, owners, _ = ae.cmd_plan(_plan_args(services=["team-order"]))
    assert owners == ["team-order", "team-sharing"]


def test_plan_without_changes_or_names_exits(monkeypatch):
    monkeypatch.setattr(ae, "config", lambda src, overlays: SERVICES)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: [])
    with pytest.raises(SystemExit):
        ae.cmd_plan(_plan_args())


def _up_env(monkeypatch, tmp_path, prev, services=SERVICES, seed_ok=True):
    monkeypatch.setattr(ae, "STATE_ROOT", tmp_path)
    if prev is not None:
        (tmp_path / "feat").mkdir()
        (tmp_path / "feat" / "state.json").write_text(json.dumps(prev))
    seen = {"config": [], "seeded": 0}

    def fake_config(src, overlays):
        seen["config"].append(list(overlays))
        return services

    def fake_seed(st):
        seen["seeded"] += 1
        return "run1" if seed_ok else None

    monkeypatch.setattr(ae, "config", fake_config)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: ["team-sharing/main.go"])
    monkeypatch.setattr(ae, "out", lambda cmd, cwd=None: "feat\n")
    monkeypatch.setattr(ae, "listening_ports", lambda: set())
    monkeypatch.setattr(ae, "compose", lambda st, *a, **kw: types.SimpleNamespace(returncode=0, stdout=""))
    monkeypatch.setattr(ae, "check_images", lambda *a: None)
    monkeypatch.setattr(ae, "build", lambda st, svcs, owners: [])
    monkeypatch.setattr(ae, "drop_unused", lambda ids: None)
    monkeypatch.setattr(ae, "seed_recsys", fake_seed)
    return seen


def _up_args(**kw):
    base = {"env": "feat", "src": "/r", "services": None, "slices": None, "with_": None, "with_optional": False,
            "branch": None, "overlay": None, "no_build": False, "seed_wait": 0}
    return types.SimpleNamespace(**{**base, **kw})


def test_up_keeps_recorded_overlays_and_adds_new_ones(monkeypatch, tmp_path):
    extra = "platform-e2e/compose/modelserve.override.yaml"
    prev = {"env": "feat", "slot": 3, "owners": [], "services": [], "overlays": [*ae.FEATURE_OVERLAYS, extra]}
    seen = _up_env(monkeypatch, tmp_path, prev)
    ae.cmd_up(_up_args(overlay=["platform-e2e/compose/recs.override.yaml"]))
    st = json.loads((tmp_path / "feat" / "state.json").read_text())
    assert st["overlays"] == [*ae.FEATURE_OVERLAYS, extra, "platform-e2e/compose/recs.override.yaml"]
    assert seen["config"] == [st["overlays"]]
    assert st["owners"] == ["team-sharing"]


def test_up_seeds_recsys_once_when_recsys_is_in_the_env(monkeypatch, tmp_path):
    services = {**SERVICES, "platform-recsys": {"image": "agora-platform-recsys:local", "build": {"context": "/r/platform-recsys"},
                                                 "depends_on": {"redis": {}}},
                "redis": {"image": "redis:7"}}
    seen = _up_env(monkeypatch, tmp_path, None, services)
    ae.cmd_up(_up_args(services=["platform-recsys"]))
    assert seen["seeded"] == 1
    assert json.loads((tmp_path / "feat" / "state.json").read_text())["recsys_seeded"] == "run1"
    ae.cmd_up(_up_args(services=["platform-recsys"]))
    assert seen["seeded"] == 1


def test_up_without_recsys_does_not_seed(monkeypatch, tmp_path):
    seen = _up_env(monkeypatch, tmp_path, None)
    ae.cmd_up(_up_args())
    assert seen["seeded"] == 0


def test_recsys_seed_uses_published_listings_first_and_pads_with_stable_synthetic_ids():
    n = ae.RECSYS_SEED_CLUSTERS * ae.RECSYS_SEED_ITEMS
    real = [f"l{i}" for i in range(n + 5)]
    assert ae.recsys_seed_listings(real) == real[:n]
    few = ae.recsys_seed_listings(real[:3])
    assert len(few) == n and len(set(few)) == n and few[:3] == real[:3]
    synthetic = ae.recsys_seed_listings([])
    assert few[3:] == synthetic[:n - 3] and not set(synthetic) & set(real)


def test_recsys_seed_beacons_give_each_cluster_its_own_items():
    listings = ae.recsys_seed_listings([])
    beacons = ae.recsys_seed_beacons(listings, "r1")
    by_user = {}
    for b in beacons:
        assert b["type"] in {"view", "click", "add_to_cart"} and b["anonymousId"].startswith("envseed-r1-")
        by_user.setdefault(b["anonymousId"], set()).add(b["listingId"])
    assert len(by_user) == ae.RECSYS_SEED_USERS
    assert all(len(v) == ae.RECSYS_SEED_PER_USER for v in by_user.values())
    clusters = {}
    for anon, items in by_user.items():
        clusters.setdefault(int(anon.rsplit("-", 1)[1]) % ae.RECSYS_SEED_CLUSTERS, set()).update(items)
    sets = list(clusters.values())
    assert all(not (a & b) for i, a in enumerate(sets) for b in sets[i + 1:])
    assert len({frozenset(v) for v in by_user.values()}) > ae.RECSYS_SEED_CLUSTERS


def test_recsys_seed_lists_far_more_items_than_top_n_so_lists_can_differ():
    assert ae.RECSYS_SEED_CLUSTERS * ae.RECSYS_SEED_ITEMS >= 4 * 50
    beacons = ae.recsys_seed_beacons(ae.recsys_seed_listings([]), "r1")
    assert len({b["listingId"] for b in beacons}) == ae.RECSYS_SEED_CLUSTERS * ae.RECSYS_SEED_ITEMS


def test_recsys_seed_rounds_give_each_visitor_one_new_item_per_round():
    rounds = ae.recsys_seed_rounds(ae.recsys_seed_listings([]), "r1")
    assert len(rounds) == ae.RECSYS_SEED_PER_USER
    first_round = {}
    for n, r in enumerate(rounds):
        per_user = {}
        for b in r:
            per_user.setdefault(b["anonymousId"], set()).add(b["listingId"])
        assert len(per_user) == ae.RECSYS_SEED_USERS and all(len(v) == 1 for v in per_user.values())
        for anon, items in per_user.items():
            (lid,) = items
            assert (anon, lid) not in first_round
            first_round[(anon, lid)] = n


def test_seed_recsys_pauses_between_rounds_so_server_timestamps_order_items(monkeypatch):
    posts, sleeps = [], []
    monkeypatch.setattr(ae, "wait_http", lambda url, timeout=120: True)
    monkeypatch.setattr(ae, "compose", lambda st, *a, **kw: types.SimpleNamespace(returncode=1, stdout=""))
    monkeypatch.setattr(ae, "post_track", lambda url, b: posts.append((len(sleeps), b)) or len(b))
    monkeypatch.setattr(ae.time, "sleep", lambda s: sleeps.append(s))
    run_id = ae.seed_recsys({"ports": {"8080": 38080}})
    assert run_id and len(sleeps) == ae.RECSYS_SEED_PER_USER - 1 and min(sleeps) >= 1
    assert all(len(b) <= 100 for _, b in posts)
    seen_at = {}
    for gap, batch in posts:
        for b in batch:
            seen_at.setdefault(b["anonymousId"], {}).setdefault(b["listingId"], gap)
    assert all(sorted(v.values()) == list(range(ae.RECSYS_SEED_PER_USER)) for v in seen_at.values())


def test_up_waits_for_the_export_after_seeding(monkeypatch, tmp_path):
    services = {**SERVICES, "platform-recsys": {"image": "agora-platform-recsys:local", "build": {"context": "/r/platform-recsys"}}}
    _up_env(monkeypatch, tmp_path, None, services)
    waited = []
    monkeypatch.setattr(ae, "wait_recsys_export", lambda st, svcs, run_id, timeout: waited.append((run_id, timeout)) or True)
    ae.cmd_up(_up_args(services=["platform-recsys"], seed_wait=420))
    assert waited == [("run1", 420)]


def test_wait_recsys_export_polls_until_every_seeded_visitor_is_exported(monkeypatch):
    counts = iter([None, 3, ae.RECSYS_SEED_USERS])
    monkeypatch.setattr(ae, "seeded_in_export", lambda st, svcs, run_id: next(counts))
    monkeypatch.setattr(ae.time, "sleep", lambda s: None)
    assert ae.wait_recsys_export({"env": "feat", "project": "feat"}, {}, "r1", timeout=60, poll=0) is True


def test_wait_recsys_export_gives_up_after_the_timeout(monkeypatch):
    monkeypatch.setattr(ae, "seeded_in_export", lambda st, svcs, run_id: 0)
    assert ae.wait_recsys_export({"env": "feat", "project": "feat"}, {}, "r1", timeout=0, poll=0) is False


EXPORT_SERVICES = {
    **SERVICES,
    "minio": {"image": "minio/minio", "ports": [{"published": "9000", "target": 9000}]},
    "flipt": {"image": "flipt/flipt"},
    "team-verification": {"image": "team-verification:local", "container_name": "team-verification-svc"},
    "team-frontend-seller": {"image": "team-frontend-seller:local", "container_name": "team-frontend-seller-svc"},
}


def test_feature_env_exports_env_minio_redis_and_containers(monkeypatch, tmp_path):
    monkeypatch.setattr(ae, "config", lambda src, overlays: EXPORT_SERVICES)
    monkeypatch.setattr(ae, "state_dir", lambda e: tmp_path / e)
    ports = {"9000": 39000, "6379": 36379, "50059": 50159, "9200": 39200, "18100": 48100, "18110": 48110}
    e = ae.e2e_exports({"env": "q7", "project": "q7", "src": str(tmp_path), "ports": ports, "owners": []})
    assert e["KYC_S3_ENDPOINT"] == "http://localhost:39000"
    assert e["AUE_REDIS_PORT"] == "36379"
    assert e["GA4_RELAY_GRPC_ADDR"] == "localhost:50159"
    assert e["SEARCH_BOOT_OPENSEARCH_URL"] == "http://host.docker.internal:39200"
    assert e["MODELSERVE_URL"] == "http://localhost:48100" and e["TEI_FAKE_URL"] == "http://localhost:48110"
    for k in ("KYC_POSTGRES_CONTAINER", "BCO_POSTGRES_CONTAINER", "BDL_POSTGRES_CONTAINER"):
        assert e[k] == "q7-postgres-1"
    assert e["KYC_MINIO_CONTAINER"] == "q7-minio-1"
    assert e["FLIPT_CONTAINER"] == "q7-flipt-1"
    assert e["VERIFICATION_CONTAINER"] == "q7-team-verification-svc"
    assert e["SELLER_CONTAINER"] == "q7-team-frontend-seller-svc"
    assert e["ORDER_CONTAINER"] == "q7-team-order-svc"


def test_main_env_exports_mainx_containers_and_default_ports(monkeypatch, tmp_path):
    monkeypatch.setattr(ae, "config", lambda src, overlays: EXPORT_SERVICES)
    monkeypatch.setattr(ae, "state_dir", lambda e: tmp_path / e)
    e = ae.e2e_exports({"env": ae.MAIN, "project": ae.MAIN_PROJECT, "src": str(tmp_path), "ports": {}, "owners": []})
    assert e["KYC_S3_ENDPOINT"] == "http://localhost:9000" and e["AUE_REDIS_PORT"] == "6379"
    assert e["KYC_MINIO_CONTAINER"] == "mainx-minio-1"
    assert e["VERIFICATION_CONTAINER"] == "team-verification-svc"


def test_closure_starts_the_service_publishing_a_localhost_port_named_by_env_or_code_defaults():
    services = {
        **SERVICES,
        "minio": {"image": "minio/minio", "ports": [{"published": "9000", "target": 9000}]},
        "minio-init": {"image": "minio/mc", "depends_on": {"minio": {}}},
        "team-domain": {"image": "team-domain:local", "container_name": "team-domain-svc", "environment": {"ENV": "local"}},
        "team-frontend": {"image": "team-frontend:local", "environment": {"NEXT_PUBLIC_GATEWAY_URL": "http://localhost:8080"}},
    }
    assert {"minio", "minio-init"} <= ae.closure(services, {"team-domain"})
    assert "team-gateway" in ae.closure(services, {"team-frontend"})
    assert "minio" not in ae.closure(services, {"team-gateway"})


def test_override_points_team_domain_code_default_storage_at_env_minio():
    services = {
        "minio": {"image": "minio/minio", "ports": [{"published": "9000", "target": 9000}]},
        "team-domain": {"image": "team-domain:local", "container_name": "team-domain-svc", "environment": {"ENV": "local"}},
    }
    ov = ae.make_override("q7", services, [], ae.port_map(services, 0))["services"]
    assert ov["team-domain"]["environment"] == {
        "STORAGE_ENDPOINT": "localhost:39000", "STORAGE_PUBLIC_BASE_URL": "http://localhost:39000/listing-images"}
    declared = {**services, "team-domain": {**services["team-domain"], "environment": {"STORAGE_ENDPOINT": "minio:9000"}}}
    ov2 = ae.make_override("q7", declared, [], ae.port_map(declared, 0))["services"]
    assert ov2["team-domain"]["environment"] == {"STORAGE_PUBLIC_BASE_URL": "http://localhost:39000/listing-images"}


def test_override_points_rpk_at_the_internal_listener():
    ov = ae.make_override("q7", SERVICES, [], ae.port_map(SERVICES, 0))["services"]
    assert ov["redpanda"]["environment"]["RPK_BROKERS"] == "127.0.0.1:9092"


def test_env_images_include_names_with_digits(monkeypatch):
    listing = "platform-e2e:q7\nteam-identity:q7\nteam-identity:local\nredis:q7\nplatform-recsys-nearline:q7\n"
    monkeypatch.setattr(ae, "out", lambda cmd, cwd=None: listing)
    assert ae.env_images("q7") == ["platform-e2e:q7", "platform-recsys-nearline:q7", "team-identity:q7"]


def test_with_is_repeatable_and_accepts_commas():
    a = ae.parser().parse_args(["plan", "--with", "a,b", "--with", "c"])
    assert a.with_ == ["a", "b", "c"]
    assert ae.parser().parse_args(["up", "x", "--with", "redis"]).with_ == ["redis"]
    assert ae.parser().parse_args(["plan"]).with_ is None


def test_resolve_builders_reads_build_dirs_from_the_src_tree():
    services = {"forecast-job": {"image": "platform-forecast:local", "build": {"context": "/w/platform-forecast"}}}
    assert ae.resolve_builders(services, ["platform-forecast"], "/w") == ["forecast-job"]
    with pytest.raises(SystemExit):
        ae.resolve_builders(services, ["platform-forecast"], "/elsewhere")


def test_plan_resolves_slice_owners_against_src(monkeypatch, tmp_path):
    services = {**SERVICES, "forecast-job": {"image": "platform-forecast:local", "build": {"context": "/w/platform-forecast"}}}
    monkeypatch.setattr(ae, "config", lambda src, overlays: services)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: [])
    _, _, owners, _ = ae.cmd_plan(_plan_args(src="/w", services=["platform-forecast"]))
    assert owners == ["forecast-job"]


def test_e2e_paths_accept_repo_relative_test_paths():
    got = ae.e2e_paths(["platform-e2e/tests/e2e/x.py::t", "tests/e2e/y.py", "-k", "z"])
    assert got == ["tests/e2e/x.py::t", "tests/e2e/y.py", "-k", "z"]


def _slices(tmp_path, slices, docs=None):
    p = tmp_path / "slices.yaml"
    p.write_text(yaml.dump({"slices": slices}))
    for cid, text in (docs or {}).items():
        d = tmp_path / "openspec" / "changes" / cid / "specs" / "x"
        d.mkdir(parents=True)
        (d / "spec.md").write_text(text)
    return p


def test_slice_extras_add_recsys_for_a_recsys_owned_slice(tmp_path):
    p = _slices(tmp_path, [{"id": "a", "owner": "platform-recsys"}])
    assert ae.slice_extras(p, tmp_path) == {"platform-recsys", "mlflow"}


def test_slice_extras_add_recsys_when_a_slice_e2e_touches_rgp(tmp_path):
    doc = "- **VERIFIED BY**: `platform-e2e/tests/e2e/step_definitions/test_rgp_generation_publish.py`\n"
    p = _slices(tmp_path, [{"id": "b", "owner": "team-ai"}], {"b": doc})
    assert ae.slice_extras(p, tmp_path) == {"platform-recsys", "mlflow"}


def test_slice_extras_add_recsys_for_a_recsys_slice_id(tmp_path):
    p = _slices(tmp_path, [{"id": "recsys-cart-cross-sell", "owner": "team-ai"}])
    assert ae.slice_extras(p, tmp_path) == {"platform-recsys", "mlflow"}


def test_slice_extras_add_nothing_for_unrelated_slices(tmp_path):
    doc = "- **VERIFIED BY**: `platform-e2e/tests/e2e/features/order/checkout.feature`\n"
    p = _slices(tmp_path, [{"id": "c", "owner": "team-order"}], {"c": doc})
    assert ae.slice_extras(p, tmp_path) == set()


def test_plan_runs_recsys_services_for_recsys_e2e_slices_and_keeps_with(monkeypatch, tmp_path):
    services = {
        **SERVICES,
        "minio": {"image": "minio/minio"},
        "minio-init": {"image": "minio/mc", "depends_on": {"minio": {}}},
        "mlflow": {"image": "mlflow", "depends_on": {"minio": {}}},
        "platform-recsys": {"image": "platform-recsys:local", "build": {"context": f"{tmp_path}/platform-recsys"},
                            "environment": {"MLFLOW_TRACKING_URI": "http://mlflow:5050"}},
    }
    monkeypatch.setattr(ae, "config", lambda src, overlays: services)
    monkeypatch.setattr(ae, "branch_changed_files", lambda src: [])
    doc = "`platform-e2e/tests/e2e/step_definitions/test_rgp_generation_publish.py`\n"
    p = _slices(tmp_path, [{"id": "b", "owner": "team-sharing"}], {"b": doc})
    (tmp_path / "team-sharing").mkdir()
    _, _, owners, run_set = ae.cmd_plan(_plan_args(src=str(tmp_path), slices=str(p), with_=["team-order"]))
    assert "platform-recsys" not in owners
    assert {"platform-recsys", "mlflow", "minio", "minio-init", "team-order"} <= set(run_set)


def test_compose_up_retries_once_after_a_failed_first_up(monkeypatch):
    codes = iter([1, 0])
    calls = []

    def fake_compose(st, *a, check=True, **kw):
        calls.append((a, check))
        return types.SimpleNamespace(returncode=next(codes), stdout="")

    monkeypatch.setattr(ae, "compose", fake_compose)
    monkeypatch.setattr(ae.time, "sleep", lambda s: None)
    ae.compose_up({"env": "feat"}, ["postgres", "team-sharing-migrate"])
    assert calls == [(("up", "-d", "--no-build", "postgres", "team-sharing-migrate"), False),
                     (("up", "-d", "--no-build", "postgres", "team-sharing-migrate"), True)]


def test_compose_up_does_not_retry_a_successful_up(monkeypatch):
    calls = []
    monkeypatch.setattr(ae, "compose", lambda st, *a, **kw: calls.append(a) or types.SimpleNamespace(returncode=0, stdout=""))
    ae.compose_up({"env": "feat"}, ["postgres"])
    assert len(calls) == 1
