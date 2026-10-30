Feature: The rendered GitOps manifests carry the deploy-runtime guarantees
  Static, no stack needed: `helm template` runs on platform-gitops/charts/service with the same
  values layering the ApplicationSet uses (envs/<env>/values.yaml, envs/services/<svc>.yaml,
  envs/<env>/services.yaml, envs/<env>/services/<svc>.yaml).
  (port-edge-authz-residuals / deploy-runtime)

  Scenario: The rendered gateway knows its environment
    When charts/service is rendered for team-gateway against the staging overlay and again against the prod overlay
    Then the gateway container has ENV "staging" and "production" respectively

  Scenario: team-ai renders an ingress allow-list
    When charts/service is rendered for team-ai with its service values
    Then a NetworkPolicy selects app team-ai and its only ingress sources are app team-gateway, app team-search-indexer, app team-domain and app prometheus

  Scenario: A gRPC service with the value set probes liveness by name
    When charts/service is rendered for team-verification (grpcPort 50064) with --set probes.livenessService=liveness
    Then the container livenessProbe is a grpc probe with service liveness, and the readinessProbe and startupProbe have no service key

  Scenario: The default render matches the with-liveness golden
    When charts/service is rendered for team-verification with its service values and --set migrate.enabled=false
    Then the output is byte-identical to the golden render platform-gitops/tests/golden/team-verification.with-liveness.yaml and the liveness probe alone carries service liveness

  Scenario: An HTTP service ignores the value
    When charts/service is rendered for team-frontend (healthPath=/, no grpcPort) with --set probes.livenessService=liveness
    Then the container probes are httpGet probes and the rendered output contains no service: liveness

  Scenario: The canary Rollout renders the same liveness probe
    When charts/service is rendered for team-verification with --set rollout.enabled=true --set probes.livenessService=liveness
    Then the Rollout container livenessProbe carries service liveness

  Scenario: Every Go service renders a liveness probe by name
    When charts/service is rendered for each of the fourteen chart-based Go services against its baseline values
    Then each Deployment livenessProbe grpc service is liveness and each readinessProbe and startupProbe grpc has no service key

  Scenario: The gateway is probed over gRPC
    When charts/service is rendered for team-gateway against its baseline values
    Then the container has grpc probes on port 8080 and the output contains no httpGet probe and no healthPath

  Scenario: Every Go service renders its strict environment
    When each of the fourteen chart-based Go services is rendered against the staging overlay and the prod overlay
    Then every container has ENV "staging" and "production" respectively

  Scenario: A database service renders a PreSync migrate job
    When charts/service is rendered for team-order against the staging overlay
    Then exactly one migrate Job team-order-migrate is rendered with hook PreSync and delete policy BeforeHookCreation, running the Deployment image with args migrate up and ENV staging

  Scenario: The strategy value renders a Recreate Deployment
    When charts/service is rendered for team-order with --set strategy.type=Recreate
    Then the Deployment strategy type is Recreate and the default render of team-order carries no strategy

  Scenario: The migrate Job is absent when disabled
    When charts/service is rendered for team-verification with --set migrate.enabled=true and again with --set migrate.enabled=false
    Then the first render contains one Job and the second contains none

  Scenario: No deployed overlay enables reflection or mock data
    When every file under platform-gitops/envs/staging and envs/prod and every shared baseline is scanned
    Then none of them names GRPC_REFLECTION_ENABLED or MOCK_DATA
