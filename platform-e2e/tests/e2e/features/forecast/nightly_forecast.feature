@analytics @batch
Feature: The nightly forecast job writes honest per-listing demand forecasts to Redis
  `python -m forecast run` reads order_facts.parquet from the warehouse export, forecasts 14 days of
  demand per listing and writes `fc:v1:seller:<seller_id>:<listing_id>` plus `fc:v1:meta` to Redis DB 3.
  It runs here as the real job image on the stack network. Scenarios that need a controlled history
  (30 days, 5 days, zero rows, no file) give the image a synthetic export with AS_OF 2026-11-10T02:45:00Z;
  the label-honesty scenario runs over the real analytics export after a paid order has been exported
  (the 300 s export cycle is waited out once). The compose and CronJob scenarios are static.
  (forecast-nightly-job / demand-forecast-job)

  Scenario: A missing export file is a configuration error
    Given an input directory without order_facts.parquet
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then the job exits with code 2 and its stderr names order_facts.parquet
    And the job wrote no fc:v1 key and no fc:v1:meta

  Scenario: A listing with history gets a 14-day key
    Given a listing with 30 days of paid lines before 2026-11-10
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then the job exits with code 0
    And Redis holds the listing key with 14 daily forecasts dated 2026-11-10 to 2026-11-23, each with 0 <= p10 <= p50 <= p90

  Scenario: Keys expire after 48 hours
    Given a listing with 30 days of paid lines before 2026-11-10
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then the job exits with code 0
    And the listing key expires in 48 hours

  Scenario: A listing without paid lines gets no key
    Given a listing "X" with only an unpaid line and a PAID line on the as-of day, and a listing "Y" with 20 days of paid lines
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then Redis holds no fc:v1:seller key for "X"
    And Redis holds a key for "Y"

  Scenario: A short history is a cold start
    Given a listing "C" with 5 days of paid lines and a listing "Y" with 30 days of paid lines
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then the key of "C" has is_cold_start true, model_version seasonal_naive_v1 and 14 daily points
    And the key of "Y" has is_cold_start false

  Scenario: The summary counts the run
    Given 3 listings of which 1 has 5 days of paid lines and 2 have 30 days
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then fc:v1:meta has listings 3, cold_starts 1 and status ok, and its TTL is 48 hours
    And the stdout of the job is one JSON line equal to fc:v1:meta

  Scenario: An empty export writes nothing and succeeds
    Given an export with zero rows
    When the forecast job runs with AS_OF 2026-11-10T02:45:00Z
    Then the job exits with code 0 and its stdout status is "empty"
    And the job wrote no fc:v1 key and no fc:v1:meta

  Scenario: A label never overstates the model
    Given a paid order has reached the analytics export
    When the forecast job runs over the analytics export with AS_OF tomorrow 02:45 UTC
    Then fc:v1:meta has status ok and the paid order's listings each have a key
    And every fc:v1 payload carries lgbm_quantile_v1 only if the meta wape is lower than baseline_wape

  Scenario: The compose job runs as a non-root user
    When the compose service forecast-job of profile forecast is rendered and started
    Then the service mounts analytics_data read-only at /data and writes Redis DB 3
    And the container user id is 1001 and the export mount rejects a write

  Scenario: The CronJob is scheduled after the export and isolated
    When the platform-gitops forecast manifests are rendered
    Then the CronJob schedule is "45 2 * * *", concurrencyPolicy is Forbid, the pod is runAsNonRoot with a read-only root filesystem and the analytics-data volume is readOnly
    And the NetworkPolicy has no ingress and egress only to DNS and valkey port 6379
