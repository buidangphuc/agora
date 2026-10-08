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
    Then a NetworkPolicy selects app team-ai and its only ingress sources are app team-gateway and app prometheus
