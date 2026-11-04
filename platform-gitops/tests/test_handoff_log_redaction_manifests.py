"""Manifest checks for change identity-sso-handoff, task 1.6.2 (ingress access-log redaction).

stdlib unittest + `helm` + `yq`, like the other platform-gitops manifest tests.
Run: python3 -m unittest discover -s platform-gitops/tests -v   (from the repo root)
"""
import json
import re
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
APPS = ("team-frontend", "team-frontend-seller")
ENVS = ("local", "staging", "prod")


def yq_json(text: str):
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", "-"], input=text,
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def render_ingress(app: str, env: str) -> dict:
    files = [ROOT / "envs" / env / "values.yaml", ROOT / "envs/services" / f"{app}.yaml",
             ROOT / "envs" / env / "services.yaml", ROOT / "envs" / env / "services" / f"{app}.yaml"]
    cmd = ["helm", "template", app, str(ROOT / "charts/service"), "--namespace", "marketplace"]
    for f in files:
        if f.exists():
            cmd += ["-f", str(f)]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"helm template {app} {env} failed: {res.stderr[-400:]}"
    for doc in re.split(r"(?m)^---\s*$", res.stdout):
        if doc.strip():
            d = yq_json(doc)
            if d.get("kind") == "Ingress":
                return d
    raise AssertionError(f"no Ingress rendered for {app} {env}")


def redaction_text(ingress: dict) -> str:
    ann = (ingress.get("metadata") or {}).get("annotations") or {}
    return "\n".join(str(v) for v in ann.values())


class HandoffLogRedactionChecks:
    def test_scenario_code_redacted_in_access_logs_ingress_names_handoff_code(self):
        for env in ENVS:
            with self.subTest(env=env):
                text = redaction_text(render_ingress(self.APP, env))
                self.assertIn("/auth/handoff", text)
                self.assertIn("code=REDACTED", text)

    def test_scenario_rejected_hand_off_request_is_still_redacted_not_status_gated(self):
        for env in ENVS:
            with self.subTest(env=env):
                text = redaction_text(render_ingress(self.APP, env))
                self.assertIn("code=REDACTED", text)
                self.assertNotIn("$status", text)
                self.assertNotIn("$upstream_status", text)

    def test_scenario_anonymous_request_is_redacted_without_a_session(self):
        for env in ENVS:
            with self.subTest(env=env):
                text = redaction_text(render_ingress(self.APP, env))
                self.assertIn("code=REDACTED", text)
                for session_var in ("$cookie_", "$http_cookie", "$http_authorization"):
                    self.assertNotIn(session_var, text)

    def test_scenario_other_query_parameters_keep_their_value_only_code_is_masked(self):
        for env in ENVS:
            with self.subTest(env=env):
                text = redaction_text(render_ingress(self.APP, env))
                masked = set(re.findall(r"([A-Za-z0-9_]+)=REDACTED", text))
                self.assertEqual(masked, {"code"})


class BuyerHandoffLogRedaction(HandoffLogRedactionChecks, unittest.TestCase):
    APP = "team-frontend"


class SellerHandoffLogRedaction(HandoffLogRedactionChecks, unittest.TestCase):
    APP = "team-frontend-seller"


if __name__ == "__main__":
    unittest.main()
