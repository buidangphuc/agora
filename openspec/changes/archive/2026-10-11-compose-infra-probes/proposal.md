## Why

Two defects in the local compose infra (`platform-core/infra/docker-compose.yaml`), both found while running the stack:

- The `qdrant` healthcheck `["CMD-SHELL", "exec 3<>/dev/tcp/127.0.0.1/6333"]` never passes. `CMD-SHELL` runs `/bin/sh`
  (dash), which has no `/dev/tcp` ("cannot create /dev/tcp/...: Directory nonexistent"); the same probe under
  `bash -c` succeeds in `qdrant/qdrant:v1.19.0`. `agora-qdrant-1` has been "unhealthy" for 3 days. Because of it the
  root `docker-compose.services.yaml` waits on qdrant with `service_started` plus a comment explaining the workaround.
- `minio-init` uses `minio/mc:latest`, which is no longer published. Gitops already moved to
  `bitnamilegacy/minio-client:2025.5.21-debian-12-r4` (see archived `minio-client-image`).

## What Changes

- **platform-core:** qdrant healthcheck becomes `["CMD", "bash", "-c", ...]`; `minio-init` uses the pinned bitnami
  client image with `MC_CONFIG_DIR=/tmp/.mc` (uid 1001, `HOME=/`) and calls `mc` from PATH. README note updated.
- **root compose:** the `team-ai` dependency on qdrant is `service_healthy`; the workaround comment is dropped.
- **platform-e2e:** one static feature, `ops/lci_local_compose_infra.feature`.
- **FEATURES.yaml (platform-gitops):** two entries.

## Capabilities

### New Capabilities
- `local-compose-infra`: healthchecks run in the image's shell; compose third-party images are pinned.

### Modified Capabilities

## Impact

- `team-ai-svc` now starts only after qdrant answers on 6333. Already-running containers are untouched until recreated.
