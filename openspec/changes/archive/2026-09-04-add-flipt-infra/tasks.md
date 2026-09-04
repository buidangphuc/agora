# Tasks

## 1. Code — platform-core (local infra compose)
- [x] `infra/docker-compose.yaml`: add a `flipt` service under "SHARED INFRA":
      pinned image (e.g. `flipt/flipt:v1.54.0`), on the default network, container HTTP/UI
      `:8080` (map to a free host port, e.g. `8083:8080`, since `8080` is the gateway) and
      gRPC `:9000` (internal; host-map optional — note `9000` is taken on the host by MinIO).
- [x] Configure a **persisted flag store**: default SQLite backend at `/var/opt/flipt`,
      mounted from a new named volume `flipt_data`; add `flipt_data:` under top-level `volumes:`.
- [x] Add a `healthcheck` hitting Flipt's HTTP health (`/health`) so dependents can wait on it.
- [x] Sanity: `docker compose -p platform-core config` parses; `up -d` shows `flipt` healthy;
      `flipt:9000` (gRPC) and `flipt:8080` (UI) reachable from another container on the network.

## 2. Code — platform-gitops (ArgoCD-deployed infra)
- [x] `platform/infra/flipt.yaml` (new): a `Deployment` (`app: flipt`, namespace `marketplace`,
      same pinned image) + a `Service` exposing `grpc` `9000` and `http` `8080` + a
      `PersistentVolumeClaim` (`flipt-data`) mounted at `/var/opt/flipt` for the flag store.
      Add an HTTP readiness probe on `/health`. Follow the shape of
      `platform/infra/opensearch.yaml` / `valkey.yaml`.
- [x] Confirm the existing `argocd/apps/infra-shared.yaml` Application (sync-wave 0,
      `directory.recurse` over `platform/infra`) picks the new manifest up — **no new
      Application needed**; wave-0 infra precedes the wave-4 services that evaluate flags.
- [x] (No `charts/service` or `envs/local/values.yaml` change — Flipt is upstream infra, not a
      marketplace `service`-chart image; nothing in `images:` to bump.)
- [x] Validate: `kubectl apply --dry-run=client -f platform/infra/flipt.yaml` (or `kustomize`/
      ArgoCD diff) is clean; after sync, `flipt.marketplace.svc:9000`/`:8080` resolve and the
      pod is Ready.

## E2E (platform-e2e)
- [x] No user-facing scenario in this change — it stands up infra only, gates nothing.
      Coverage for the flag-driven behavior lands in `wire-openfeature` (change 2).
- [x] Optional smoke (non-blocking): a stack-up check that `flipt` is healthy alongside the
      other infra containers, if the infra smoke harness wants it. VERIFIED on the kind cluster
      (flipt Deployment 1/1 Running alongside the other marketplace infra).

## Archive
- [x] Flipt healthy in local compose and reachable in-network at `flipt:9000`/`:8080`.
      VERIFIED: `docker compose -p platform-core up -d flipt` → healthy; on `platform-core_default`
      `curl http://flipt:8080/health` → `{"status":"SERVING"}`, `nc -z flipt 9000` → reachable,
      host `http://localhost:8083/health` → SERVING, `flipt.db` present in `/var/opt/flipt` (volume).
- [x] Flipt reconciled by ArgoCD in `marketplace`, Service + PVC present, pod Ready before services.
      VERIFIED on the kind cluster (2026-09-04): ArgoCD app `flipt` Healthy; `service/flipt` exposes
      `9000/TCP,8080/TCP`; `persistentvolumeclaim/flipt-data` Bound (1Gi); pod `flipt` 1/1 Running.
- [x] `make -C platform-e2e features-check` still green (no feature regressions).
      VERIFIED: `make -C platform-e2e features-check` → "All manifests valid; 60/60 automated (100%)". ✓
- [ ] Archive (`/opsx:archive add-flipt-infra`).
