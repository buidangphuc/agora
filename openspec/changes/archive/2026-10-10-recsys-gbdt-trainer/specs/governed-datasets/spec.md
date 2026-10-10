## ADDED Requirements

### Requirement: The ranking training dataset is built point-in-time from impressions

`python -m featurestore dataset` SHALL also build `rank_training@v1`: one row per (`impression_id`, `listing_id`) of an
`impression` event in the `DATASET_WINDOW_DAYS` before `AS_OF`, with the user's `user_key`, the impression's `position`, its
`occurred_at`, and a `label`: 2 if an `add_to_cart` event carrying the same `impression_id` and `listing_id` occurred at or
after the impression, else 1 if a `click` did, else 0. Impressions without an `impression_id` or a `user_key` are not rows.
Only events up to `AS_OF` count, and the manifest and file follow the other datasets (`rank_training/v1/as_of=<stamp>`).

#### Scenario: A clicked impression becomes a positive training row

- **WHEN** a buyer is shown listings in one recommendation row and clicks one of them with the same impressionId, the export cycle completes, and the dataset job runs
- **THEN** rank_training@v1 has a row for that buyer, impression and listing with label 1

#### Scenario: An add-to-cart is a stronger positive than a click

- **WHEN** the buyer adds another listing of that row to the cart with the same impressionId, the export cycle completes, and the dataset job runs
- **THEN** rank_training@v1 has a row for that listing with label 2

#### Scenario: An impression without a click is a negative row

- **WHEN** a third listing of that row is never clicked
- **THEN** rank_training@v1 has a row for it with label 0 and the row's position

#### Scenario: Impressions after AS_OF are not in the ranking dataset

- **WHEN** the events hold one impression before AS_OF and one after it, and the dataset job runs with that AS_OF
- **THEN** rank_training@v1 has a row for the first and none for the second
