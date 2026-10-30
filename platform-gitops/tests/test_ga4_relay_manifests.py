"""Static manifest checks for change add-ga4-purchase-relay (task gitops).

No manifest-test convention exists in platform-gitops (only scripts/check_identity_seed.py),
so this is the smallest check: stdlib unittest + `yq` (mikefarah) to parse multi-doc YAML.
Run: python3 -m unittest discover -s platform-gitops/tests -v   (from the repo root)
"""
import json
import re
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RELAY = ROOT / "platform/team-analytics/ga4-relay.yaml"
REDPANDA = ROOT / "platform/infra/redpanda.yaml"
KUSTOMIZATION = ROOT / "platform/team-analytics/kustomization.yaml"
GRPC_PORT = 50059
SECRET_KEYS = {"GA4_API_SECRET", "GA4_MEASUREMENT_ID"}


def load_docs(path: Path) -> list:
    assert path.exists(), f"{path} does not exist"
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", str(path)],
                         capture_output=True, text=True, check=True).stdout
    return [json.loads(l) for l in out.splitlines() if l.strip()]


def kind(docs, k):
    return [d for d in docs if d and d.get("kind") == k]


def containers(dep):
    return dep["spec"]["template"]["spec"]["containers"]


class Ga4RelayManifest(unittest.TestCase):
    def setUp(self):
        self.docs = load_docs(RELAY)

    def dep(self):
        deps = [d for d in kind(self.docs, "Deployment") if d["metadata"]["name"] == "ga4-relay"]
        self.assertEqual(len(deps), 1, "exactly one Deployment ga4-relay")
        return deps[0]

    def test_relay_probes_are_grpc(self):
        """Scenario: The relay is probed over gRPC."""
        for c in containers(self.dep()):
            env = {e["name"]: e.get("value") for e in c.get("env", [])}
            self.assertEqual(env.get("GRPC_PORT"), str(GRPC_PORT))
            self.assertNotIn("GA4_RELAY_PROBE_PORT", env)
            ports = {p["containerPort"]: p.get("name") for p in c.get("ports", [])}
            self.assertEqual(ports, {GRPC_PORT: "grpc"}, "one container port, named grpc")
            self.assertNotIn(8085, ports)
            for name in ("livenessProbe", "readinessProbe"):
                probe = c[name]
                self.assertNotIn("httpGet", probe)
                self.assertEqual(probe["grpc"]["port"], GRPC_PORT)
            self.assertEqual(c["livenessProbe"]["grpc"].get("service"), "liveness")
            self.assertNotIn("service", c["readinessProbe"]["grpc"])
        self.assertNotIn("8085", RELAY.read_text())
        self.assertNotIn("GA4_RELAY_PROBE_PORT", RELAY.read_text())

    def test_scenario_no_inbound_endpoint_beyond_probes_no_service_or_ingress(self):
        for d in self.docs:
            if d.get("kind") == "Ingress":
                self.fail("ga4-relay must not have an Ingress")
        for svc in kind(self.docs, "Service"):
            for p in svc["spec"].get("ports", []):
                self.fail("ga4-relay must not expose a Service port")

    def test_scenario_secret_unset_externalsecret_declares_only_the_two_keys(self):
        es = kind(self.docs, "ExternalSecret")
        self.assertEqual(len(es), 1, "one ExternalSecret")
        spec = es[0]["spec"]
        keys = {d["secretKey"] for d in spec.get("data", [])}
        self.assertEqual(keys, SECRET_KEYS)
        self.assertNotIn("dataFrom", spec, "no wildcard extraction; keys are explicit")

    def test_scenario_secret_unset_deployment_reads_keys_from_secret_never_inline(self):
        es_target = kind(self.docs, "ExternalSecret")[0]["spec"]["target"]["name"]
        found = {}
        for c in containers(self.dep()):
            for e in c.get("env", []):
                if e["name"] in SECRET_KEYS:
                    self.assertNotIn("value", e, f"{e['name']} must not be inline")
                    ref = e["valueFrom"]["secretKeyRef"]
                    self.assertEqual(ref["name"], es_target)
                    found[e["name"]] = ref["key"]
            for ef in c.get("envFrom", []):
                if ef.get("secretRef", {}).get("name") == es_target:
                    found = {k: k for k in SECRET_KEYS}
        self.assertEqual(set(found), SECRET_KEYS)

    def test_scenario_secret_unset_secretstore_vault_provider_and_serviceaccount_defined(self):
        docs = []
        for f in (ROOT / "platform").rglob("*.yaml"):
            if f.name != "kustomization.yaml":
                try:
                    docs += load_docs(f)
                except Exception:
                    pass
        ref = kind(self.docs, "ExternalSecret")[0]["spec"]["secretStoreRef"]
        stores = [d for d in kind(docs, "SecretStore") if d["metadata"]["name"] == ref["name"]]
        self.assertEqual(len(stores), 1, "SecretStore referenced by the ExternalSecret must be defined")
        vault = stores[0]["spec"]["provider"]["vault"]
        for k in ("server", "path"):
            self.assertTrue(vault.get(k), f"vault.{k} required")
        self.assertEqual(vault.get("version"), "v2")
        auth = vault["auth"]["kubernetes"]
        sa_name = auth["serviceAccountRef"]["name"]
        self.assertTrue(auth.get("role"))
        self.assertIn(sa_name, [d["metadata"]["name"] for d in kind(docs, "ServiceAccount")])
        pod = self.dep()["spec"]["template"]["spec"]
        self.assertEqual(pod.get("serviceAccountName"), sa_name)

    def test_scenario_secret_unset_no_plain_secret_kind_in_manifest(self):
        self.assertEqual(kind(self.docs, "Secret"), [], "secret values must not live in git")

    def test_scenario_no_inbound_endpoint_beyond_probes_kustomization_includes_manifest(self):
        text = KUSTOMIZATION.read_text()
        self.assertRegex(text, r"(?m)^\s*-\s*ga4-relay\.yaml\s*$")


