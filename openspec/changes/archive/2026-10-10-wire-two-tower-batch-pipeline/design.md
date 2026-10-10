## Context

The stage built a catalogue of constants (`price=100`, `popularity=1`, `category="general"`), never trained, and wrote one
plain collection with no generation scoping, so every vector was identical and zero. Reconciled with the archived
`featurestore-*` and `recsys-generation-publish` changes.

## Decisions

### D1. Governed inputs only
The job already trains only on the governed dataset. The stage reads the featurestore's offline snapshots the same way
(mounted read-only at `/features`): `item_popularity@v1` (`/features/item_popularity/v1/as_of=*.parquet`, entity `listing_id`)
and `user_activity@v2` (`.../user_activity/v2/`, entity `user_key`), latest `as_of` or an explicit
`ITEM_FEATURES_PATH`/`USER_FEATURES_PATH`. Missing snapshot: `ConfigError`, exit 2 before Spark. The snapshot's sha256 and
file name go to `parameters.two_tower.features`.

Mappings (`recsys/two_tower/features.py`, fixed scales so training and any later query agree): item
`historical_ctr = ctr_7d`, `popularity_score = log1p(views + 2 clicks + 5 add_to_cart + 3 favourites) / log1p(500)` capped at 1;
user `lifetime_purchases = paid_orders_30d`, `activity_score = log1p(views + clicks + add_to_cart) / log1p(200)` capped at 1.

**Open dependency.** `item_popularity` and `user_activity` carry no category, price, preferred categories or average order
value, so those tower inputs are 0 until the featurestore adds an attribute view (follow-up change, featurestore side).
The tower and its tests already handle them; the task wording "different categories" is met at the tower level and, in the
pipeline, as "different recorded features".

### D2. Catalogue = the item snapshot
Every snapshot row is a catalogue item, so an item favourited or reviewed but absent from the 30-day dataset (no ALS factor)
still gets a vector.

### D3. Training
`recsys/two_tower/train.py`: numpy, in-batch softmax over (user, item) pairs from the dataset (sampled to
`TWO_TOWER_MAX_PAIRS`, seeded), exact gradients through ReLU and L2 normalisation, SGD (`TWO_TOWER_EPOCHS`, `_LR`,
`_BATCH_SIZE`, `_TEMPERATURE`). Deterministic. Epochs 0 skips training.

### D4. Generation naming, written before the switch
`item_two_tower_vectors__<model_version>`, written by `publish_generation` with the ALS collections before the pointer
moves; retention deletes it with the generation (serving and previous are kept) and also removes the old plain
`item_two_tower_vectors`. No alias: it is new, so no reader predates pointer-resolved names; a consumer names it from
`recs:v1:serving`. Rollback needs no change: the previous generation's collection is kept with it.

### D5. Guard and failure policy
A zero or non-finite vector is refused (counted in `two_tower.refused`, WARNING); if none remains the stage raises
`DegenerateEmbeddingError`. The stage runs after the promotion decision and before publish; any stage error rejects the
candidate (`gate_reason` = "two-tower stage failed: ...") and re-raises, exactly like a failed publish, so serving never
changes. Chosen over "publish ALS anyway" so a generation is complete or invisible and failures are loud (exit 1).

## Risks
- Without category/price the learned space is driven by popularity and CTR: retrieval is a popularity-shaped baseline until
  the featurestore ships attributes. The pipeline, lineage, guard and generation handling do not change then.
