"""Task 1.1.2 (deploy-go-sdk-v1): gitops manifests for the Go SDK v1 migration.

Scenarios (specs/deploy-runtime/spec.md): "Every Go service renders a liveness probe by name",
"Every Go service renders its strict environment", "A service without an overlay is flagged",
"The argocd inline block mirrors the baseline", "Every database service renders a migrate Job and no
other service does", "The secret exists before the migrate Job runs", "The Job takes no migrations from a
mounted volume", "The migrate Job precedes the new pods", "No baseline or inline block carries a removed
key", "The promotion kill-switch is live in every cluster", "A required upstream address is set in every
rendered environment", "The domain service gets its container port", "The analytics consumer overlay
renders its strict environment", "The raw manifest bases boot locally with ENV and GRPC_PORT", "The
Deployment never migrates at boot", "The search indexer overlay renders its strict environment",
"Deployed manifests never enable reflection or mock data", "The analytics consumer reads its secret
through an ExternalSecret", "No literal secret is committed for a deployed environment", "The Vault seed
covers the new key and the analytics service", "The dead-letter topics have a seven-day retention in
staging and prod", "A failing migrate Job is bounded and fails the sync", "The migrate Job renders
without an ExternalSecret", "A failed migrate Job in the first rollout is recoverable within the bound",
"A database service renders one PreSync migrate Job", "The strategy value renders a Recreate Deployment",
"Recreate is never committed to a deployed overlay".
Run: python -m pytest platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py -q   (from the repo root)
"""
import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

GO_SERVICES = ["team-chat", "team-verification", "team-search", "team-audit", "team-notification",
               "team-promotion", "team-payment", "team-engagement", "team-domain", "team-gateway",
               "team-order", "team-identity", "team-sharing", "team-referral"]
DB_SERVICES = [s for s in GO_SERVICES if s != "team-gateway"]
ENVS = {"staging": "staging", "prod": "production"}

REMOVED_KEYS = {"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "OUTBOX_MAX_ATTEMPTS",
                "OUTBOX_CLAIM_LOCK_SECONDS", "FEATURE_FLAGS_ENABLED", "FEATURE_FLAGS_EVAL_TIMEOUT_MS",
                "IDENTITY_CONSUMER_ENABLED", "PAYMENT_SETTLEMENT_CONSUMER_ENABLED",
                "ENGAGEMENT_OUTBOX_RELAY_INTERVAL", "MOCK_PAYMENTS", "EDGE_REFLECTION_ENABLED",
                "GA4_EXPORT_SOURCE", "GA4_RELAY_PROBE_PORT", "UPSTREAM_CALL_TIMEOUT_SECONDS"}
UPSTREAMS = {
    "UPSTREAM_AUDIT_ADDR": ["team-verification", "team-promotion", "team-payment", "team-engagement",
                            "team-domain", "team-order", "team-identity"],
    "UPSTREAM_ORDER_ADDR": ["team-payment", "team-engagement"],
    "UPSTREAM_DOMAIN_ADDR": ["team-order", "team-promotion", "team-notification"],
    "UPSTREAM_PROMOTION_ADDR": ["team-order"],
    "UPSTREAM_IDENTITY_ADDR": ["team-order", "team-notification"],
    "UPSTREAM_AI_ADDR": ["team-domain"],
}
HOOK = "argocd.argoproj.io/hook"
DELETE_POLICY = "argocd.argoproj.io/hook-delete-policy"
WAVE = "argocd.argoproj.io/sync-wave"


def yq_json(text: str):
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", "-"], input=text,
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def yq_docs(text: str):
    docs = []
    for doc in re.split(r"(?m)^---\s*$", text):
        if doc.strip():
            d = yq_json(doc)
            if d:
                docs.append(d)
    return docs


def layers(svc, env):
    files = [ROOT / "envs" / env / "values.yaml", ROOT / "envs/services" / f"{svc}.yaml",
             ROOT / "envs" / env / "services.yaml", ROOT / "envs" / env / "services" / f"{svc}.yaml"]
    return [f for f in files if f.exists()]


