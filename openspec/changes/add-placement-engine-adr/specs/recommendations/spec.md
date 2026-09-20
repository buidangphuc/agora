## ADDED Requirements

### Requirement: Recommendation placement engine architecture specification

The system architecture SHALL support placement-specific recommendation configurations with a 4-tier fallback ladder (Personalized ML $\rightarrow$ Contextual/Vector $\rightarrow$ Category Popular $\rightarrow$ Global Popularity Floor) and diagnostic attribution metadata.

#### Scenario: Fallback ladder degrades on sparse candidate retrieval

- **WHEN** personalized candidate retrieval yields fewer than the requested number of items
- **THEN** the placement engine progressively retrieves fallback candidates from lower tiers until the quota is satisfied, stamping the `fallback_tier` in the response metadata
