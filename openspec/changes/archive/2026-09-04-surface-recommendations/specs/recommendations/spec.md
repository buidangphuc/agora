## ADDED Requirements

### Requirement: Gateway forwards Recommend to team-ai without business logic

The system SHALL expose `platform.recommendation.v1.RecommendationService/Recommend` at the
gateway as a read-only forwarder to team-ai's gRPC service, verifying auth once and forwarding
`x-principal-{id,type,scopes}` downstream, holding no retrieval, ranking, or filtering logic
itself (architecture Rules 1–2). The frontend SHALL reach recommendations only through this
gateway path, never by calling team-ai directly.

#### Scenario: Recommend routes through the gateway to team-ai

- **WHEN** the frontend calls `Recommend` through the gateway with the caller's session
- **THEN** the gateway forwards the request to team-ai over gRPC with the forwarded
  `x-principal-*` metadata and returns team-ai's product list unchanged (no gateway-side business
  logic)

### Requirement: A "Gợi ý cho bạn" recommendations row is shown to buyers

The system SHALL render a **"Gợi ý cho bạn"** recommendations row, on the home page and/or the
product-detail page, populated from `team-ai` (`RecommendationService/Recommend`) via the gateway
using the caller's session, displaying up to ten product cards. On the product-detail page the
row SHALL be seeded with the current listing id; the browser SHALL never call team-ai directly.

#### Scenario: Logged-in buyer sees a recommendations row sourced from team-ai

- **WHEN** a logged-in buyer opens the page carrying the recommendations row
- **THEN** the "Gợi ý cho bạn" row is populated with product cards sourced from team-ai via the
  gateway (not a client-side mock or hardcoded list)

#### Scenario: Recommendations are unavailable without breaking the page

- **WHEN** the recommendation service returns `UNAVAILABLE` (e.g. `RECS_ENABLED=false`)
- **THEN** the page still renders and the "Gợi ý cho bạn" row is hidden or empty rather than
  erroring the whole page
