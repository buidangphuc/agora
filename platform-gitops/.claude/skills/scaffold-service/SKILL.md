---
name: scaffold-service
description: >-
  Scaffold a new service into the platform-gitops repo (ArgoCD + the generic
  charts/service Helm chart). Use when adding GitOps/deployment for a new
  marketplace service on the local kind stack — it generates argocd/apps/<svc>.yaml,
  registers the image tag in envs/local/values.yaml, and (when the service needs
  secrets) appends it to the Vault SERVICES list. Handles http / grpc / worker
  variants and optional ingress. Triggers: "thêm service vào gitops", "tạo chart
  cho service mới", "scaffold gitops service", "new service chart", "deploy service
  mới lên kind", "đăng ký service với argocd/vault".
---

# Scaffold a platform-gitops service

This repo deploys via **ArgoCD + one generic Helm chart** (`charts/service`). A new
service is NOT a new chart — it's an ArgoCD `Application` that renders `charts/service`
with inline values, plus an image-tag entry and (optionally) a Vault registration.
Adding a service = **3 small edits**, which this skill generates.

## 1. Gather inputs

Ask the user for anything not given (infer sensible defaults; confirm the name):

| Input | Meaning | Default |
|---|---|---|
| `name` | service name, e.g. `team-ai` (also the k8s Service + Vault role) | required |
| `kind` | `http` (has an HTTP port + optional ingress) / `grpc` (gRPC port) / `worker` (no inbound port) | `http` |
| `containerPort` | the port the container listens on (e.g. `50060` gRPC, `8000` http) | `8080` |
| `grpcPort` | extra gRPC port when the service also serves gRPC alongside http (optional) | none |
| `image` | registry image repo | `localhost:5001/<name>` |
| `ingress` | expose via nginx ingress? host, e.g. `marketplace.localtest.me` | off |
| `secrets` | needs Vault secrets (DB creds, config)? | on for stateful, off for mock/stateless |
| `jwtShared` | pull the RS256 signing key (`JWT_PRIVATE_KEY` + `JWT_KID`) from `svc/shared/jwt` — identity only (ADR-0006) | off |
| `env` | inline literal env vars (map) | `{}` |
| `syncWave` | ArgoCD sync-wave (services usually `"4"`, infra lower) | `"4"` |
| `namespace` | target namespace | `marketplace` |

Read `argocd/apps/team-search.yaml` (a clean reference) and `charts/service/values.yaml`
(the full knob surface) before generating, so the output matches current conventions.

## 2. Generate `argocd/apps/<name>.yaml`

Write the Application. Include a block only when relevant (omit `grpcPort`, `ingress`,
`networkPolicy` if unused). Keep the two-source shape (values ref + chart path) exactly.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: <name>
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "<syncWave>"
spec:
  project: default
  destination:
    server: https://kubernetes.default.svc
    namespace: <namespace>
  sources:
    - repoURL: http://gitea-http.gitea.svc.cluster.local:3000/ci/platform-gitops.git
      targetRevision: main
      ref: values
    - repoURL: http://gitea-http.gitea.svc.cluster.local:3000/ci/platform-gitops.git
      targetRevision: main
      path: charts/service
      helm:
        releaseName: <name>
        valueFiles:
          - $values/envs/local/values.yaml
        values: |
          name: <name>
          image:
            repo: <image>
            pullPolicy: Always      # kind + :local tag → must re-pull on each build
          containerPort: <containerPort>
          service:
            port: <containerPort>
          # grpc: add grpcPort so the gateway can reach it (chart exposes a 2nd svc port)
          grpcPort: <grpcPort>
          # http+ingress:
          healthPath: /healthz
          ingress:
            enabled: true
            host: <ingressHost>
            path: /
          env:
            <KEY>: "<value>"
          secrets:
            enabled: <secrets>
            jwtShared: <jwtShared>
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### Autoscaling (optional)

To make a service scale under load, add `resources` (HPA needs a CPU **request**) and an
`autoscaling` block. Requires metrics-server in the cluster. When enabled, the Deployment
drops its static `replicas` and the HPA owns the count.

```yaml
          resources:
            requests: { cpu: 100m, memory: 128Mi }
            limits:   { cpu: 500m, memory: 512Mi }
          autoscaling:
            enabled: true
            minReplicas: 1
            maxReplicas: 4
            targetCPUUtilizationPercentage: 50
            # targetMemoryUtilizationPercentage: 70   # optional
            # behavior: { scaleDown: { stabilizationWindowSeconds: 60 } }  # faster scale-down
```

Verify after deploy: `kubectl -n <ns> get hpa <name>` (TARGETS shows `cpu: X%/50%`).

Variant rules:
- **worker**: omit `service`, `grpcPort`, `healthPath`, `ingress` (no inbound). Keep `env`, `secrets`.
- **grpc**: set `grpcPort` = the gRPC port; `containerPort`/`service.port` may be the HTTP/health port (e.g. team-ai: containerPort 8000, grpcPort 50060). If gRPC-only, set both to the gRPC port and drop `healthPath`.
- **http**: set `healthPath: /healthz`; add `ingress` only if externally reachable.
- Drop `pullPolicy: Always` only if the service image tag is immutable (not `:local`).

## 3. Register the image tag — `envs/local/values.yaml`

Add the service under `images:` so `charts/service` resolves its tag:

```yaml
images:
  ...
  <name>: local
```

## 4. Register with Vault (only if `secrets.enabled: true`)

Append `<name>` to the space-separated `SERVICES` string in
`platform/vault-config/vault-config.yaml` (the ConfigMap, ~line 30). The seed Job then
creates `svc/<name>/config`, a read policy, and a k8s auth role named `<name>`. If the
service needs config keys beyond the defaults, note them for the user to add to the
`vault kv put "svc/${svc}/config" ...` block.

Skip this step entirely for `secrets.enabled: false` services (mock/stateless) — they
don't get an ExternalSecret.

## 5. Validate

Render the chart to catch errors before ArgoCD sees them:

```bash
helm template <name> charts/service \
  -f envs/local/values.yaml \
  --set name=<name> --set image.repo=<image> --set containerPort=<containerPort> \
  | head -40
```

If `kubeconform` is available, pipe through it (`| kubeconform -strict -summary`).
If a NetworkPolicy restricts the target (e.g. gateway → this service), also set
`networkPolicy.enabled: true` + `allowFrom: [<caller>]` in the app values.

## 6. Ship it

The changes are inert until pushed to the in-cluster gitea repo that ArgoCD watches:

```bash
cd platform-gitops
git add argocd/apps/<name>.yaml envs/local/values.yaml platform/vault-config/vault-config.yaml
git commit -m "feat: scaffold <name> service (argocd app + image tag)"
git push origin main            # remote = gitea; ArgoCD auto-syncs
```

Then, because the `:local` tag is unchanged, force a fresh pull:
```bash
kubectl -n argocd annotate app <name> argocd.argoproj.io/refresh=hard --overwrite
# after ArgoCD applies:
kubectl -n marketplace rollout status deploy/<name>
```

## Reference facts (this repo)

- Gateway upstreams: to route the gateway to a new backend, also add
  `UPSTREAM_<X>_ADDR: "<name>:<port>"` to `argocd/apps/team-gateway.yaml` env.
- Service DNS inside the cluster: `<name>.<namespace>.svc` (short `<name>` within the same ns).
- Ingress hosts in use: `marketplace.localtest.me` (frontend), `api.localtest.me` (gateway).
- The chart's full knob surface is `charts/service/values.yaml` — never invent keys; use those.
