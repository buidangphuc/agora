# platform-gitops

**Desired-state (GitOps) repo** for the marketplace — the single source of truth for
what runs in the cluster. ArgoCD pulls from here and converges the cluster
(auto-sync, self-heal, prune). CI never deploys directly; it only bumps image tags
in `envs/local/values.yaml`.

```
argocd/apps/        child ArgoCD Applications (one per service + infra), synced by the root app-of-apps
charts/<service>/   Helm chart per service (Deployment/Service/ConfigMap/NetworkPolicy; Ingress for gateway+frontend)
charts/infra/       postgres-* (per-service DB), redpanda, opensearch, minio, redis
envs/local/values.yaml   image tags (CI bumps these), host, replicas, resources
```

Local git host: Gitea (`http://gitea.localtest.me`, repo `ci/platform-gitops`).
ArgoCD reads it in-cluster at `http://gitea-http.gitea.svc:3000/ci/platform-gitops.git`.

## Checks

`python3 scripts/check_identity_seed.py` fails if any manifest enables team-identity's
admin seed (`SEED_ADMIN_ENABLED` truthy) or carries a `SEED_ADMIN_PASSWORD` literal.
Run it before pushing; there is no CI wired into this repo yet.
