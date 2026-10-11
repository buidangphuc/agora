## Why

`add-recsys-nearline-signals` (P2-T4) delivered `NearlineSignalStore.get_debiased_ctr`, which
applies inverse-propensity weighting to impression/click streams so an item is not rewarded
merely for having been shown at position 1. Grep shows its only callers are
`tests/test_nearline.py` and an error-log line: **no production code reads it.**

Meanwhile `extract_candidate_features` fills `historical_ctr` from a static value. The ranker is
therefore trained and scored on raw, position-biased CTR while a debiased figure sits unused one
module away. This is the most consequential of the remaining dead wires: it does not merely omit
a signal, it feeds the ranker a **biased** one.

## Depends on

`wire-serving-gbdt-featurestore` — the enrichment path this change writes into is created there.
These two changes edit the same candidate-enrichment code and MUST NOT be run in parallel.

## What Changes

- **platform-recsys** (`recsys/ranker/features.py`):
  - `extract_candidate_features` accepts a nearline signal source and uses
    `get_debiased_ctr(listing_id)` for `historical_ctr` when a value is available.
  - Absent or insufficient nearline data falls back to the existing value; the feature vector
    records which source was used so training and serving can be compared.
- **team-ai**: the serving enrichment path supplies the nearline source alongside feature-store
  features.

## Non-goals

- No change to the IPS weighting formula itself.
- No backfill of historical training data with debiased values.
