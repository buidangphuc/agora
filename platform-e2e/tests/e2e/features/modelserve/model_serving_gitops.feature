@modelserve
Feature: Model serving GitOps manifests
  Static, no stack needed: the manifest platform-gitops/platform/infra/modelserve.yaml is parsed.
  Change: add-platform-modelserve, capability model-serving.

  Scenario: NetworkPolicy admits only team-ai, team-search and prometheus
    When the platform-modelserve GitOps manifest is parsed
    Then a NetworkPolicy selects app modelserve-router and its only ingress sources are app team-ai, app team-search, app team-search-indexer and app prometheus on port 8100
