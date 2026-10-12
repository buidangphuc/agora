## ADDED Requirements

### Requirement: Training interactions exclude events without personalisation consent

The governed `als_interactions` and `rank_training` datasets SHALL include only tracking events whose `consent_personalization`
is true. An event with `consent_version` 0, or without a consent value, SHALL be treated as denied. For a `user_key` whose
history contains a denied event followed by a granted one, only events after the latest event with `personalization = false` SHALL be used.

#### Scenario: Denied buyer has no interaction rows

- **WHEN** buyer `u-deny` has 10 view events all with `consent_personalization=false` and buyer `u-ok` has 3 with true, and the datasets are built
- **THEN** `als_interactions` has 0 rows for `u-deny` and at least 1 row for `u-ok`

#### Scenario: Only post-grant events are used after a later grant

- **WHEN** buyer `u-late` has 4 denied events at T1 and 2 granted events at T2 > T1
- **THEN** the dataset row for `u-late` counts `interactions = 2` and its `last_occurred_at` is at T2

#### Scenario: Missing consent is treated as denied

- **WHEN** an event row has `consent_version = 0` or a null `consent_personalization`
- **THEN** that event contributes 0 to any dataset row

#### Scenario: Denied buyer is served popular recommendations

- **WHEN** a signed-in buyer with no granted events calls `Recommend` for the home feed through the gateway
- **THEN** the response lists at least 1 listing id, served by the existing popular path

### Requirement: Nearline signals drop events without personalisation consent

`python -m recsys.nearline` SHALL ignore, before any Redis write, a tracking event whose `consent.personalization` is false or
whose `consent.version` is 0. It SHALL count each ignored event in a `consent_denied` counter and SHALL commit its offset like
any applied event. Denied events SHALL NOT update any per-user key or any item-level CTR counter. A message that cannot be decoded SHALL be skipped,
counted in a `decode_errors` counter, and its offset committed. A denied event SHALL be dropped even when it arrives after a grant
event of the same actor, so per-user keys hold only events after the latest denied event.

#### Scenario: Denied event changes no per-user key

- **WHEN** a view event with `consent.personalization=false` is consumed for actor `u-deny`
- **THEN** `recs:nearline:user:u-deny:items`, `recs:nearline:user:u-deny:cats` and the session and co-view keys are unchanged
- **AND** `consent_denied` increases by 1

#### Scenario: Granted event is applied

- **WHEN** a view event with `consent.personalization=true` is consumed for actor `u-ok`
- **THEN** `recs:nearline:user:u-ok:items` lists the viewed listing at rank 1

#### Scenario: Event without a consent field is denied

- **WHEN** a tracking event encoded without field 26 is consumed
- **THEN** it is counted as `consent_denied` and no per-user key changes

#### Scenario: Denied event updates no item CTR counter

- **WHEN** an impression and a click with `consent.personalization=false` are consumed for listing `L1`
- **THEN** every item-level CTR counter key for `L1` is unchanged

#### Scenario: Denied event arriving after a grant is dropped

- **WHEN** actor `u-late` sends a granted view at T2, then a denied view with occurred time T3 > T2 is consumed
- **THEN** the denied event's listing is absent from every `recs:nearline:user:u-late:*` key and `consent_denied` increases by 1

#### Scenario: Corrupt message is skipped and committed

- **WHEN** a message that is not a decodable TrackingEvent is consumed
- **THEN** no Redis key changes, `decode_errors` increases by 1, its offset is committed and the next message is processed

#### Scenario: Grant after deny uses only later events

- **WHEN** actor `u-late` sends a denied view at T1 and a granted view at T2 > T1
- **THEN** `recs:nearline:user:u-late:items` holds only the listing of the T2 event

### Requirement: Nearline session signals include cart and checkout events

The nearline consumer SHALL dedupe `view_cart` and `begin_checkout` by event id and SHALL treat `view_cart` (EventType 11) and `begin_checkout` (EventType 6) of consented actors as engagement
events: they SHALL update the actor's recents and category affinity with weights 3.0 for `view_cart` and 6.0 for
`begin_checkout`. Other behaviour of existing events SHALL NOT change.

