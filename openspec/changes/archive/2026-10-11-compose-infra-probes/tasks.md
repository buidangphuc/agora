## 1. Code track (platform-core, root compose)

- [x] 1.1 qdrant healthcheck via `bash -c`; `minio-init` pinned image, `MC_CONFIG_DIR`, `mc` on PATH; README note. Verify: "Infra healthchecks run in the image's shell", "Compose third-party images are pinned".
- [x] 1.2 Root compose: qdrant dependency `service_healthy`, comment dropped. Verify: "The qdrant dependency waits for health".

## 2. e2e / static-test track (platform-e2e)

- [x] 2.1 `ops/lci_local_compose_infra.feature` + steps + binding; two `platform-gitops/FEATURES.yaml` entries. Verify: pytest on lci.

## Evidence

- Static tests: lci (3 scenarios fail on the HEAD compose files, pass after), lcw; black/ruff; openspec validate --strict; features.py --strict; repo_doctor; `docker compose config -q` (config only).
- Live (2026-10-11, compose project agora): `agora-qdrant-1` recreated with the bash probe went from unhealthy to healthy; `minio-init` with the pinned bitnami image created/kept `listing-images` and set the download policy (exit 0).
