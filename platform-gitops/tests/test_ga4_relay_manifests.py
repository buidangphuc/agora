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
PROBE_PORT = 8085
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

    def test_scenario_no_inbound_endpoint_beyond_probes_deployment_ports_and_probes(self):
        for c in containers(self.dep()):
            ports = {p["containerPort"] for p in c.get("ports", [])}
            self.assertLessEqual(ports, {PROBE_PORT}, "only the probe port may be declared")
            for name in ("livenessProbe", "readinessProbe"):
                p = c[name]["httpGet"]
                self.assertEqual(p["port"], PROBE_PORT)
            self.assertEqual(c["livenessProbe"]["httpGet"]["path"], "/healthz")
            self.assertEqual(c["readinessProbe"]["httpGet"]["path"], "/readyz")

    def test_scenario_no_inbound_endpoint_beyond_probes_no_service_or_ingress(self):
        for d in self.docs:
            if d.get("kind") == "Ingress":
                self.fail("ga4-relay must not have an Ingress")
        for svc in kind(self.docs, "Service"):
            for p in svc["spec"].get("ports", []):
                self.assertEqual(p.get("port", p.get("targetPort")), PROBE_PORT,
                                 "a Service may expose nothing but the probe port")
                self.assertEqual(p.get("targetPort", PROBE_PORT), PROBE_PORT)

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


if __name__ == "__main__":
    unittest.main()
