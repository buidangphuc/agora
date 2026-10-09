Feature: The cluster runs the featurestore and hands its dataset to recsys
  Static, no stack needed: the raw manifests of platform-gitops are parsed, and the recsys
  freshness guard script is extracted from the manifest and executed against temp directories.
  (featurestore-cluster-job / featurestore-cluster-job)

  Scenario: The three steps run in order
    When the featurestore CronJob manifest is rendered
    Then its init containers run materialize then parity, its main container runs dataset, and concurrencyPolicy is Forbid

  Scenario: The featurestore is scheduled before recsys
    When the featurestore and recsys CronJobs are rendered
    Then the featurestore schedule fires earlier in the day than recsys and an Application syncs platform/featurestore

  Scenario: The dataset volume is shared
    When the featurestore volume manifests are rendered
    Then the featurestore writes claim featurestore-data and recsys mounts it read-only under DATASET_DIR with no emptyDir

  Scenario: The analytics export reaches the featurestore
    When the analytics export manifests are rendered
    Then team-analytics exports to claim analytics-data and the featurestore mounts it read-only at its input dir

  Scenario: Absent dataset blocks training
    When the recsys freshness guard runs against a missing directory and an empty one
    Then it exits non-zero both times

  Scenario: Stale dataset blocks training
    When the recsys freshness guard runs against a directory with only an old snapshot
    Then it exits non-zero

  Scenario: Fresh dataset passes the guard
    When the recsys freshness guard runs against a directory with a new snapshot
    Then it exits zero

  Scenario: The job is hardened and isolated
    When the featurestore security manifests are rendered
    Then every container is locked down with resources, the NetworkPolicy allows no ingress and only DNS and valkey egress, and no secret is in Git
