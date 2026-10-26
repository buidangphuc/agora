"""Static manifest checks: every service that reads UPSTREAM_AUDIT_ADDR is wired to team-audit.

Audited admin mutations fail closed (Unavailable) when UPSTREAM_AUDIT_ADDR is unset, and the
team-audit NetworkPolicy (ADR-0010) drops callers missing from its allowFrom. Both the
ApplicationSet baseline (envs/services/<svc>.yaml) and the local per-service Application
(argocd/apps/<svc>.yaml, inline helm values) must carry the wiring. Also pins the synchronous
gRPC peers of team-order (team-payment, team-engagement) and team-ai (team-domain).
Same style as test_ga4_relay_manifests.py: stdlib unittest + `yq` (mikefarah).
Run: python3 -m unittest discover -s platform-gitops/tests -v   (from the repo root)
"""
import json
import re
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent          # platform-gitops/
WORKSPACE = ROOT.parent                                 # agora polyrepo root
AUDIT_ADDR = "team-audit:50066"
AUDIT_READ = re.compile(r'env:"UPSTREAM_AUDIT_ADDR"|Getenv\("UPSTREAM_AUDIT_ADDR"\)|config\.Audit\b')
# Known producers at the time of writing; discovery must find at least these, so the
# test cannot pass vacuously if the config scan breaks.
KNOWN_CALLERS = {"team-domain", "team-engagement", "team-identity", "team-order",
                 "team-payment", "team-promotion", "team-verification"}


def yq_json(text: str) -> dict:
    out = subprocess.run(["yq", "-o=json", "-I=0", ".", "-"], input=text,
                         capture_output=True, text=True, check=True).stdout.strip()
    return json.loads(out) if out and out != "null" else {}


def baseline_values(svc: str) -> dict:
    path = ROOT / "envs/services" / f"{svc}.yaml"
    assert path.exists(), f"{path} does not exist"
    return yq_json(path.read_text())


def local_app_values(svc: str) -> dict:
    path = ROOT / "argocd/apps" / f"{svc}.yaml"
    assert path.exists(), f"{path} does not exist"
    app = yq_json(path.read_text())
    inline = [s["helm"]["values"] for s in app["spec"]["sources"]
              if s.get("helm", {}).get("values")]
    assert len(inline) == 1, f"{path}: expected one source with inline helm values"
    return yq_json(inline[0])


def audit_callers() -> set:
    callers = set()
    for cfg in WORKSPACE.glob("team-*/internal/config/config.go"):
        if AUDIT_READ.search(cfg.read_text()):
            callers.add(cfg.parents[2].name)
    return callers


def both(svc: str):
    return (("envs/services", baseline_values(svc)), ("argocd/apps", local_app_values(svc)))


def allow_from(values: dict) -> list:
    return (values.get("networkPolicy") or {}).get("allowFrom") or []


def policies(svc: str):
    """The NetworkPolicy values to check for svc. The baseline must declare one (deployed
    envs are default-deny). A local Application may omit it (local is exempt per ADR-0010),
    but one that declares a policy must admit the same callers."""
    base = baseline_values(svc)
    assert (base.get("networkPolicy") or {}).get("enabled"), f"envs/services/{svc}.yaml: networkPolicy disabled"
    yield "envs/services", base
    local = local_app_values(svc)
    if (local.get("networkPolicy") or {}).get("enabled"):
        yield "argocd/apps", local


class AuditWiring(unittest.TestCase):
    def setUp(self):
        self.callers = audit_callers()

    def test_discovery_finds_known_audit_producers(self):
        self.assertLessEqual(KNOWN_CALLERS, self.callers)

    def test_every_audit_caller_sets_upstream_audit_addr(self):
        for svc in sorted(self.callers):
            for where, values in both(svc):
                with self.subTest(svc=svc, file=where):
                    env = values.get("env") or {}
                    self.assertEqual(env.get("UPSTREAM_AUDIT_ADDR"), AUDIT_ADDR)

    def test_team_audit_network_policy_admits_every_caller(self):
        for where, values in policies("team-audit"):
            with self.subTest(file=where):
                missing = self.callers - set(allow_from(values))
                self.assertFalse(missing, f"team-audit allowFrom lacks {sorted(missing)}")


class SyncGrpcPeers(unittest.TestCase):
    def assert_admits(self, target: str, peers: set):
        for where, values in policies(target):
            with self.subTest(target=target, file=where):
                missing = peers - set(allow_from(values))
                self.assertFalse(missing, f"{target} allowFrom lacks {sorted(missing)}")

    def test_team_order_admits_payment_and_engagement(self):
        self.assert_admits("team-order", {"team-payment", "team-engagement"})

    def test_team_ai_admits_domain(self):
        self.assert_admits("team-ai", {"team-domain"})


if __name__ == "__main__":
    unittest.main()