class RedpandaPolicy(unittest.TestCase):
    def setUp(self):
        self.text = REDPANDA.read_text()

    def test_scenario_retries_exhausted_dlq_topic_created_by_init_job(self):
        self.assertRegex(self.text, r"(?m)^[^#\n]*analytics\.ga4-relay\.dlq")

    def test_scenario_retries_exhausted_dlq_owner_documented_team_analytics(self):
        lines = [l for l in self.text.splitlines()
                 if l.lstrip().startswith("#") and "analytics.ga4-relay.dlq" in l
                 and not re.search(r"(?i)PRODUCE|CONSUME", l)]
        self.assertTrue(any(re.search(r"owner:\s*team-analytics", l) for l in lines),
                        "topic declaration line must read `owner: team-analytics`")

    def test_scenario_retries_exhausted_dlq_topic_has_explicit_retention_ms(self):
        self.assertRegex(
            self.text,
            r"(?m)^[^#\n]*(?:analytics\.ga4-relay\.dlq[^\n]*retention\.ms=\d+|retention\.ms=\d+[^\n]*analytics\.ga4-relay\.dlq)")

    def test_scenario_retries_exhausted_dlq_acl_produce_consume_only_ga4_relay(self):
        comments = "\n".join(l.lstrip("# ").rstrip() for l in self.text.splitlines() if l.startswith("#"))
        m = re.search(r"analytics\.ga4-relay\.dlq\s+[—-]\s+PRODUCE/CONSUME:(.*?)(?=\n\s*•|\n[^\s]|\Z)",
                      comments, re.S)
        self.assertIsNotNone(m, "ACL entry `analytics.ga4-relay.dlq — PRODUCE/CONSUME: ...` missing")
        self.assertRegex(m.group(1), r"ONLY\s+(?:principal\s+)?(?:ga4-relay|team-analytics)")

    def test_scenario_forged_orderpaidevent_acl_order_events_produce_only_team_order(self):
        # ACLs are documented as comment blocks until broker auth is on (see file header).
        comments = [l.lstrip("# ").rstrip() for l in self.text.splitlines() if l.startswith("#")]
        block = "\n".join(comments)
        m = re.search(r"order\.events\s+[—-]\s+PRODUCE:(.*?)(?=\n\s*•|\n[^\s]|\Z)", block, re.S)
        self.assertIsNotNone(m, "ACL entry `order.events — PRODUCE: ...` missing")
        entry = m.group(1)
        self.assertRegex(entry, r"ONLY\s+(?:principal\s+)?team-order")
        self.assertRegex(entry, r"(?i)deny\s+all\s+other")
        self.assertNotRegex(entry, r"allow[^.]*ga4-relay")

    def test_scenario_dlq_retention_enforced_when_topic_preexists_alter_config(self):
        code = [l for l in self.text.splitlines() if not l.strip().startswith("#")]
        alter = [i for i, l in enumerate(code)
                 if re.search(r"rpk topic alter-config analytics\.ga4-relay\.dlq\b.*--set retention\.ms=604800000", l)]
        self.assertTrue(alter, "init script must `rpk topic alter-config analytics.ga4-relay.dlq --set retention.ms=604800000`")
        create = [i for i, l in enumerate(code) if re.search(r"rpk topic create analytics\.ga4-relay\.dlq\b", l)]
        self.assertTrue(create, "create line for the DLQ topic missing")
        self.assertGreater(min(alter), min(create), "alter-config must follow the create line")
        for i in create:
            if "|| true" in code[i]:
                self.assertTrue(alter, "create swallowed by `|| true` without alter-config after it")


