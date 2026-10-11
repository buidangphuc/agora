@recsys @recommendations @batch
Feature: The recsys job trains and ships a GBDT ranker (recsys-gbdt-trainer)
  The real platform-recsys job image runs against the stack in an isolated namespace (a dedicated Redis DB and
  Qdrant collections, see rgt_job_flow) over a ranking dataset of 300 impression lists whose clicks depend on item
  price, with featurestore snapshots older than every impression. Needs the rebuilt platform-recsys image.

  Scenario: The trainer learns from clicks and beats the fixed-weight baseline
    When the recsys job runs with the GBDT stage enabled over a ranking dataset whose clicks depend on item price
    Then the run summary reports the GBDT candidate as promoted with an ndcg@10 greater than the baseline_ndcg@10 of the fixed-weight ranker

  Scenario: The model's feature list is the serving contract
    When the GBDT stage publishes its artifact
    Then the artifact lists the features item_popularity.views_7d, item_popularity.clicks_7d, item_popularity.add_to_cart_7d, item_popularity.favorites_current, item_popularity.review_count, item_popularity.avg_rating, item_popularity.ctr_7d and item_attributes.price, in that order, with the view versions

  Scenario: Each training row records where its CTR came from
    When the GBDT stage runs with a path set for the training rows
    Then every row records a ctr_source of debiased or fallback, both occur, and the model's metadata counts each

  Scenario: Evaluation uses a temporal holdout
    When the GBDT stage evaluates its candidate
    Then the model's metadata records an evaluation protocol, a cutoff, and that every held-out impression is later than every training impression

  Scenario: A missing ranking dataset stops the run
    When the recsys job runs with the GBDT stage enabled and no ranking dataset
    Then the job exits 2, its log names RANK_DATASET_DIR, and no model is registered

  Scenario: A disabled trainer leaves the run unchanged
    When the recsys job runs without enabling the GBDT stage
    Then the summary has no GBDT entry and the generation has no ranker artifact

  Scenario: The published artifact scores items the way the trainer does
    When an independent evaluator reads the artifact from recs:v1:gen:<generation>:ranker and scores a cheap and an expensive item
    Then its scores equal the trainer's to 1e-9 and the cheap item scores higher

  Scenario: A model below the promotion threshold is rejected and the generation ships without a ranker
    When the recsys job runs with the GBDT stage enabled and a promotion improvement threshold no model can reach
    Then the GBDT candidate is recorded as rejected with its reason, the generation is serving, and no ranker artifact exists for it

  Scenario: The ranker artifact lives and dies with its generation
    When the recsys job promotes three generations in a row with the GBDT stage enabled
    Then a ranker artifact with a TTL exists for the serving and previous generations and none for the oldest
