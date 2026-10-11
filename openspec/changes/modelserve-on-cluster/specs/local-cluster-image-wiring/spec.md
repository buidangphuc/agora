## ADDED Requirements

### Requirement: Every cluster workload pulls an image the local registry can serve

The model-serving Deployments of `platform-gitops` (`modelserve-router`, `tei-embed`, `tei-rerank`) SHALL pull from
`localhost:5001` an image that `deploy/images/push-images.sh` pushes, with the tag set by the kustomization of their
app; the TEI pods run the compose stand-in `tei-fake`, which is built for the node's architecture.

#### Scenario: The modelserve images come from the local registry

- **WHEN** the modelserve app of platform-gitops is rendered with kustomize and the push script is read
- **THEN** every container image of modelserve-router, tei-embed and tei-rerank is a localhost:5001 repo that the push list ships, tei-embed and tei-rerank run tei-fake, and no image is pulled from ghcr.io or carries the `agora/` prefix

### Requirement: Each gitops path has one owning Application

No two ArgoCD Applications in `platform-gitops/argocd/apps` SHALL sync the same source path, so every resource has one
owner and one app's unhealthy workload cannot hold another app's sync hooks.

#### Scenario: No two Applications sync the same path

- **WHEN** the Applications of platform-gitops are read
- **THEN** their source paths are pairwise distinct, and no path is a parent of another app's path when the parent app recurses into directories
