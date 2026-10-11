Feature: Images and gitops content reach the local cluster
  Static, no stack and no cluster: `kubectl kustomize` renders the raw-manifest apps of platform-gitops, and the deploy
  scripts and compose files are parsed. (local-cluster-image-wiring)

  Scenario: No recsys manifest hardcodes a local tag
    When the manifests of the raw-manifest apps are rendered with kustomize
    Then no manifest file carries a localhost:5001 image with a tag, and every rendered image is localhost:5001/<repo> with the newTag of its kustomization

  Scenario: Kustomize tags follow the local values
    When the local values and the kustomizations of the raw-manifest apps are read
    Then each kustomization image tag equals the images value of its image key, and their Applications set no directory

  Scenario: The push list names real compose images
    When the push script, the compose files and the local values are read
    Then every source image of the push list is an image of the compose files, every values image key is on the list, and every repo a gitops manifest pulls from localhost:5001 is on the list

  Scenario: gitops-push does not target origin
    When the Makefile target, the bootstrap script and the publish script are read
    Then none runs git add, git commit or git push origin inside the agora checkout, the target calls publish-gitops.sh, and the publish script pushes to a Gitea remote from a temporary clone

  Scenario: A single-writer volume is replaced, not surged
    When the Deployments of platform-gitops that mount a ReadWriteOnce claim are read
    Then each one with a single replica uses the Recreate strategy, so a rollout never runs two pods on one volume

  Scenario: The broker data dir is on a claim
    When the Redpanda manifest of platform-gitops is read
    Then the redpanda container mounts a volume at `/var/lib/redpanda/data` that is backed by a `PersistentVolumeClaim` declared in the same manifest, and the pod's `fsGroup` lets the `redpanda` user (gid 101) write it

  Scenario: The modelserve images come from the local registry
    When the modelserve app of platform-gitops is rendered with kustomize and the push script is read
    Then every container image of modelserve-router, tei-embed and tei-rerank is a localhost:5001 repo that the push list ships, tei-embed and tei-rerank run tei-fake, and no image is pulled from ghcr.io or carries the `agora/` prefix

  Scenario: No two Applications sync the same path
    When the Applications of platform-gitops are read
    Then their source paths are pairwise distinct, and no path is a parent of another app's path when the parent app recurses into directories
