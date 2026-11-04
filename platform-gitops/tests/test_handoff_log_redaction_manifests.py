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


def render_docs(app: str, env: str) -> list:
    files = [ROOT / "envs" / env / "values.yaml", ROOT / "envs/services" / f"{app}.yaml",
             ROOT / "envs" / env / "services.yaml", ROOT / "envs" / env / "services" / f"{app}.yaml"]
    cmd = ["helm", "template", app, str(ROOT / "charts/service"), "--namespace", "marketplace"]
    for f in files:
        if f.exists():
            cmd += ["-f", str(f)]
    res = subprocess.run(cmd, capture_output=True, text=True)
    assert res.returncode == 0, f"helm template {app} {env} failed: {res.stderr[-400:]}"
    return [yq_json(doc) for doc in re.split(r"(?m)^---\s*$", res.stdout) if doc.strip()]


def render_ingress(app: str, env: str) -> dict:
    for d in render_docs(app, env):
        if d.get("kind") == "Ingress":
            return d
    raise AssertionError(f"no Ingress rendered for {app} {env}")


def config_text(app: str, env: str) -> str:
    parts = []
    for d in render_docs(app, env):
        if d.get("kind") == "Ingress":
            parts.append(redaction_text(d))
        elif d.get("kind") == "ConfigMap":
            parts.extend(str(v) for v in (d.get("data") or {}).values())
    return "\n".join(parts)


def log_directives(text: str) -> list:
    return [ln for ln in text.splitlines() if re.search(r"\b(access_log|log_format)\b|log-format", ln)]


def redactor(text: str):
    cond = re.search(r'if\s*\(\s*\$request_uri\s*~\s*"([^"]+)"\s*\)', text)
    repl = re.search(r'set\s+\$handoff_log_uri\s+"([^"]+)"\s*;', text)
    assert cond and repl, "no `if ($request_uri ~ \"...\")` + `set $handoff_log_uri \"...\"` pair in rendered config"
    pattern = re.compile(cond.group(1))
    template = re.sub(r"\$\{(\d+)\}", r"\\g<\1>", repl.group(1))

    def apply(uri: str) -> str:
        m = pattern.search(uri)
        return m.expand(template) if m else uri
    return apply


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


    def test_scenario_code_redacted_in_access_logs_variable_is_written_to_the_access_log(self):
        for env in ENVS:
            with self.subTest(env=env):
                text = config_text(self.APP, env)
                lines = log_directives(text)
                self.assertTrue(any("$handoff_log_uri" in ln for ln in lines),
                                f"no access_log/log_format line references $handoff_log_uri: {lines}")

    def test_scenario_code_redacted_in_access_logs_raw_request_is_not_logged(self):
        for env in ENVS:
            with self.subTest(env=env):
                for ln in log_directives(config_text(self.APP, env)):
                    self.assertIsNone(re.search(r"\$request(_uri)?\b", ln), ln)

    def test_scenario_code_redacted_in_access_logs_first_middle_last_and_only_parameter(self):
        for env in ENVS:
            with self.subTest(env=env):
                r = redactor(config_text(self.APP, env))
                cases = {
                    "/auth/handoff?code=SECRET": "/auth/handoff?code=REDACTED",
                    "/auth/handoff?code=SECRET&next=%2Fdash&state=abc": "/auth/handoff?code=REDACTED&next=%2Fdash&state=abc",
                    "/auth/handoff?next=%2Fdash&code=SECRET&state=abc": "/auth/handoff?next=%2Fdash&code=REDACTED&state=abc",
                    "/auth/handoff?next=%2Fdash&state=abc&code=SECRET": "/auth/handoff?next=%2Fdash&state=abc&code=REDACTED",
                }
                for raw, want in cases.items():
                    self.assertEqual(r(raw), want)
                    self.assertNotIn("SECRET", r(raw))

    def test_scenario_other_query_parameters_keep_their_value_regex_leaves_them_intact(self):
        for env in ENVS:
            with self.subTest(env=env):
                r = redactor(config_text(self.APP, env))
                untouched = [
                    "/auth/handoff",
                    "/auth/handoff?next=%2Fdash&state=abc",
                    "/auth/handoff?xcode=keep&state=abc",
                    "/auth/handoff?a=1&xcode=keep",
                    "/auth/handoff?postcode=keep",
                    "/products?code=PUBLIC&page=2",
                    "/other/auth/handoff?code=PUBLIC",
                ]
                for uri in untouched:
                    self.assertEqual(r(uri), uri)

    def test_scenario_rejected_and_anonymous_requests_are_redacted_by_uri_alone(self):
        for env in ENVS:
            with self.subTest(env=env):
                r = redactor(config_text(self.APP, env))
                self.assertEqual(r("/auth/handoff?code=BAD&error=access_denied"),
                                 "/auth/handoff?code=REDACTED&error=access_denied")
                self.assertEqual(r("/auth/handoff?code="), "/auth/handoff?code=REDACTED")


class BuyerHandoffLogRedaction(HandoffLogRedactionChecks, unittest.TestCase):
    APP = "team-frontend"


class SellerHandoffLogRedaction(HandoffLogRedactionChecks, unittest.TestCase):
    APP = "team-frontend-seller"


if __name__ == "__main__":
    unittest.main()
