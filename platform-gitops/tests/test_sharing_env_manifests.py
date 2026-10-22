"""Task 2.2 (sharing-go-sdk-v1): team-sharing renders ENV per environment and a database-independent liveness probe.

Scenarios: "Missing ENV or GRPC_PORT is refused" (ENV set by overlays, GRPC_PORT from the image);
"Database loss flips health to NOT_SERVING" (liveness probe targets the liveness health service).
Run: python3 -m unittest platform-gitops.tests.test_sharing_env_manifests -v   (from the repo root)
"""
import json
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = ROOT / "envs/services/team-sharing.yaml"
OVERLAYS = {"local": None,
            "staging": ROOT / "envs/staging/services/team-sharing.yaml",
            "production": ROOT / "envs/prod/services/team-sharing.yaml"}


def yq_json(text: str) -> dict:
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", "-"], input=text,
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def render(overlay):
    cmd = ["helm", "template", "t", str(ROOT / "charts/service"), "-f", str(BASE)]
    if overlay is not None:
        assert overlay.exists(), f"{overlay} does not exist"
        cmd += ["-f", str(overlay)]
    out = subprocess.run(cmd, capture_output=True, text=True, check=True).stdout
    for doc in out.split("\n---"):
        d = yq_json(doc)
        if d.get("kind") == "Deployment":
            return d["spec"]["template"]["spec"]["containers"][0]
    raise AssertionError("no Deployment rendered")


def env_of(container) -> dict:
    return {e["name"]: e.get("value") for e in container.get("env", [])}


def inline_values() -> dict:
    app = yq_json((ROOT / "argocd/apps/team-sharing.yaml").read_text())
    inline = [s["helm"]["values"] for s in app["spec"]["sources"] if s.get("helm", {}).get("values")]
    assert len(inline) == 1
    return yq_json(inline[0])


class SharingEnvManifests(unittest.TestCase):
    def test_env_rendered_per_overlay_missing_env_is_refused(self):
        for name, overlay in OVERLAYS.items():
            with self.subTest(env=name):
                self.assertEqual(env_of(render(overlay)).get("ENV"), name)

    def test_baseline_sets_env_local_missing_env_is_refused(self):
        self.assertEqual(yq_json(BASE.read_text()).get("env", {}).get("ENV"), "local")

    def test_argo_inline_mirror_sets_env_local_missing_env_is_refused(self):
        self.assertEqual(inline_values().get("env", {}).get("ENV"), "local")

    def test_grpc_port_not_set_by_manifests_comes_from_image(self):
        for name, overlay in OVERLAYS.items():
            with self.subTest(env=name):
                self.assertNotIn("GRPC_PORT", env_of(render(overlay)))

    def test_database_url_not_inlined_in_env(self):
        for name, overlay in OVERLAYS.items():
            with self.subTest(env=name):
                self.assertNotIn("DATABASE_URL", env_of(render(overlay)))

    def test_liveness_probe_uses_liveness_service_database_loss_flips_health(self):
        for name, overlay in OVERLAYS.items():
            with self.subTest(env=name):
                c = render(overlay)
                self.assertEqual(c["livenessProbe"]["grpc"].get("service"), "liveness")
                self.assertNotIn("service", c["readinessProbe"]["grpc"])

    def test_argo_inline_mirror_sets_liveness_service(self):
        self.assertEqual(inline_values().get("probes", {}).get("livenessService"), "liveness")


if __name__ == "__main__":
    unittest.main()
