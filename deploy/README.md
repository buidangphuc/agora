# deploy/ — local GitOps CI/CD plane

Simulates the **deployment plane** (not AWS data services) so CI/CD is realistic and
runnable locally. GitOps: git is the source of truth; **ArgoCD pulls** and converges
the cluster. CI only builds images + bumps tags in `platform-gitops`.

## Map: local ↔ AWS
| Role | AWS | Local |
|---|---|---|
| Orchestrator | EKS | **kind** (`marketplace`) |
| Registry | ECR | **registry:2** `localhost:5001` |
| Edge / LB | ALB | **ingress-nginx** (host `*.localtest.me`) |
| CD engine | ArgoCD | **ArgoCD** (ns `argocd`) |
| Git host (desired state) | GitHub / CodeCommit | **Gitea** (`gitea.localtest.me`, repo `ci/platform-gitops`) |
| Actions runner | GitHub-hosted | **act** |
| Data infra | RDS / MSK / OpenSearch / S3 | Postgres / Redpanda / OpenSearch / MinIO (in-cluster) |

## Bring it up
```bash
make -C deploy cluster-up     # kind + registry + ingress + argocd + gitea + app-of-apps
make -C deploy images         # retag local :local images -> registry, bump gitops tags
make -C deploy gitops-push    # push tag bumps; ArgoCD auto-syncs
make -C deploy status         # argocd apps + pods
make -C deploy argocd-ui      # ArgoCD UI (admin / printed password)
make -C deploy smoke          # platform-e2e against the cluster ingress
make -C deploy cluster-down
```

## Layout
```
deploy/kind/        kind cluster + registry + ingress + argocd bootstrap
deploy/gitea/       Gitea git host + repo/token/app-of-apps bootstrap
deploy/argocd/      root app-of-apps Application
deploy/images/      retag+push local images to the registry (ECR stand-in)
deploy/act/         local GitHub Actions payloads (P4)
../platform-gitops/ desired-state repo (Helm charts + envs/local + argocd/apps)
```

## GitOps flow
`dev push → CI (act): buf + make check + build/push image + e2e gate + bump tag in
platform-gitops → Gitea → ArgoCD auto-sync → cluster (+ PostSync e2e smoke)`.
Never `kubectl apply`/`helm upgrade` by hand — change `platform-gitops` instead.
