# Proposal: add-two-tower-retrieval

## Summary
Implements a Two-Tower Deep Neural Network retrieval model in `platform-recsys` to generate dense vector embeddings for users and items based on rich entity features and behavioral context. This supplements ALS collaborative filtering by enabling candidate retrieval for new/cold-start users and items.

## Motivation
Pure matrix factorization (ALS) requires prior interaction history (user-item co-occurrence) and fails on cold items or users with zero historical transactions. Two-Tower models project both user context (category preferences, purchase power, activity) and item attributes (category, price, textual metadata) into a shared semantic latent space, allowing high-throughput approximate nearest neighbor (ANN) retrieval.

## Architecture
- `recsys.two_tower.user_tower.UserTower`: maps user profile features into dense embedding vector $\mathbf{u} \in \mathbb{R}^d$.
- `recsys.two_tower.item_tower.ItemTower`: maps item attributes into dense embedding vector $\mathbf{v} \in \mathbb{R}^d$.
- `recsys.two_tower.model.TwoTowerModel`: manages towers, vector indexing, forward scoring via dot-product / cosine similarity, and top-$K$ candidate retrieval.
- Evaluated via `recsys.evals.ModelEvaluator` on test sets.