def helm(svc, env="local", sets=()):
    cmd = ["helm", "template", svc, str(ROOT / "charts/service"), "--namespace", "marketplace"]
    for f in layers(svc, env):
        cmd += ["-f", str(f)]
    for s in sets:
        cmd += ["--set", s]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"helm template {svc} {env} failed: {res.stderr[-400:]}"
    return yq_docs(res.stdout)


def kustomize(path: Path):
    exe = ["kustomize", "build"] if shutil.which("kustomize") else ["kubectl", "kustomize"]
    res = subprocess.run(exe + [str(path)], capture_output=True, text=True)
    assert res.returncode == 0, f"kustomize build {path.relative_to(ROOT)} failed: {res.stderr[-400:]}"
    return yq_docs(res.stdout)


def of_kind(docs, *kinds):
    return [d for d in docs if d.get("kind") in kinds]


def workload(docs):
    found = of_kind(docs, "Deployment", "Rollout")
    assert len(found) == 1, f"expected one workload, got {len(found)}"
    return found[0]


def container(doc, name=None):
    cs = doc["spec"]["template"]["spec"]["containers"]
    if name is None:
        return cs[0]
    match = [c for c in cs if c["name"] == name]
    assert match, f"no container {name} in {doc['metadata']['name']}"
    return match[0]


def env_of(c):
    return {e["name"]: e.get("value") for e in c.get("env", [])}


def env_entry(c, name):
    for e in c.get("env", []):
        if e["name"] == name:
            return e
    return None


def jobs(docs):
    return of_kind(docs, "Job")


def argo_inline(svc):
    app = yq_json((ROOT / "argocd/apps" / f"{svc}.yaml").read_text())
    inline = [s["helm"]["values"] for s in app["spec"]["sources"] if s.get("helm", {}).get("values")]
    assert len(inline) == 1, f"{svc}: expected one inline values block"
    return yq_json(inline[0])


def baseline(svc):
    return yq_json((ROOT / "envs/services" / f"{svc}.yaml").read_text())


def names_in(obj):
    out = set()
    if isinstance(obj, dict):
        for k, v in obj.items():
            out.add(str(k))
            if k == "name" and isinstance(v, str):
                out.add(v)
            out |= names_in(v)
    elif isinstance(obj, list):
        for v in obj:
            out |= names_in(v)
    return out


def literal_values(obj, keys):
    found = []
    if isinstance(obj, dict):
        if obj.get("name") in keys and isinstance(obj.get("value"), str) and obj["value"] != "":
            found.append((obj["name"], obj["value"]))
        for k, v in obj.items():
            if k in keys and isinstance(v, str) and v != "":
                found.append((k, v))
            found += literal_values(v, keys)
    elif isinstance(obj, list):
        for v in obj:
            found += literal_values(v, keys)
    return found


def yaml_files(path: Path):
    return sorted(p for p in path.rglob("*.yaml") if p.is_file())


def parsed_names(path: Path):
    text = path.read_text()
    docs = yq_docs(text)
    out = set()
    for d in docs:
        out |= names_in(d)
        if d.get("kind") == "Application":
            for s in d.get("spec", {}).get("sources", []):
                inline = s.get("helm", {}).get("values")
                if inline:
                    out |= names_in(yq_json(inline))
    return out


RAW = {"team-analytics": ROOT / "platform/team-analytics", "search-indexer": ROOT / "platform/search-indexer"}


class Probes(unittest.TestCase):
    def test_every_go_service_probes_liveness_by_name(self):
        for svc in GO_SERVICES:
            with self.subTest(svc=svc):
                c = container(workload(helm(svc)))
                self.assertEqual(c["livenessProbe"]["grpc"].get("service"), "liveness")
                self.assertNotIn("service", c["readinessProbe"]["grpc"])
                self.assertNotIn("service", c["startupProbe"]["grpc"])
                self.assertNotIn("httpGet", c["livenessProbe"])