#### Scenario: Checkout events raise category affinity

- **WHEN** a consented actor sends `view_cart` then `begin_checkout` for a listing of category `books`
- **THEN** `recs:nearline:user:<actor>:cats` field `books` increases by 9.0 (3.0 + 6.0)

#### Scenario: Cart event without a listing is skipped

- **WHEN** a `view_cart` event has an empty `listing_id`
- **THEN** it changes no key and the consumer keeps running

#### Scenario: Redelivery of a checkout event is applied once

- **WHEN** the same `begin_checkout` event id is delivered twice, at any interval
- **THEN** the category affinity increases by 6.0 once

### Requirement: Ranker training debiases position using list id and position

Ranker training SHALL weight each impression-labelled row by the inverse propensity of its (`item_list_id`, `position`) pair
when both columns are present, and SHALL fall back to the existing position-only handling when `item_list_id` is absent.
Rows with `position` < 1 SHALL be treated as position 1.

#### Scenario: Same CTR at worse position gets higher weight

- **WHEN** two items have equal clicks and impressions in the same `item_list_id`, one always at position 8 and one at position 1
- **THEN** the position-8 item's debiased weight is higher than the position-1 item's

#### Scenario: Missing item_list_id keeps position-only behaviour

- **WHEN** the input Parquet has no `item_list_id` column
- **THEN** training completes and the weights equal those of the previous position-only method

#### Scenario: Non-positive position is clamped

- **WHEN** a row has `position = 0` or -3
- **THEN** it is weighted as position 1 and training does not fail

### Requirement: Session acquisition context is an evaluated ranker training feature

The ranker MAY use `session_source`, `session_medium` and `session_campaign` as TRAINING features only. There SHALL be no
request-time campaign boost and `RecommendRequest` SHALL NOT change. The training run SHALL evaluate the model with and without
them on the same time-based split and SHALL enable them only when the with-context `ndcg@k` and `recall@k` are both not lower than
the baseline. The decision and both metric sets SHALL be recorded in the run report. Columns absent from the input SHALL disable
the feature without failing the run.

#### Scenario: Context kept when it does not regress

- **WHEN** the with-context metrics are equal to or above the baseline on both `ndcg@k` and `recall@k`
- **THEN** the run report states `context_features=enabled` with both metric sets

#### Scenario: Context dropped on regression

- **WHEN** the with-context `ndcg@k` or `recall@k` is lower than the baseline
- **THEN** the run report states `context_features=disabled` and the published model uses no context features

#### Scenario: Legacy input without context columns

- **WHEN** the input Parquet has no `session_source`, `session_medium` or `session_campaign` column
- **THEN** the run completes with `context_features=disabled` and reason `columns_absent`

### Requirement: Exported tracking Parquet carries consent and context columns

The tracking Parquet export (team-analytics `internal/export/export.go`) SHALL add `consent_personalization`, `consent_version`,
`item_list_id`, `position`, `session_source`, `session_medium` and `session_campaign`. This slice owns the join of `ga4_sessions`
(built by `add-ga4-export-import`) on `ga_client_id` that fills the session columns; they SHALL be empty when no session matches.
The export SHALL remain atomic, and a failed export SHALL keep the previous file.

#### Scenario: Exported file has consent and context columns

- **WHEN** an export runs over events with consent stamped and a matching `ga4_sessions` row
- **THEN** the Parquet contains the seven columns and the matched row has the session's source, medium and campaign

#### Scenario: Unmatched event has empty session context

- **WHEN** an event has an empty `ga_client_id` or no matching session
- **THEN** its three session columns are empty strings and the row is still exported

#### Scenario: Failed export keeps the previous file

- **WHEN** an export fails while a previous file exists
- **THEN** the previous file is byte-identical and the error is logged

#### Scenario: Export never reads GA4 as an interaction source

- **WHEN** only `ga4_sessions` rows exist and no first-party events
- **THEN** the exported file has 0 rows
