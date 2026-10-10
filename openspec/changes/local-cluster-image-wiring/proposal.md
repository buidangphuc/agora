## Why

A live check of the local kind cluster (archived `recs-on-cluster`) found three wiring defects in the local GitOps plane:

1. `deploy/images/push-images.sh` maps source images that are not the ones the stack builds (the stack runs under an
   isolation override that names images `agora-<name>:local`; the bare `<name>:local` can be a stale image), and it
   skips most services in `envs/local/values.yaml`.
2. The raw-manifest apps (`platform/recsys`, `featurestore`, `team-analytics`, `search-indexer`) hardcode
   `localhost:5001/<image>:local`, so a tag bump in `envs/local/values.yaml` never reaches them, and a fixed `:local`
   tag with `IfNotPresent` keeps serving a stale node cache.
3. `make -C deploy gitops-push` and `gitea/bootstrap-gitea.sh` run `git add -A && git commit && git push origin main`
   inside `platform-gitops/`, which is a directory of the agora monorepo (origin = GitHub), not a repository.

## What Changes

- **deploy:** `images/push-images.sh` maps every `images:` key of `envs/local/values.yaml` to the compose `image:` name,
  resolves the built image (`agora-<name>:local` or `<name>:local`, newest wins), tags with a content-derived tag
  (`sha-<12 hex>`; `TAG=` overrides) and writes it to `values.yaml` and to each raw-manifest `kustomization.yaml`.
  New `gitea/publish-gitops.sh` publishes the platform-gitops tree to the local Gitea through a scratch clone;
  `make gitops-push` and `bootstrap-gitea.sh` call it and never touch the agora repo.
- **platform-gitops:** `platform/{recsys,featurestore,team-analytics,search-indexer}` manifests carry a tagless image and
  a `kustomization.yaml` with `images[].newTag`; their Applications drop `directory.recurse` so ArgoCD builds with
  kustomize. `search-indexer` gets `command: ["/indexer"]` (the binary ships in the team-search image).
- **platform-e2e:** static scenarios `lcw_*`; the `roc_*` image and sync scenarios follow the new shape.

No service code, compose file, or proto changes.

## Capabilities

### New Capabilities
- `local-cluster-image-wiring`: how images and gitops content reach the local cluster.

### Modified Capabilities
- None.

## Non-goals

- Starting the kind cluster; verification is static (render, parse, `bash -n`).
- Staging/prod overlays (they have no live cluster).
- Making `envs/local/values.yaml` itself non-tracked; the script keeps writing it as before.
