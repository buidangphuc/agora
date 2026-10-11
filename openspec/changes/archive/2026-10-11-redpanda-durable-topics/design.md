## Context

`redpanda.yaml` runs a single dev-container broker with no volume. ArgoCD's PostSync hook `redpanda-init` creates the
14 topics. A node restart (`docker stop/start marketplace-control-plane`) recreates the pod without a sync, so the
broker comes back empty and stays empty.

## Decisions

- **D1 Persist the data dir instead of re-running topic creation on start.** A volume keeps records and committed
  offsets too; re-creating topics on start (sidecar or postStart loop) would bring back empty topics and reset every
  consumer group. The hook stays for first provisioning.
- **D2 `Recreate` strategy.** One broker owns an RWO volume; a surge pod could not open it (same rule as
  team-analytics, covered by the existing RWO scenario).
- **D2a `rollingUpdate: null`.** A live Deployment carries the server-defaulted `strategy.rollingUpdate`; a client-side
  apply that only changes `type` to `Recreate` is rejected. An explicit null makes the patch delete it. Applied to every
  Deployment switched to `Recreate` after its first apply (redpanda, team-analytics, tei-embed, tei-rerank); flipt was
  `Recreate` from its first apply.
- **D3 `fsGroup: 101`.** The image runs as `redpanda` (uid/gid 101); local-path volumes are root-owned.
- **D4 2Gi.** Enough for local event volume; retention is the broker default.

## Risks

- The first sync after this change starts on an empty volume; the same sync's PostSync hook recreates topics.
