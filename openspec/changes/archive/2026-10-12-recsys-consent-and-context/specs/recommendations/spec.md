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
- **VERIFIED BY**: `platform-featurestore/tests/test_consent_filter.py` "test_missing_consent_is_denied" (Clarifications R1 Q1)

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
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_granted_event_is_applied" (Clarifications R1 Q1)

#### Scenario: Event without a consent field is denied

- **WHEN** a tracking event encoded without field 26 is consumed
- **THEN** it is counted as `consent_denied` and no per-user key changes
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_event_without_consent_field_is_denied" (Clarifications R1 Q1)

#### Scenario: Denied event updates no item CTR counter

- **WHEN** an impression and a click with `consent.personalization=false` are consumed for listing `L1`
- **THEN** every item-level CTR counter key for `L1` is unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_denied_event_updates_no_ctr_counter" (Clarifications R1 Q1)

#### Scenario: Denied event arriving after a grant is dropped

- **WHEN** actor `u-late` sends a granted view at T2, then a denied view with occurred time T3 > T2 is consumed
- **THEN** the denied event's listing is absent from every `recs:nearline:user:u-late:*` key and `consent_denied` increases by 1
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_denied_after_grant_is_dropped" (Clarifications R1 Q1)

#### Scenario: Corrupt message is skipped and committed

- **WHEN** a message that is not a decodable TrackingEvent is consumed
- **THEN** no Redis key changes, `decode_errors` increases by 1, its offset is committed and the next message is processed
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_corrupt_message_skipped_and_committed" (Clarifications R1 Q1)

#### Scenario: Grant after deny uses only later events

- **WHEN** actor `u-late` sends a denied view at T1 and a granted view at T2 > T1
- **THEN** `recs:nearline:user:u-late:items` holds only the listing of the T2 event
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_consent.py` "test_grant_after_deny_uses_only_later_events" (Clarifications R1 Q1)

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
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_checkout.py` "test_cart_event_without_listing_is_skipped" (Clarifications R1 Q1)

#### Scenario: Redelivery of a checkout event is applied once

- **WHEN** the same `begin_checkout` event id is delivered twice, at any interval
- **THEN** the category affinity increases by 6.0 once
- **VERIFIED BY**: `platform-recsys/tests/test_nearline_checkout.py` "test_checkout_redelivery_applied_once" (Clarifications R1 Q1)

### Requirement: Ranker training debiases position using list id and position

Ranker training SHALL weight each impression-labelled row by the inverse propensity of its (`item_list_id`, `position`) pair
when both columns are present, and SHALL fall back to the existing position-only handling when `item_list_id` is absent.
Rows with `position` < 1 SHALL be treated as position 1. The propensity of (`item_list_id`, `position`) SHALL be the
empirical click-through rate at that position divided by the rate at position 1 in the same list, each smoothed as
(clicks + 1) / (impressions + 2) and the ratio clamped to [0.05, 1]; when either cell has fewer than 100 impressions the
row SHALL use the position-only weight instead (Clarifications R2 Q5).

#### Scenario: Same CTR at worse position gets higher weight

- **WHEN** two items have equal clicks and impressions in the same `item_list_id`, one always at position 8 and one at position 1
- **THEN** the position-8 item's debiased weight is higher than the position-1 item's
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_debias.py` "test_same_ctr_worse_position_higher_weight" (Clarifications R1 Q1)

#### Scenario: List-specific propensity replaces the position-only weight

- **WHEN** list `L` has at least 100 impressions at positions 1 and 4 with smoothed CTR 0.20 at position 1 and 0.05 at position 4
- **THEN** a position-4 row of `L` is weighted 1 / (0.05 / 0.20) = 4.0, not the position-only weight
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_debias.py` "test_list_propensity_replaces_position_only_weight" (Clarifications R2 Q5)

#### Scenario: Sparse list cell falls back to position-only

- **WHEN** a (`item_list_id`, `position`) cell or its position-1 cell has fewer than 100 impressions
- **THEN** rows of that cell get the position-only weight
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_debias.py` "test_sparse_list_cell_falls_back_to_position_only" (Clarifications R2 Q5)

#### Scenario: Missing item_list_id keeps position-only behaviour

- **WHEN** the input Parquet has no `item_list_id` column
- **THEN** training completes and the weights equal those of the previous position-only method
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_debias.py` "test_missing_item_list_id_keeps_position_only" (Clarifications R1 Q1)

#### Scenario: Non-positive position is clamped

- **WHEN** a row has `position = 0` or -3
- **THEN** it is weighted as position 1 and training does not fail
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_debias.py` "test_non_positive_position_is_clamped" (Clarifications R1 Q1)

### Requirement: Session acquisition context is an evaluated ranker training feature

The ranker MAY use `session_source`, `session_medium` and `session_campaign` as TRAINING features only. There SHALL be no
request-time campaign boost and `RecommendRequest` SHALL NOT change. The training run SHALL evaluate the model with and without
them on the same time-based split and SHALL enable them only when the with-context `ndcg@k` and `recall@k` are both not lower than
the baseline. The decision and both metric sets SHALL be recorded in the run report. Columns absent from the input SHALL disable
the feature without failing the run. Until the serving ranker (team-ai) accepts a feature list other than
`RANKING_FEATURES`, the published artifact SHALL keep exactly `RANKING_FEATURES`; `context_features=enabled` records that
the context variant qualified, not that it was published (Clarifications R3 Q6).

#### Scenario: Context kept when it does not regress

- **WHEN** the with-context metrics are equal to or above the baseline on both `ndcg@k` and `recall@k`
- **THEN** the run report states `context_features=enabled` with both metric sets and the published artifact's features equal `RANKING_FEATURES`
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_context.py` "test_context_kept_without_regression" (Clarifications R1 Q1)

#### Scenario: Context dropped on regression

- **WHEN** the with-context `ndcg@k` or `recall@k` is lower than the baseline
- **THEN** the run report states `context_features=disabled` and the published model uses no context features
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_context.py` "test_context_dropped_on_regression" (Clarifications R1 Q1)

#### Scenario: Legacy input without context columns

- **WHEN** the input Parquet has no `session_source`, `session_medium` or `session_campaign` column
- **THEN** the run completes with `context_features=disabled` and reason `columns_absent`
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_context.py` "test_legacy_input_without_context_columns" (Clarifications R1 Q1)

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
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` "TestExport_UnmatchedEventHasEmptySessionContext" (Clarifications R1 Q1)

#### Scenario: Failed export keeps the previous file

- **WHEN** an export fails while a previous file exists
- **THEN** the previous file is byte-identical and the error is logged
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` "TestExport_FailedExportKeepsPreviousFile" (Clarifications R1 Q1)

#### Scenario: Export never reads GA4 as an interaction source

- **WHEN** only `ga4_sessions` rows exist and no first-party events
- **THEN** the exported file has 0 rows
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` "TestExport_NeverReadsGA4AsInteractionSource" (Clarifications R1 Q1)
