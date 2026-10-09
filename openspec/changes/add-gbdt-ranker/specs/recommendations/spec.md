## ADDED Requirements

### Requirement: GBDT candidate re-ranking stage

The system SHALL provide a GBDT re-ranking model in `platform-recsys/recsys/ranker/` that scores candidate items using multi-signal feature vectors (similarity score, popularity weight, category affinity match, price affinity) and achieves higher NDCG@10 than raw cosine sorting.

#### Scenario: GBDT ranker outperforms raw cosine baseline on offline eval

- **WHEN** candidates are ranked by the GBDT model versus raw similarity cosine score on the evaluation holdout dataset
- **THEN** the GBDT ranker achieves a higher `ndcg@10` than the baseline cosine sorting

- **VERIFIED BY**: platform-recsys/tests/test_ranker.py › test_gbdt_ranker_outperforms_raw_cosine_ndcg. Not verifiable end to end: the NDCG comparison is offline arithmetic over a labelled holdout; no deployed surface takes a holdout and returns both rankings, and the serving path (team-ai) exposes only the re-ranked list, never the raw-cosine baseline.
