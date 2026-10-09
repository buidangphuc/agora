@analytics @batch @recsys
Feature: The ALS training dataset is governed, point in time, and traceable from the model
  `python -m featurestore dataset` builds als_interactions@v1 from the Parquet exports of
  team-analytics as of AS_OF, into <offline dir>/datasets/als_interactions/v1 with a manifest. The
  recsys ALS job trains only from such a dataset and records it in the model's parameters.dataset.
  Both jobs run here as the real images on the stack network.
  (featurestore-datasets / governed-datasets)

  The scenarios share one batch of tracked activity (one seller with a listing, one buyer who views
  and favourites it, and one visitor who views anonymously and then logs in), created by the first
  scenario that runs, so the 300 s export cycle is waited out once.

  Scenario: A buyer's views and favourite become one weighted row
    When a new buyer "b1" views "L1" twice and favourites it, the export cycle completes, and the dataset is built
    Then the dataset has one row for "b1" and "L1" with weight 5 and 3 interactions

  Scenario: Pre-login views count toward the buyer
    When a visitor views "L2" anonymously, logs in as a new buyer "b2" and views it again with the same anonymous id, the export cycle completes, and the dataset is built
    Then the dataset has one row for "b2" and "L2" with weight 2

  Scenario: Events after AS_OF are not in the dataset
    Given a new buyer "b1" has viewed "L1" twice and favourited it, and the export cycle has completed
    When the dataset is built with AS_OF just before "b1"'s first event
    Then the dataset has no row for "b1"

  Scenario: A dataset manifest describes its file
    Given a new buyer "b1" has viewed "L1" twice and favourited it, and the export cycle has completed
    When the dataset is built
    Then its manifest names the as_of, a positive row count, and a file SHA-256 equal to the SHA-256 of the dataset file

  Scenario: A trained model names its dataset
    Given a new buyer "b1" has viewed "L1" twice and favourited it, and the export cycle has completed
    When the dataset is built and the recsys ALS job runs on it
    Then the model the job registered records als_interactions, version 1, the dataset's as_of and the file SHA-256 from its manifest

  Scenario: Recsys refuses to train without a dataset
    When the recsys ALS job starts with DATASET_DIR pointing at an empty directory
    Then it exits non-zero, its log names DATASET_DIR, and no model is registered