class StrictEnv(unittest.TestCase):
    def test_every_go_service_renders_strict_env(self):
        for env, want in ENVS.items():
            for svc in GO_SERVICES:
                with self.subTest(env=env, svc=svc):
                    self.assertEqual(env_of(container(workload(helm(svc, env)))).get("ENV"), want)

    def test_every_appset_go_service_has_both_overlays(self):
        appset = yq_json((ROOT / "argocd/apps/appset-services.yaml").read_text())
        listed = {e["name"] for g in appset["spec"]["generators"][0]["matrix"]["generators"]
                  for e in g.get("list", {}).get("elements", [])}
        self.assertEqual(sorted(set(GO_SERVICES) - listed), [], "Go services missing from the ApplicationSet list")
        missing = [f"envs/{d}/services/{svc}.yaml" for svc in GO_SERVICES for d in ("staging", "prod")
                   if not (ROOT / "envs" / d / "services" / f"{svc}.yaml").exists()]
        self.assertEqual(missing, [], "services without an overlay")
        for svc in GO_SERVICES:
            for d in ("staging", "prod"):
                env = yq_json((ROOT / "envs" / d / "services" / f"{svc}.yaml").read_text()).get("env", {})
                self.assertNotEqual(env.get("ENV"), "local", f"{svc} {d}")

    def test_baselines_set_env_local(self):
        for svc in GO_SERVICES:
            with self.subTest(svc=svc):
                self.assertEqual(baseline(svc).get("env", {}).get("ENV"), "local")
                self.assertEqual(argo_inline(f"{svc}").get("env", {}).get("ENV"), "local")


class InlineMirror(unittest.TestCase):
    ALLOWED = {"team-payment": {"MOCK_DATA": "payment-provider"},
               "team-engagement": {"LOG_KEY_FP_SECRET": "local-e2e-log-key-fp-secret"},
               "team-order": {"LOG_KEY_FP_SECRET": "local-dev-key-fp-secret"}}

    def test_inline_mirror_equals_baseline(self):
        for svc in GO_SERVICES:
            with self.subTest(svc=svc):
                base, inline = baseline(svc), argo_inline(svc)
                allowed = self.ALLOWED.get(svc, {})
                inline_env = dict(inline.get("env", {}))
                for k, v in allowed.items():
                    self.assertNotIn(k, base.get("env", {}), f"{svc}: {k} is inline-only")
                    if k in inline_env:
                        self.assertEqual(inline_env.pop(k), v)
                self.assertEqual(inline_env, base.get("env", {}))
                for key in ("probes", "grpcPort", "migrate"):
                    self.assertEqual(inline.get(key), base.get(key), key)
                self.assertEqual(inline.get("probes", {}).get("livenessService"), "liveness")


