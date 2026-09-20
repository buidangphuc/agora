# Capability: Two-Tower Dense Vector Retrieval

## ADDED Requirements

### Requirement: Two-Tower Feature Projection
The system MUST support projecting user features and item features into a shared $D$-dimensional latent vector space.

#### Scenario: User tower and item tower embedding generation
- **GIVEN** user features (category preferences, activity, lifetime purchases) and item features (category, price, popularity)
- **WHEN** user tower and item tower compute embeddings
- **THEN** both vectors have dimension $D$ and normalized magnitude for cosine similarity calculation.

### Requirement: Candidate Retrieval via Nearest Neighbor
The system MUST retrieve top-$K$ item candidates for a user query vector via inner product / cosine similarity scoring against the item index.

#### Scenario: Top-K candidate generation for user
- **GIVEN** an item catalog indexed into candidate vectors
- **WHEN** top-$K$ retrieval is requested for a user vector
- **THEN** top-$K$ candidate item IDs are returned ranked by similarity score.

### Requirement: Cold-Start Item Retrieval
The system MUST be able to index and retrieve cold items with zero interaction history based solely on item feature representations.

#### Scenario: Cold item is retrievable by relevant user profile
- **GIVEN** a newly added item with category and price features but no interaction history
- **WHEN** item is projected into candidate index and user with matching category profile queries
- **THEN** new item is present in top candidate results.
