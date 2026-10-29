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
    When charts/service is rendered for team-gateway (healthPath=/healthz, no grpcPort) with --set probes.livenessService=liveness
    Then the container probes are httpGet probes and the rendered output contains no service: liveness

  Scenario: The canary Rollout renders the same liveness probe
    When charts/service is rendered for team-verification with --set rollout.enabled=true --set probes.livenessService=liveness
    Then the Rollout container livenessProbe carries service liveness
