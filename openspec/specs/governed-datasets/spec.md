# governed-datasets Specification

## Purpose
Defines the governed, point-in-time training dataset (als_interactions) built by platform-featurestore, its manifest,
and the rule that recsys trains only from it and records the dataset lineage on every model.

## Requirements

### Requirement: The ALS training dataset is built point-in-time from governed inputs

`python -m featurestore dataset` SHALL build `als_interactions@v1` as of `AS_OF` (default now). It SHALL use only rows with
`ingested_at` at or before `AS_OF` that occurred in the `DATASET_WINDOW_DAYS` before it, keyed by the stitched `user_key`.
Each (user_key, listing_id) row SHALL have:
- a `weight`, which is the sum of:
  - the tracking event weights (impression 0.5, view 1, click 2, view_cart 2.5, add_to_cart 5, add_shipping_info 6,
    add_payment_info 7, begin_checkout 8, purchase 10, any other type 0);
  - +3 if the pair is a current favourite as of `AS_OF`;
  - +2 for a review rated 4 or 5;
  - −2 for a review rated 1 or 2;
- the number of contributing interactions;
- the last occurrence time.

Rows with a weight of 0 or less SHALL be dropped.

#### Scenario: A buyer's views and favourite become one weighted row

- **WHEN** a new buyer views a listing twice and favourites it, the export cycle completes, and the dataset is built
- **THEN** the dataset has one row for that buyer and listing, with weight 5 and 3 interactions

#### Scenario: Pre-login views count toward the buyer

- **WHEN** a visitor views a listing anonymously, logs in as a new buyer and views it again with the same anonymous id,
  the export cycle completes, and the dataset is built
- **THEN** the dataset has one row for that buyer and listing, with weight 2

#### Scenario: Events after AS_OF are not in the dataset

- **WHEN** the dataset is built with `AS_OF` just before a new buyer's first event
- **THEN** the dataset has no row for that buyer

### Requirement: Each dataset build writes a manifest

Each build SHALL write the dataset under `<offline dir>/datasets/als_interactions/v1/as_of=<as_of>.parquet` and a
`manifest.json` beside it. The manifest SHALL record:
- `as_of` and the window in days;
- the row, user and item counts;
- the SHA-256 of the definition;
- the input watermark;
- the SHA-256 of the dataset file.

#### Scenario: A dataset manifest describes its file

- **WHEN** the dataset is built
- **THEN** its manifest names the `as_of`, a positive row count, and a file SHA-256 equal to the SHA-256 of the dataset
  file

### Requirement: Recsys trains only from a governed dataset and records its lineage

The recsys ALS job SHALL read its interactions only from a governed dataset: `DATASET_PATH`, or the latest snapshot
under `DATASET_DIR`. It SHALL use the dataset's `weight` as given. It SHALL exit non-zero, naming the dataset setting,
when no dataset exists. It SHALL never read raw tracking events. Every model it registers SHALL record in
`parameters.dataset` the dataset name, its version, its `as_of` and its file SHA-256.

#### Scenario: A trained model names its dataset

- **WHEN** the dataset is built and the recsys ALS job runs on it
- **THEN** the model the job registered records `als_interactions`, version 1, the dataset's `as_of` and the file SHA-256
  from its manifest

#### Scenario: Recsys refuses to train without a dataset

- **WHEN** the recsys ALS job starts with `DATASET_DIR` pointing at an empty directory
- **THEN** it exits non-zero, its log names `DATASET_DIR`, and no model is registered
