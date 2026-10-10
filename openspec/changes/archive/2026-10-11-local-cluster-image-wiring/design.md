## Context

`platform-gitops/` has no `.git` of its own; `git -C platform-gitops` resolves to the agora repo. Helm apps take the tag
from `envs/local/values.yaml` through `charts/service`; raw-manifest apps (`directory.recurse`) cannot template.

## Decisions

- **Kustomize for raw-manifest apps.** ArgoCD builds a path that holds a `kustomization.yaml` with kustomize (an explicit
  `directory:` would force plain directory mode, so it is removed). Manifests keep a tagless image; the tag lives in
  `images[].newTag`. `kubectl kustomize` renders it offline. A Helm chart per app was rejected: more churn for
  single-Deployment apps, and the CronJobs/NetworkPolicies are already plain YAML.
- **Two write targets, one writer.** `values.yaml` stays the tag record for Helm apps; kustomize cannot read it, so
  `push-images.sh` writes both and a static scenario asserts they agree.
- **Unique tags.** Default tag `sha-<12 hex of image id>`: a rebuild changes the manifest, so the Deployment rolls and
  the node never reuses a stale `:local`. `TAG=local` keeps the old behaviour on demand.
- **Mapping source = compose.** MAP lists `<values-key> <registry-repo> <compose-image>`; the compose `image:` names are
  the contract, and the built image is resolved from `agora-<name>` or `<name>` (newest `Created`), because the e2e
  isolation override renames them. `search-indexer` reuses the team-search image.
- **Publish via scratch clone.** `publish-gitops.sh` clones the Gitea repo, `rsync --delete`s the working tree over it,
  commits there if changed and pushes to Gitea only (non-force, history kept). It refuses a non-local host. Alternative
  `git subtree split` was rejected: it only sees committed content, and the working-tree edits are what must be deployed.

## Risks

- `push-images.sh` still edits tracked files in the agora checkout (`values.yaml`, `kustomization.yaml`); the diff is the
  local tag bump and is never committed by these scripts.
- A tag written to `values.yaml` for Helm apps and not yet published would diverge until `make gitops-push`.
