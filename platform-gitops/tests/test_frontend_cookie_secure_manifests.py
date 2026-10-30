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
ENVS = ("staging", "prod")
APPS = ("team-frontend", "team-frontend-seller", "team-frontend-admin")


def load(path: Path) -> dict:
    assert path.exists(), f"{path} does not exist"
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", str(path)],
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def base(app: str) -> dict:
    return load(ROOT / f"envs/services/{app}.yaml")


def overlay(app: str, env: str) -> dict:
    return load(ROOT / f"envs/{env}/services/{app}.yaml")


def merged_env(app: str, env: str) -> dict:
    """Helm-style map merge: overlay env keys layered over the base env."""
    return {**(base(app).get("env") or {}), **(overlay(app, env).get("env") or {})}


class CookieSecureChecks:
    APP = ""

    def test_scenario_cookie_secure_staging_and_prod_set_it(self):
        for env in ENVS:
            with self.subTest(env=env):
                self.assertEqual((overlay(self.APP, env).get("env") or {}).get("COOKIE_SECURE"), "true")

    def test_scenario_cookie_secure_overlay_keeps_base_env_merged(self):
        base_env = base(self.APP)["env"]
        for env in ENVS:
            with self.subTest(env=env):
                merged = merged_env(self.APP, env)
                self.assertEqual(merged["COOKIE_SECURE"], "true")
                for k, v in base_env.items():
                    self.assertEqual(merged[k], v, f"base env {k} lost or changed by the {env} overlay")

    def test_scenario_cookie_secure_overlay_keeps_existing_settings(self):
        for env in ENVS:
            with self.subTest(env=env):
                ov = overlay(self.APP, env)
                self.assertIs(ov["ingress"]["enabled"], True)
                self.assertTrue(ov["ingress"]["host"])

    def test_scenario_cookie_secure_local_compose_leaves_it_unset_boundary(self):
        self.assertNotIn("COOKIE_SECURE", base(self.APP).get("env") or {})
        self.assertNotIn("COOKIE_SECURE", (ROOT / "envs/local/values.yaml").read_text())
        for name in ("docker-compose.services.yaml", "platform-e2e/docker-compose.e2e.yaml"):
            p = REPO / name
            if p.exists():
                self.assertNotIn("COOKIE_SECURE", p.read_text(), name)


class FrontendCookieSecureManifest(CookieSecureChecks, unittest.TestCase):
    APP = "team-frontend"


class SellerCookieSecureManifest(CookieSecureChecks, unittest.TestCase):
    APP = "team-frontend-seller"


class AdminCookieSecureManifest(CookieSecureChecks, unittest.TestCase):
    APP = "team-frontend-admin"


class FrontendAppsAreSeparateHosts(unittest.TestCase):
    def test_each_app_has_its_own_name_port_and_host(self):
        ports = {app: base(app)["containerPort"] for app in APPS}
        self.assertEqual(ports, {"team-frontend": 3000, "team-frontend-seller": 3001, "team-frontend-admin": 3002})
        for app in APPS:
            with self.subTest(app=app):
                self.assertEqual(base(app)["name"], app)
                self.assertEqual(base(app)["service"]["port"], ports[app])
        for env in ENVS:
            with self.subTest(env=env):
                hosts = [overlay(app, env)["ingress"]["host"] for app in APPS]
                self.assertEqual(len(set(hosts)), len(APPS), hosts)

    def test_seller_and_admin_point_at_the_gateway_like_the_buyer(self):
        buyer = base("team-frontend")["env"]["GATEWAY_URL"]
        for app in APPS[1:]:
            with self.subTest(app=app):
                self.assertEqual(base(app)["env"]["GATEWAY_URL"], buyer)


if __name__ == "__main__":
    unittest.main()
