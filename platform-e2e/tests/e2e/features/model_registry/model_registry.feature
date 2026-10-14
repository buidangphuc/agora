@registry
Feature: Model registry (MLflow on the infra stack)
  A pinned MLflow 3.2.0 server on host port 5050 with a dedicated postgres-mlflow backend store and
  the MinIO bucket mlflow-artifacts as artifact root, plus the py-modelregistry publish gate driven
  against it. Needs only postgres-mlflow, minio, minio-init and mlflow from platform-core/infra.
  Change: add-model-registry, capabilities model-registry and model-publish-gate.

  Scenario: mlflow_server_up - Server is up with the pinned version
    When the version endpoint of the registry on port 5050 is called
    Then the registry answers 200 and the body is "3.2.0"

  Scenario: run_and_artifact_stored - A run and a registered model version are stored with artifacts in MinIO
    Given the probe model "e2e-registry-probe" does not exist in the registry
    When a client logs one run with a model artifact and creates a version of "e2e-registry-probe"
    Then version 1 of "e2e-registry-probe" exists in the registry
    And the artifact object of that run exists in bucket "mlflow-artifacts" on MinIO

  Scenario: bucket_init_idempotent - Bucket creation is idempotent
    Given the probe model "e2e-registry-probe" does not exist in the registry
    And a run with a model artifact is stored in bucket "mlflow-artifacts"
    When the minio-init job runs a second time
    Then the minio-init job exits with status 0
    And the bucket "mlflow-artifacts" still holds exactly the same objects as before

  Scenario: gate_against_real_server - Submit against a real MLflow server
    Given the probe model "e2e-gate-probe" does not exist in the registry
    And a test policy for "e2e-gate-probe" with primary "ndcg@10" and min relative improvement 0.05
    And a weak champion for "e2e-gate-probe" seeded with "ndcg@10" 0.10
    When a stronger candidate with "ndcg@10" 0.20 is submitted for "e2e-gate-probe"
    Then the submit decision outcome is "promoted"
    And in the registry alias "champion" of "e2e-gate-probe" is on version 2
    And in the registry alias "previous" of "e2e-gate-probe" is on version 1
    And version 2 of "e2e-gate-probe" has tag "gate.outcome" equal to "promoted"
