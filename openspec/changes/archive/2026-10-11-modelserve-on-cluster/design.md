## Context

`platform/infra` is synced by both `infra-shared` and `platform-modelserve`. The modelserve manifests pull an image no
registry serves (router) and an amd64-only image (TEI) on an arm64 node. Compose runs the router built from
`platform-modelserve/` as `modelserve-router:local` and one `tei-fake:local` for both TEI roles.

## Decisions

- **D1 Own directory + kustomize for modelserve.** Same pattern as the other raw-manifest apps (lcw): tagless images,
  `images[].newTag` written by `push-images.sh`, Application without `directory`. Removes the double ownership.
- **D2 tei-fake for both TEI Deployments (user decision).** Two Deployments keep the services `tei-embed` /
  `tei-rerank` and their ports, so the router env is unchanged; the fake is started with `--port` equal to the existing
  container port. Probes use `/healthz`. The model cache claim and the Recreate strategy that guarded it go away
  (no RWO volume left; RollingUpdate is fine).
- **D3 Values keys `platform-modelserve` and `tei-fake`.** Keeps the lcw invariant: every values key is on the push
  list and every kustomization tag equals its values key.
- **D4 Ownership check is static.** Pairwise-distinct paths, plus a prefix check for recursing apps.

## Risks

- The prune of `modelserve-models-cache` deletes the downloaded model cache (none was ever downloaded on arm64).
- Real TEI on an amd64 cluster would need the upstream image back; out of scope for the local cluster.
