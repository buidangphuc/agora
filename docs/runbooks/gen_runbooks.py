#!/usr/bin/env python3
"""Generate docs/runbooks/<service>.md for every service in platform-gitops/envs/services from
_template.md. Offline; needs `yq` (mikefarah). Run: python3 docs/runbooks/gen_runbooks.py"""
import json, re, subprocess
from pathlib import Path
from string import Template

HERE = Path(__file__).resolve().parent
GITOPS = HERE.parents[1] / "platform-gitops"
PG = (GITOPS / "platform/postgres/postgres.yaml").read_text()
PG_SERVICES = set(re.search(r'SERVICES:\s*"([^"]+)"', PG).group(1).split())

def load(path):
    out = subprocess.run(["yq", "-o=json", ".", str(path)], capture_output=True, text=True, check=True).stdout
    return json.loads(out) if out.strip() and out.strip() != "null" else {}

def main():
    tpl = Template((HERE / "_template.md").read_text())
    staging_kind = {"team-gateway", "team-frontend"}
    for f in sorted((GITOPS / "envs/services").glob("team-*.yaml")):
        name = f.stem
        v = load(f)
        env = v.get("env") or {}
        sec = v.get("secrets") or {}
        grpc, port = v.get("grpcPort") or 0, v.get("containerPort")
        health = v.get("healthPath")
        health_s = f"HTTP `{health}`" if health else (f"gRPC health (grpc.health.v1) on {grpc}" if grpc else "none configured")
        if health and grpc:
            health_s += f" and gRPC health on {grpc}"
        deps = []
        for k, val in sorted(env.items()):
            if k.endswith("_ADDR") and val and not str(val).startswith("http"):
                deps.append(f"- upstream `{str(val).split(':')[0]}` (`{k}={val}`)")
        if "JWKS_URL" in env:
            deps.append(f"- identity JWKS `{env['JWKS_URL']}`")
        if env.get("KAFKA_BROKERS"):
            deps.append(f"- Kafka/Redpanda `{env['KAFKA_BROKERS']}`")
        for k in ("REDIS_HOST", "RECS_QDRANT_URL"):
            if env.get(k):
                deps.append(f"- `{k}={env[k]}`")
        if name in PG_SERVICES:
            deps.append("- Postgres (own database, `platform/postgres`; `DATABASE_URL` from Vault)")
        if sec.get("enabled"):
            deps.append(f"- Vault via ExternalSecret (`svc/{name}/config` -> Secret `{name}-env`); a missing secret shows as `CreateContainerConfigError`")
        if not deps:
            deps.append("- none declared beyond the cluster itself")
        alerts = []
        if name == "team-gateway":
            alerts += ["- `HighErrorRate` (edge gRPC error rate > 5% for 2m): find the failing upstream by `rpc_service`/status labels in Prometheus, then use that service's runbook.",
                       "- `HighLatencyP95` (edge p95 > 500ms for 5m): same, per upstream."]
        if name == "team-ai":
            alerts.append("- `ServiceDown` (`up{job=\"team-ai\"} == 0` for 1m): pod not ready or `/metrics` unreachable on 8000.")
        if not alerts:
            alerts.append("- none specific. `HighErrorRate`/`HighLatencyP95` are measured at the gateway and can be caused by this service; `ServiceDown` only covers scraped jobs (team-ai, otel-collector).")
        if name == "team-gateway":
            rollback = ("Canary service (Rollout + AnalysisTemplate `team-gateway-canary`).\n"
                        "- Stop a bad canary: `scripts/rollback.sh team-gateway --env staging --abort`\n"
                        "- Previous revision now: `scripts/rollback.sh team-gateway --env staging`\n"
                        "- Durable (survives ArgoCD selfHeal): `scripts/rollback.sh team-gateway --env staging --to <good-tag> && make -C deploy gitops-push`\n"
                        "Details and drill: `docs/runbooks/rollback.md`.")
        else:
            rollback = (f"Plain Deployment (no canary). Revert the tag in git, then publish:\n"
                        f"`scripts/rollback.sh {name} --env local --to <good-tag> && make -C deploy gitops-push`\n"
                        f"(this edits `images.{name}`; with `--env staging` for the staging overlay). A bare `kubectl rollout undo` is reverted by ArgoCD selfHeal.")
        note = " (edge only, with a kind overlay)" if name in staging_kind else " (not deployed in the kind staging overlay)"
        (HERE / f"{name}.md").write_text(tpl.substitute(
            name=name, port=port, port_kind="gRPC" if grpc and port == grpc else "container port",
            health=health_s, deps="\n".join(deps), alerts="\n".join(alerts),
            rollback=rollback, staging_note=note))
        print("wrote", f"{name}.md")

if __name__ == "__main__":
    main()