class MigrateJob(unittest.TestCase):
    def test_migrate_job_only_for_database_services(self):
        for svc in GO_SERVICES + ["team-frontend"]:
            with self.subTest(svc=svc):
                found = jobs(helm(svc))
                if svc in DB_SERVICES:
                    self.assertEqual([j["metadata"]["name"] for j in found], [f"{svc}-migrate"])
                    self.assertTrue(baseline(svc).get("migrate", {}).get("enabled") is True)
                    self.assertTrue(argo_inline(svc).get("migrate", {}).get("enabled") is True)
                else:
                    self.assertEqual(found, [])

    def test_database_service_renders_one_presync_migrate_job(self):
        docs = helm("team-order", "staging")
        found = jobs(docs)
        self.assertEqual([j["metadata"]["name"] for j in found], ["team-order-migrate"])
        job = found[0]
        ann = job["metadata"]["annotations"]
        self.assertEqual(ann[HOOK], "PreSync")
        self.assertEqual(ann[DELETE_POLICY], "BeforeHookCreation")
        c = container(job)
        self.assertEqual(c["args"], ["migrate", "up"])
        self.assertEqual(c["image"], container(workload(docs))["image"])
        self.assertEqual(env_of(c).get("ENV"), "staging")
        self.assertEqual([e["secretRef"]["name"] for e in c["envFrom"]], ["team-order-env"])

    def test_migrate_job_absent_when_disabled(self):
        self.assertEqual(len(jobs(helm("team-verification", sets=["migrate.enabled=true"]))), 1)
        self.assertEqual(jobs(helm("team-verification", sets=["migrate.enabled=false"])), [])

    def test_secret_resources_precede_migrate_job(self):
        docs = helm("team-order", sets=["migrate.enabled=true", "secrets.enabled=true"])
        for kind in ("ServiceAccount", "SecretStore", "ExternalSecret"):
            found = of_kind(docs, kind)
            self.assertEqual(len(found), 1, kind)
            ann = found[0]["metadata"].get("annotations", {})
            self.assertEqual(ann.get(HOOK), "PreSync", kind)
            self.assertEqual(ann.get(WAVE), "-1", kind)
        ann = jobs(docs)[0]["metadata"]["annotations"]
        self.assertEqual(ann.get(HOOK), "PreSync")
        self.assertNotIn(WAVE, ann)

    def test_migrate_job_failure_bounds(self):
        job = jobs(helm("team-order", sets=["migrate.enabled=true"]))[0]
        self.assertEqual(job["spec"]["backoffLimit"], 2)
        self.assertEqual(job["spec"]["activeDeadlineSeconds"], 600)
        self.assertEqual(job["spec"]["template"]["spec"]["restartPolicy"], "Never")
        self.assertEqual(job["metadata"]["annotations"][HOOK], "PreSync")

    def test_migrate_job_without_secrets(self):
        docs = helm("team-order", sets=["migrate.enabled=true", "secrets.enabled=false",
                                         "env.DATABASE_URL=postgres://x/y"])
        job = jobs(docs)
        self.assertEqual(len(job), 1)
        c = container(job[0])
        env = env_of(c)
        self.assertEqual(env.get("DATABASE_URL"), "postgres://x/y")
        self.assertEqual(env.get("ENV"), env_of(container(workload(docs))).get("ENV"))
        self.assertNotIn("envFrom", c)
        for kind in ("ServiceAccount", "SecretStore", "ExternalSecret"):
            for d in of_kind(docs, kind):
                self.assertNotIn(HOOK, d["metadata"].get("annotations", {}), kind)

    def test_migrate_job_has_no_volume(self):
        for svc in DB_SERVICES:
            with self.subTest(svc=svc):
                job = jobs(helm(svc))
                self.assertEqual(len(job), 1)
                pod = job[0]["spec"]["template"]["spec"]
                self.assertNotIn("volumes", pod)
                for c in pod["containers"]:
                    self.assertNotIn("volumeMounts", c)

    def test_migrate_job_is_presync_deployment_is_not(self):
        docs = helm("team-order")
        self.assertEqual(jobs(docs)[0]["metadata"]["annotations"][HOOK], "PreSync")
        dep = workload(docs)
        self.assertNotIn(HOOK, dep["metadata"].get("annotations", {}))
        self.assertNotIn(WAVE, dep["metadata"].get("annotations", {}))

    def test_deployment_never_runs_migrate(self):
        for svc in DB_SERVICES:
            with self.subTest(svc=svc):
                docs = helm(svc, sets=["migrate.enabled=true"])
                c = container(workload(docs))
                self.assertNotIn("migrate", c.get("args", []) + c.get("command", []))
                migrating = [j for j in jobs(docs) if "migrate" in container(j).get("args", [])]
                self.assertEqual(len(migrating), 1)
                self.assertEqual(container(migrating[0])["args"], ["migrate", "up"])


