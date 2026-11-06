import json
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
APPSET = ROOT / "argocd/preview/appset-e2e.yaml"
PREVIEW_ROOT = ROOT.parent / "deploy/argocd/preview-root-app.yaml"
INFRA = sorted((ROOT / "platform/infra").glob("*.yaml")) + [ROOT / "platform/postgres/postgres.yaml"]


def yq_docs(path: Path) -> list:
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", str(path)], capture_output=True, text=True, check=True).stdout
    return [json.loads(line) for line in out.splitlines() if line.strip() and line.strip() != "null"]


class InfraIsNamespaceAgnostic(unittest.TestCase):
    def test_no_hardcoded_marketplace_namespace_or_fqdn(self):
        for f in INFRA:
            with self.subTest(file=f.name):
                text = f.read_text()
                self.assertNotRegex(text, r"(?m)^\s*namespace:\s*marketplace\s*$")
                self.assertNotIn("namespace: marketplace }", text)
                self.assertNotIn(".marketplace.svc", text)

    def test_redpanda_advertises_its_own_namespace(self):
        text = (ROOT / "platform/infra/redpanda.yaml").read_text()
        self.assertIn("--advertise-kafka-addr=PLAINTEXT://redpanda.$(POD_NAMESPACE).svc:9092", text)
        self.assertIn("fieldPath: metadata.namespace", text)


class PreviewApplicationSets(unittest.TestCase):
    def setUp(self):
        self.docs = {d["metadata"]["name"]: d for d in yq_docs(APPSET)}

    def test_both_appsets_are_driven_by_initiative_files(self):
        self.assertEqual(set(self.docs), {"e2e-infra", "e2e-services"})
        infra_git = self.docs["e2e-infra"]["spec"]["generators"][0]["git"]
        svc_git = self.docs["e2e-services"]["spec"]["generators"][0]["matrix"]["generators"][0]["git"]
        for g in (infra_git, svc_git):
            self.assertEqual(g["files"], [{"path": "envs/e2e/initiatives/*/env.yaml"}])

    def test_each_initiative_gets_its_own_namespace_and_cascading_delete(self):
        for name, d in self.docs.items():
            with self.subTest(appset=name):
                t = d["spec"]["template"]
                self.assertEqual(t["spec"]["destination"]["namespace"], "e2e-{{.id}}")
                self.assertIn("resources-finalizer.argocd.argoproj.io", t["metadata"]["finalizers"])
                self.assertIn("CreateNamespace=true", t["spec"]["syncPolicy"]["syncOptions"])

    def test_services_use_prod_chart_and_baseline_then_e2e_layers(self):
        helm = self.docs["e2e-services"]["spec"]["template"]["spec"]["sources"][1]["helm"]
        self.assertEqual(helm["valueFiles"], [
            "$values/envs/e2e/values.yaml",
            "$values/envs/services/{{.name}}.yaml",
            "$values/envs/e2e/services.yaml",
            "$values/envs/e2e/initiatives/{{.id}}/{{.name}}.yaml",
        ])
        self.assertEqual(self.docs["e2e-services"]["spec"]["template"]["spec"]["sources"][1]["path"], "charts/service")

    def test_preview_root_only_watches_the_preview_dir(self):
        root = yq_docs(PREVIEW_ROOT)[0]
        self.assertEqual(root["spec"]["source"]["path"], "argocd/preview")
        self.assertFalse(list((ROOT / "argocd/apps").glob("*e2e*")))


class E2EOverlayRendersWithoutVault(unittest.TestCase):
    def test_sharing_renders_plain_env_without_secrets_or_network_policy(self):
        with tempfile.NamedTemporaryFile("w", suffix=".yaml") as f:
            f.write("images: {team-sharing: sha-abc}\nenv:\n  DATABASE_URL: postgres://sharing_svc:sharing_pass@postgres:5432/sharing_db\n")
            f.flush()
            out = subprocess.run(["helm", "template", "team-sharing", str(ROOT / "charts/service"), "--namespace", "e2e-x",
                                  "-f", str(ROOT / "envs/e2e/values.yaml"), "-f", str(ROOT / "envs/services/team-sharing.yaml"),
                                  "-f", str(ROOT / "envs/e2e/services.yaml"), "-f", f.name],
                                 capture_output=True, text=True, check=True).stdout
        kinds = set(re.findall(r"(?m)^kind: (\w+)", out))
        self.assertNotIn("ExternalSecret", kinds)
        self.assertNotIn("NetworkPolicy", kinds)
        self.assertIn("Deployment", kinds)
        self.assertIn(":sha-abc", out)
        self.assertIn("postgres://sharing_svc:sharing_pass@postgres:5432/sharing_db", out)


if __name__ == "__main__":
    unittest.main()
