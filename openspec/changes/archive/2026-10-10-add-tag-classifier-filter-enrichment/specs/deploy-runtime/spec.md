## RENAMED Requirements

- FROM: `### Requirement: team-ai accepts traffic only from the gateway and Prometheus`
- TO: `### Requirement: team-ai accepts traffic only from its gRPC callers and Prometheus`

## MODIFIED Requirements

### Requirement: team-ai accepts traffic only from its gRPC callers and Prometheus

The team-ai deployment SHALL ship an ingress NetworkPolicy that admits only these pods:
- `app: team-gateway`, the edge caller;
- `app: team-search-indexer`, the caller of the internal `AIService.ClassifyTags` (scope `ai.classify`);
- `app: prometheus`, the KEDA scaling metric source.

#### Scenario: team-ai renders an ingress allow-list

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** a `NetworkPolicy` selects `app: team-ai` and its only ingress sources are `app: team-gateway`,
  `app: team-search-indexer` and `app: prometheus`
