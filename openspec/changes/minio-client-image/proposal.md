## Why

During the `modelserve-on-cluster` live check (2026-10-11) the `minio-init` PostSync hook sat in ImagePullBackOff:
`minio/mc:latest` is no longer published (Docker Hub and quay.io both answer "repository does not exist"). A `:latest`
tag is pulled with `imagePullPolicy: Always`, so even the copy cached on the kind node is unusable. Both
`infra-shared` and `platform-modelserve` then wait forever on the hook, and the nightly `postgres-backup` CronJob
(init container `fetch-mc`) and `postgres-backup-bucket-init` fail the same way.

## What Changes

- **platform-gitops:** the three `mc` containers (`infra/minio.yaml` minio-init, `backup/bucket-init.yaml`,
  `backup/cronjob.yaml` fetch-mc) use `bitnamilegacy/minio-client:2025.5.21-debian-12-r4` (pinned, arm64 + amd64,
  `mc` on PATH, static binary) with `imagePullPolicy: IfNotPresent`. The image runs as uid 1001 with `HOME=/`, so
  minio-init sets `MC_CONFIG_DIR=/tmp/.mc` (the other two already do). `fetch-mc` copies
  `/opt/bitnami/minio-client/bin/mc`.
- **platform-e2e:** a static scenario: no third-party image in platform-gitops floats on `latest` or has no tag.
- **FEATURES.yaml (platform-gitops):** one entry.

`platform-core/infra/docker-compose.yaml` (compose `minio-init`) has the same image and is out of scope here: it is
platform-core territory and is reported separately.

## Capabilities

### New Capabilities

### Modified Capabilities
- `local-cluster-image-wiring`: adds "Third-party images are pinned".

## Impact

- The next sync of `infra-shared` runs minio-init with the new image and can complete its PostSync phase.
