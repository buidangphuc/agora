## Why

After `redpanda-durable-topics` the local kind cluster still could not run the model-serving tier, and that blocked
more than embeddings:

1. `modelserve-router` pulls `agora/platform-modelserve:latest`, which no registry has; `push-images.sh` does not
   ship the image the stack builds (`modelserve-router:local`).
2. `tei-embed` / `tei-rerank` pull `ghcr.io/huggingface/text-embeddings-inference:cpu-1.5`, which is published for
   amd64 only; the kind node is arm64, so both stay in ImagePullBackOff.
3. Two ArgoCD Applications (`infra-shared` and `platform-modelserve`) sync the same path `platform/infra`, so every
   infra resource has two owners. While `modelserve-router` is unhealthy, `infra-shared` never finishes its sync and its
   PostSync hook `redpanda-init` never runs: a fresh broker gets no topics.

## What Changes

- **platform-gitops:** `platform/infra/modelserve.yaml` moves to `platform/modelserve/` with a `kustomization.yaml`
  (raw-manifest app, same shape as recsys/featurestore). The router image is `localhost:5001/platform-modelserve`;
  `tei-embed` and `tei-rerank` run `localhost:5001/tei-fake` (the compose stand-in, `platform-e2e/fakes/tei_fake`) on
  their existing ports, without the model cache volume. `argocd/apps/platform-modelserve.yaml` points at
  `platform/modelserve` (kustomize, no `directory`); `infra-shared` keeps `platform/infra`, which no longer holds
  modelserve. `envs/local/values.yaml` gains the two image keys.
- **deploy:** `images/push-images.sh` maps `platform-modelserve` → `modelserve-router:local` and `tei-fake` →
  `tei-fake:local`, and writes their tags into the new kustomization.
- **platform-e2e:** static scenarios: no two Applications sync the same path; the modelserve images come from the
  local registry. Existing lcw / modelserve static steps follow the move.

On the local cluster embeddings and rerank are the deterministic fake, exactly as in compose: RAG on the cluster
checks wiring, not retrieval quality (decision of 2026-10-11).

## Capabilities

### New Capabilities

### Modified Capabilities
- `local-cluster-image-wiring`: adds "Every cluster workload pulls an image the local registry can serve" and "Each
  gitops path has one owning Application".

## Impact

- The first sync after this change prunes the old modelserve objects from `infra-shared`'s view and recreates them
  under `platform-modelserve`; the `modelserve-models-cache` claim is pruned (the fake needs no cache).
- `make -C deploy images` must run once to push the two new images.
