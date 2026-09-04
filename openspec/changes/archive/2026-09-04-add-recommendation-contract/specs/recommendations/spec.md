## ADDED Requirements

### Requirement: A recommendation serving contract exists in platform-core

The platform SHALL define a `platform.recommendation.v1.RecommendationService` gRPC service in
`platform-core/packages/proto` with a single `Recommend(RecommendRequest) returns (RecommendResponse)`
RPC, additively (a new package/file) so no existing contract is renumbered, removed, or otherwise
broken. The contract SHALL follow the platform's proto conventions (buf v2, STANDARD lint with the
enum-value-prefix rule, reuse of `platform.common.v1`) and SHALL be the single source of truth for
the online recommendation RPC (Rule 4).

#### Scenario: The recommendation proto lints and stays non-breaking

- **WHEN** `buf lint` and `buf breaking` run over `packages/proto` after adding
  `platform/recommendation/v1/recommendation.proto`
- **THEN** lint passes (STANDARD rules, enum values prefixed `RECOMMENDATION_CONTEXT_*`) and the
  breaking check passes because the change only adds a new package and touches no existing message,
  field number, or RPC

### Requirement: Recommend request carries caller identity, an optional seed, context, and a limit

`RecommendRequest` SHALL let a caller ask for recommendations as either an authenticated user
(`user_id`) or an anonymous visitor (`anonymous_id`), SHALL allow an optional `seed_listing_id` to
anchor item-to-item contexts (PDP "similar items", cart), SHALL carry a `RecommendationContext`
enum identifying where the recommendation is shown, and SHALL accept a `limit` (0 = server default).

#### Scenario: An anonymous PDP "similar items" request is expressible

- **WHEN** a caller builds a `RecommendRequest` with an empty `user_id`, an `anonymous_id`, a
  `seed_listing_id` set to the viewed listing, `context = RECOMMENDATION_CONTEXT_SIMILAR_ITEMS`, and
  `limit = 12`
- **THEN** the message is valid under the contract, so the serving layer has every field it needs to
  choose an item-item strategy for an anonymous visitor without a schema change

### Requirement: Recommend response returns ranked listing ids with scores and a model version

`RecommendResponse` SHALL return a `repeated RecommendedItem`, each carrying a `listing_id`, a
`score` (higher = more relevant), and a 1-based `rank`, ordered best-first, plus a `model_version`
identifying the offline artifact that produced the ranking. The response SHALL carry **listing ids
and scores only** (not hydrated listing cards), so the recommendation side never owns listing
content and the serving layer hydrates cards from the owning services (Rule 3).

#### Scenario: A ranked result set is expressible with provenance

- **WHEN** the serving layer fills a `RecommendResponse` with ordered `RecommendedItem`s and the
  `model_version` of the batch artifact it read
- **THEN** the consumer can render the ranking in order using each `listing_id`/`rank`, sort/threshold
  on `score`, and trace which offline model produced the result via `model_version`
