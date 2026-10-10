# local-cluster-image-wiring Specification

## Purpose
How images built by the local compose stack and the platform-gitops tree reach the local kind cluster: which image
and tag each app runs, how gitops content is published to the local Gitea without touching the agora repository, and
how single-writer volumes are rolled out.

## Requirements

### Requirement: Images reach the cluster through the pushed tag

The raw-manifest apps `platform/recsys`, `platform/featurestore`, `platform/team-analytics` and `platform/search-indexer`
SHALL NOT hardcode a tag on a `localhost:5001` image; each directory SHALL hold a `kustomization.yaml` whose `images[]`
entries name every such image, and its Application SHALL NOT force `directory` mode. The `newTag` of each entry SHALL equal
`images.<key>` of `envs/local/values.yaml`.

#### Scenario: No recsys manifest hardcodes a local tag

- **WHEN** the manifests of the raw-manifest apps are rendered with kustomize
- **THEN** no manifest file carries a `localhost:5001/...:<tag>` image, and every rendered image is `localhost:5001/<repo>` with the `newTag` of its kustomization

#### Scenario: Kustomize tags follow the local values

- **WHEN** the local values and the kustomizations of the raw-manifest apps are read
- **THEN** each kustomization image tag equals the `images` value of its image key, and their Applications set no `directory`

### Requirement: The push list matches the compose images

`deploy/images/push-images.sh` SHALL list every key of `images:` in `envs/local/values.yaml`, each with the `image:` name a
compose service declares, and SHALL resolve the built image from `agora-<name>` or `<name>`.

#### Scenario: The push list names real compose images

- **WHEN** the push script, the compose files and the local values are read
- **THEN** every source image of the push list is an `image:` of the compose files, every values image key is on the list, and every repo a gitops manifest pulls from `localhost:5001` is on the list

### Requirement: Publishing gitops never commits to the agora repo

`make -C deploy gitops-push` and `gitea/bootstrap-gitea.sh` SHALL publish `platform-gitops/` to the local Gitea through
`gitea/publish-gitops.sh`, which SHALL work in a scratch clone, push only to the Gitea remote, and refuse a non-local host.

#### Scenario: gitops-push does not target origin

- **WHEN** the Makefile target, the bootstrap script and the publish script are read
- **THEN** none runs `git add`, `git commit` or `git push origin` inside the agora checkout, the target calls `publish-gitops.sh`, and the publish script pushes to a Gitea remote from a temporary clone

### Requirement: A single-writer volume is never mounted by two pods

A Deployment in `platform-gitops` that mounts a `ReadWriteOnce` claim with one replica (team-analytics: DuckDB holds a
file lock on `analytics-data`) SHALL use the `Recreate` strategy, so a rolling update never starts a second pod that
cannot open the volume.

#### Scenario: A single-writer volume is replaced, not surged

- **WHEN** the Deployments of platform-gitops that mount a `ReadWriteOnce` claim are read
- **THEN** each one with a single replica uses the `Recreate` strategy, so a rollout never runs two pods on one volume

### Requirement: The broker keeps its topics across a restart

The Redpanda Deployment in `platform-gitops/platform/infra/redpanda.yaml` SHALL mount a `PersistentVolumeClaim` at its
data directory `/var/lib/redpanda/data`, so topics, records and consumer group offsets survive a pod or node restart;
topic provisioning by the `redpanda-init` hook alone is not enough because the hook runs only on an ArgoCD sync.

#### Scenario: The broker data dir is on a claim

- **WHEN** the Redpanda manifest of platform-gitops is read
- **THEN** the redpanda container mounts a volume at `/var/lib/redpanda/data` that is backed by a `PersistentVolumeClaim` declared in the same manifest, and the pod's `fsGroup` lets the `redpanda` user (gid 101) write it
