## ADDED Requirements

### Requirement: The broker keeps its topics across a restart

The Redpanda Deployment in `platform-gitops/platform/infra/redpanda.yaml` SHALL mount a `PersistentVolumeClaim` at its
data directory `/var/lib/redpanda/data`, so topics, records and consumer group offsets survive a pod or node restart;
topic provisioning by the `redpanda-init` hook alone is not enough because the hook runs only on an ArgoCD sync.

#### Scenario: The broker data dir is on a claim

- **WHEN** the Redpanda manifest of platform-gitops is read
- **THEN** the redpanda container mounts a volume at `/var/lib/redpanda/data` that is backed by a `PersistentVolumeClaim` declared in the same manifest, and the pod's `fsGroup` lets the `redpanda` user (gid 101) write it
