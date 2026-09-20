## ADDED Requirements

### Requirement: Placement-specific recommendation execution

The recommendation service in `team-ai` SHALL route requests by `placement_id` (`home_feed`, `similar_items`, `cart_cross_sell`) and execute the 4-tier fallback ladder when candidates are sparse or cold-start conditions occur.

#### Scenario: Home feed surfaces personalized recommendations with popularity fallback

- **WHEN** a user queries the `home_feed` placement
- **THEN** the service attempts personalized retrieval (Tier 1) and falls back to global popular items (Tier 4) if personalized candidates are unavailable, stamping `placement_id: "home_feed"` and `fallback_tier` in the result

#### Scenario: Similar items placement retrieves item similarities

- **WHEN** a client queries `similar_items` with `seed_listing_id`
- **THEN** the service retrieves similar items via vector similarity and falls back to category/global popular items if sparse