class GatewayAndPorts(unittest.TestCase):
    def test_scenario_the_gateway_renders_no_http_probe(self):
        c = container(workload(helm("team-gateway")))
        for probe in ("readinessProbe", "livenessProbe", "startupProbe"):
            with self.subTest(probe=probe):
                self.assertNotIn("httpGet", c[probe])
                self.assertEqual(c[probe]["grpc"]["port"], 8080)
        self.assertNotIn("healthPath", baseline("team-gateway"))
        self.assertNotIn("healthPath", argo_inline("team-gateway"))

    def test_scenario_the_domain_service_gets_its_container_port(self):
        c = container(workload(helm("team-domain")))
        self.assertIn(50051, [p["containerPort"] for p in c["ports"]])
        self.assertEqual(env_of(c).get("GRPC_PORT"), "50051")

    def test_scenario_design_d1_additions_present(self):
        self.assertEqual(baseline("team-search").get("grpcPort"), 50052)
        self.assertEqual(env_of(container(workload(helm("team-order")))).get("OUTBOX_CLAIM_LOCK"), "60s")
        self.assertEqual(env_of(container(workload(helm("team-engagement")))).get("OUTBOX_POLL_INTERVAL"), "500ms")


class Strategy(unittest.TestCase):
    def test_strategy_value_renders_recreate_deployment(self):
        dep = workload(helm("team-order", sets=["strategy.type=Recreate"]))
        self.assertEqual(dep["spec"]["strategy"]["type"], "Recreate")

    def test_no_committed_recreate_strategy(self):
        self.assertEqual(yq_json((ROOT / "charts/service/values.yaml").read_text()).get("strategy"), {})
        files = yaml_files(ROOT / "envs") + sorted((ROOT / "argocd/apps").glob("team-*.yaml"))
        for f in files:
            with self.subTest(file=str(f.relative_to(ROOT))):
                self.assertNotIn("strategy", parsed_names(f))


