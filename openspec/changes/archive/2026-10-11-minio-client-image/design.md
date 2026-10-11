## Decisions

- **D1 `bitnamilegacy/minio-client:2025.5.21-debian-12-r4`.** Multi-arch, pullable, frozen archive tag (cannot move).
  `mc` is a static Go binary, so `fetch-mc` can still copy it into `postgres:16-alpine`. Chainguard's image has no
  shell, which the jobs need.
- **D2 `IfNotPresent`.** With a fixed tag the node cache is valid; no pull on every hook run.
- **D3 `MC_CONFIG_DIR=/tmp/.mc` in minio-init.** uid 1001 cannot write `HOME=/`.
- **D4 Static scan, not an image list.** Pins every third-party image, so the next withdrawn `latest` is caught in CI.
