"""Manifest checks for change frontend-cookie-secure, task 1.6.

stdlib unittest + `yq` (mikefarah), like the other platform-gitops manifest tests.
Run: python3 -m unittest discover -s platform-gitops/tests -v   (from the repo root)
"""
import json
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
REPO = ROOT.parent
BASE = ROOT / "envs/services/team-frontend.yaml"
ENVS = ("staging", "prod")


def load(path: Path) -> dict:
    assert path.exists(), f"{path} does not exist"
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", str(path)],
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def overlay(env: str) -> dict:
    return load(ROOT / f"envs/{env}/services/team-frontend.yaml")


def merged_env(env: str) -> dict:
    """Helm-style map merge: overlay env keys layered over the base env."""
    return {**(load(BASE).get("env") or {}), **(overlay(env).get("env") or {})}


class FrontendCookieSecureManifest(unittest.TestCase):
    def test_scenario_cookie_secure_staging_and_prod_set_it(self):
        for env in ENVS:
            with self.subTest(env=env):
                # Must be the string "true" (a YAML bool would render as a different env value type).
                self.assertEqual((overlay(env).get("env") or {}).get("COOKIE_SECURE"), "true")

    def test_scenario_cookie_secure_overlay_keeps_base_env_merged(self):
        base = load(BASE)["env"]
        for env in ENVS:
            with self.subTest(env=env):
                merged = merged_env(env)
                self.assertEqual(merged["COOKIE_SECURE"], "true")
                for k, v in base.items():
                    self.assertEqual(merged[k], v, f"base env {k} lost or changed by the {env} overlay")

    def test_scenario_cookie_secure_overlay_keeps_existing_settings(self):
        for env in ENVS:
            with self.subTest(env=env):
                ov = overlay(env)
                self.assertIs(ov["ingress"]["enabled"], True)
                self.assertTrue(ov["ingress"]["host"])

    def test_scenario_cookie_secure_local_compose_leaves_it_unset_boundary(self):
        self.assertNotIn("COOKIE_SECURE", load(BASE).get("env") or {})
        self.assertNotIn("COOKIE_SECURE", (ROOT / "envs/local/values.yaml").read_text())
        for name in ("docker-compose.services.yaml", "platform-e2e/docker-compose.e2e.yaml"):
            p = REPO / name
            if p.exists():
                self.assertNotIn("COOKIE_SECURE", p.read_text(), name)


if __name__ == "__main__":
    unittest.main()