class RemovedKeysAndEnv(unittest.TestCase):
    def test_no_removed_key_in_gitops(self):
        files = sorted((ROOT / "envs/services").glob("*.yaml")) + sorted((ROOT / "argocd/apps").glob("team-*.yaml"))
        files += yaml_files(RAW["team-analytics"]) + yaml_files(RAW["search-indexer"])
        for f in files:
            with self.subTest(file=str(f.relative_to(ROOT))):
                present = parsed_names(f)
                self.assertEqual(sorted(present & REMOVED_KEYS), [])
                if f.name == "team-gateway.yaml":
                    self.assertNotIn("HTTP_PORT", present)

    def test_promotion_kill_switch_live_and_upstreams_set(self):
        for env in ("local", "staging", "prod"):
            with self.subTest(env=env):
                env_map = env_of(container(workload(helm("team-promotion", env))))
                self.assertNotIn("FEATURE_FLAGS_ENABLED", env_map)
                for key in ("FLIPT_ADDR", "KAFKA_BROKERS", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_AUDIT_ADDR"):
                    self.assertTrue(env_map.get(key), key)

    def test_required_upstream_addresses_set_in_every_env(self):
        for addr, services in UPSTREAMS.items():
            for svc in services:
                for env in ("local", "staging", "prod"):
                    with self.subTest(addr=addr, svc=svc, env=env):
                        self.assertTrue(env_of(container(workload(helm(svc, env)))).get(addr))
        for sub in ("", "overlays/staging", "overlays/prod"):
            with self.subTest(indexer=sub or "base"):
                c = container(workload(kustomize(RAW["search-indexer"] / sub)), "indexer")
                self.assertTrue((env_entry(c, "UPSTREAM_AI_ADDR") or {}).get("value"))

    def test_grpc_port_in_container_env(self):
        self.assertEqual(env_of(container(workload(helm("team-domain")))).get("GRPC_PORT"), "50051")
        self.assertEqual(env_of(container(workload(helm("team-gateway")))).get("GRPC_PORT"), "8080")
        chat = baseline("team-chat")
        self.assertEqual(env_of(container(workload(helm("team-chat")))).get("GRPC_PORT"), str(chat["containerPort"]))


class RawManifests(unittest.TestCase):
    def test_raw_manifest_bases_set_local_env_and_grpc_port(self):
        docs = kustomize(RAW["team-analytics"])
        for name in ("team-analytics", "ga4-relay"):
            dep = [d for d in of_kind(docs, "Deployment") if d["metadata"]["name"] == name]
            self.assertEqual(len(dep), 1, name)
            env = env_of(container(dep[0]))
            self.assertEqual(env.get("ENV"), "local", name)
            self.assertEqual(env.get("GRPC_PORT"), "50059", name)
        idx = container(workload(kustomize(RAW["search-indexer"])), "indexer")
        self.assertEqual(env_of(idx).get("ENV"), "local")

    def test_analytics_overlays_render_strict_env(self):
        for env, want in ENVS.items():
            with self.subTest(env=env):
                docs = kustomize(RAW["team-analytics"] / "overlays" / env)
                consumer = container(
                    [d for d in of_kind(docs, "Deployment") if d["metadata"]["name"] == "team-analytics"][0], "consumer")
                env_map = env_of(consumer)
                self.assertEqual(env_map.get("ENV"), want)
                self.assertEqual(env_map.get("GRPC_PORT"), "50059")
                for key in ("LOG_KEY_FP_SECRET", "GA4_API_SECRET", "GA4_MEASUREMENT_ID", "GA4_EXPORT_DATASET"):
                    entry = env_entry(consumer, key)
                    self.assertIsNotNone(entry, key)
                    self.assertIn("secretKeyRef", entry.get("valueFrom", {}), key)
                for key in ("MOCK_DATA", "GRPC_REFLECTION_ENABLED"):
                    self.assertNotIn(key, env_map)
                relay = container(
                    [d for d in of_kind(docs, "Deployment") if d["metadata"]["name"] == "ga4-relay"][0])
                self.assertEqual(env_of(relay).get("ENV"), want)
                self.assertIn("secretKeyRef", (env_entry(relay, "LOG_KEY_FP_SECRET") or {}).get("valueFrom", {}))

    def test_indexer_overlays_render_strict_env(self):
        for env, want in ENVS.items():
            with self.subTest(env=env):
                c = container(workload(kustomize(RAW["search-indexer"] / "overlays" / env)), "indexer")
                self.assertEqual(env_of(c).get("ENV"), want)
                self.assertNotIn("KAFKA_ENABLED", env_of(c))
                explicit = ((env_entry(c, "LOG_KEY_FP_SECRET") or {}).get("valueFrom", {})
                            .get("secretKeyRef", {}).get("name"))
                from_all = [e["secretRef"]["name"] for e in c.get("envFrom", []) if "secretRef" in e]
                self.assertTrue(explicit == "team-search-env" or "team-search-env" in from_all)

    def test_analytics_consumer_has_external_secret(self):
        docs = kustomize(RAW["team-analytics"])
        es = [d for d in of_kind(docs, "ExternalSecret") if d["metadata"]["name"] == "team-analytics"]
        self.assertEqual(len(es), 1)
        self.assertEqual(es[0]["spec"]["target"]["name"], "team-analytics-env")
        keys = [x["extract"]["key"] for x in es[0]["spec"].get("dataFrom", [])]
        keys += [x["remoteRef"]["key"] for x in es[0]["spec"].get("data", [])]
        self.assertIn("team-analytics/config", keys)
        consumer = container(
            [d for d in of_kind(docs, "Deployment") if d["metadata"]["name"] == "team-analytics"][0], "consumer")
        ref = env_entry(consumer, "LOG_KEY_FP_SECRET")["valueFrom"]["secretKeyRef"]
        self.assertEqual(ref["name"], "team-analytics-env")


def deployed_overlay_docs():
    docs = []
    for env in ENVS:
        for f in yaml_files(ROOT / "envs" / env):
            docs.append((str(f.relative_to(ROOT)), names_in(yq_json(f.read_text()))))
        for raw in RAW.values():
            for d in kustomize(raw / "overlays" / env):
                docs.append((f"{raw.name}/overlays/{env}", names_in(d)))
    return docs


class Guards(unittest.TestCase):
    def test_no_deployed_manifest_sets_reflection_or_mock_data(self):
        banned = {"GRPC_REFLECTION_ENABLED", "MOCK_DATA"}
        for label, present in deployed_overlay_docs():
            with self.subTest(file=label):
                self.assertEqual(sorted(present & banned), [])
        shared = [ROOT / "charts/service/values.yaml"] + sorted((ROOT / "envs/services").glob("*.yaml"))
        for f in shared:
            with self.subTest(file=str(f.relative_to(ROOT))):
                self.assertEqual(sorted(names_in(yq_json(f.read_text())) & banned), [])

    def test_no_literal_secret_in_deployed_overlays(self):
        keys = {"LOG_KEY_FP_SECRET", "GA4_API_SECRET", "DATABASE_URL"}
        for env in ENVS:
            for f in yaml_files(ROOT / "envs" / env):
                with self.subTest(file=str(f.relative_to(ROOT))):
                    self.assertEqual(literal_values(yq_json(f.read_text()), keys), [])
            for raw in RAW.values():
                with self.subTest(raw=raw.name, env=env):
                    for d in kustomize(raw / "overlays" / env):
                        self.assertEqual(literal_values(d, keys), [])

    def test_vault_seed_has_log_key_secret(self):
        docs = yq_docs((ROOT / "platform/vault-config/vault-config.yaml").read_text())
        data = {}
        for d in docs:
            data.update(d.get("data", {}) if isinstance(d.get("data"), dict) else {})
        services = data["SERVICES"].split()
        self.assertIn("team-analytics", services)
        script = "\n".join(v for v in data.values() if "vault kv" in v)
        guard = re.search(r"kv get[^\n]*LOG_KEY_FP_SECRET", script)
        write = re.search(r"LOG_KEY_FP_SECRET=", script)
        self.assertTrue(guard and write, "seed must read then write LOG_KEY_FP_SECRET")
        self.assertLess(guard.start(), write.start())

    def test_dlq_topics_have_seven_day_retention(self):
        text = (ROOT / "platform/infra/redpanda.yaml").read_text()
        value = "retention.ms=604800000"
        loops = [(m.group(2).replace("\\\n", " ").split(), m.group(3))
                 for m in re.finditer(r"for (\w+) in ((?:.|\n)*?);\s*do((?:.|\n)*?)\bdone", text)]
        for topic in ("analytics.events.dlq", "listing.events.analytics.dlq", "identity.events.dlq"):
            with self.subTest(topic=topic):
                created = altered = False
                for items, body in loops:
                    if topic in items and "topic create" in body and value in body and "alter-config" in body:
                        created = True
                        altered = altered or any(value in ln for ln in body.splitlines() if "alter-config" in ln)
                for ln in text.splitlines():
                    if topic in ln and "topic create" in ln and value in ln:
                        created = True
                    if topic in ln and "alter-config" in ln and value in ln:
                        altered = True
                self.assertTrue(created, f"{topic} not created with {value}")
                self.assertTrue(altered, f"{topic} not altered to {value}")


class Runbook(unittest.TestCase):
    def test_outbox_runbook_covers_autosync_and_failed_job(self):
        path = ROOT / "docs/runbooks/outbox-first-rollout.md"
        self.assertTrue(path.exists(), "runbook missing")
        text = path.read_text()
        steps = [re.search(p, text, re.I) for p in (
            r"--sync-policy none", r"kubectl scale[^\n]*--replicas=0",
            r"strategy:\s*\{\s*type:\s*Recreate\s*\}", r"re-?enable[^\n]*auto-?sync")]
        self.assertTrue(all(steps), f"missing steps: {[bool(s) for s in steps]}")
        pos = [s.start() for s in steps]
        self.assertEqual(pos, sorted(pos), "steps out of order")
        self.assertRegex(text, r"selfHeal")
        failure = re.search(r"(?im)^#+[^\n]*fail[^\n]*$", text)
        self.assertIsNotNone(failure, "no failure section")
        section = text[failure.start():]
        self.assertRegex(section, r"(?i)outage|service is down|stays down")
        self.assertRegex(section, r"30 ?min")
        self.assertRegex(section, r"(?i)re-?sync")
        self.assertRegex(section, r"(?i)scal\w+[^\n]*(old|previous)")
        self.assertRegex(section, r"(?i)select count\(\*\)")


if __name__ == "__main__":
    unittest.main()
