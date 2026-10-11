## Decisions

- **D1 `CMD` + `bash -c`, not `CMD-SHELL`.** `CMD-SHELL` is hard-wired to `/bin/sh`; the qdrant image's sh is dash
  without `/dev/tcp`. Naming bash is explicit and keeps the no-curl TCP probe.
- **D2 Same mc image as gitops.** One pinned client everywhere; `mc` is on PATH, so the entrypoint drops `/usr/bin/`.
  `MC_CONFIG_DIR=/tmp/.mc` because uid 1001 cannot write `HOME=/`.
- **D3 Only the qdrant `service_started` is flipped.** The `depends_on: [qdrant, redis]` list form of the `jobs`
  profile (ALS trainer) already means "started" for both and is not a workaround; it is left alone.
- **D4 Static checks.** Compose files are parsed as YAML; no docker daemon. The pin scan has an explicit
  exceptions list; it is empty because every other image in `platform-core/infra/docker-compose.yaml` is already
  tagged (README claimed `minio/minio:latest`, which is stale: it is a `RELEASE.` tag).