REPO = ROOT.parent
COMPOSE = REPO / "docker-compose.services.yaml"
GATES = REPO / ".github/workflows/asdlc-gates.yml"


class Ga4RelayHardening(unittest.TestCase):
    def pod_dep(self):
        deps = [d for d in kind(load_docs(RELAY), "Deployment") if d["metadata"]["name"] == "ga4-relay"]
        self.assertEqual(len(deps), 1)
        return deps[0]

    def test_scenario_ops_pod_runs_nonroot_with_fsgroup(self):
        sc = self.pod_dep()["spec"]["template"]["spec"].get("securityContext") or {}
        self.assertIs(sc.get("runAsNonRoot"), True)
        self.assertIsInstance(sc.get("fsGroup"), int, "fsGroup needed so nonroot can write the PVC")
        self.assertGreater(sc["fsGroup"], 0)

    def test_scenario_ops_container_disallows_privilege_escalation(self):
        for c in containers(self.pod_dep()):
            self.assertIs((c.get("securityContext") or {}).get("allowPrivilegeEscalation"), False)


class AsdlcGatesRunsGitopsTests(unittest.TestCase):
    def test_scenario_ops_gates_workflow_runs_gitops_unittest_discover(self):
        self.assertTrue(GATES.exists())
        live = [l for l in GATES.read_text().splitlines() if not l.lstrip().startswith("#")]
        self.assertTrue(any(re.search(r"python3 -m unittest discover -s platform-gitops/tests", l) for l in live))


def _env_map(svc):
    env = svc.get("environment") or {}
    if isinstance(env, list):
        out = {}
        for e in env:
            k, _, v = str(e).partition("=")
            out[k] = v
        return out
    return {k: ("" if v is None else str(v)) for k, v in env.items()}


class LocalCompose(unittest.TestCase):
    def setUp(self):
        self.doc = load_docs(COMPOSE)[0]

    def svc(self):
        self.assertIn("ga4-relay", self.doc["services"], "compose service ga4-relay missing")
        return self.doc["services"]["ga4-relay"]

    def test_scenario_ops_compose_relay_uses_analytics_image_with_relay_entrypoint(self):
        s = self.svc()
        self.assertIn("analytics", s.get("image", "") + str(s.get("build", "")))
        ep = s.get("entrypoint")
        ep = ep if isinstance(ep, list) else str(ep).split()
        self.assertEqual(ep[:1], ["/ga4-relay"])

    def test_scenario_ops_compose_relay_env_and_own_volume(self):
        s = self.svc()
        env = _env_map(s)
        for k in ("GA4_RELAY_CONSUMER_GROUP", "GA4_RELAY_DLQ_TOPIC", "DUCKDB_PATH"):
            self.assertTrue(env.get(k), f"{k} must be set")
        self.assertEqual(env["GA4_RELAY_DLQ_TOPIC"], "analytics.ga4-relay.dlq")
        self.assertNotIn("GA4_RELAY_PROBE_PORT", env)
        mount_dir = env["DUCKDB_PATH"].rsplit("/", 1)[0]
        mine = [v.split(":")[0] for v in s.get("volumes", []) if isinstance(v, str) and v.split(":")[1:2] == [mount_dir]]
        self.assertEqual(len(mine), 1, f"a volume must be mounted at {mount_dir}")
        for name, other in self.doc["services"].items():
            if name != "ga4-relay":
                used = [v.split(":")[0] for v in other.get("volumes", []) if isinstance(v, str)]
                self.assertNotIn(mine[0], used, f"volume shared with {name}")
        self.assertIn(mine[0], self.doc.get("volumes") or {})

    def test_scenario_ops_compose_relay_runs_disabled_without_ga4_secrets(self):
        env = _env_map(self.svc())
        for k in SECRET_KEYS:
            self.assertFalse(env.get(k), f"{k} must have no value in compose")

    def test_scenario_ops_compose_redpanda_init_creates_relay_dlq_topic(self):
        init = self.doc["services"]["redpanda-init"]
        cmd = init["command"]
        cmd = " ".join(cmd) if isinstance(cmd, list) else str(cmd)
        self.assertRegex(cmd, r"rpk topic create[^\n]*analytics\.ga4-relay\.dlq")


class VaultConfigSeedsRelay(unittest.TestCase):
    def test_scenario_ops_vault_config_services_includes_ga4_relay(self):
        cms = [d for d in kind(load_docs(ROOT / "platform/vault-config/vault-config.yaml"), "ConfigMap")
               if d["metadata"]["name"] == "vault-config-script"]
        self.assertEqual(len(cms), 1)
        self.assertIn("ga4-relay", cms[0]["data"]["SERVICES"].split())


if __name__ == "__main__":
    unittest.main()
