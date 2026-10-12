# Proposal: Recsys honours personalisation consent and uses acquisition context

initiative: ga4-analytics

## Why

Buyers choose through the consent banner whether behaviour may personalise their recommendations (Decree 13/2023). The edge stamps
that choice on every tracking event (`TrackingEvent.consent`, from slice `ga4-consent-contract`), but recsys today learns from every
event. Recsys must (1) exclude events of buyers who denied personalisation, (2) stop using such events for nearline signals,
(3) keep serving them the existing popular path, and (4) use list position and acquisition context (`session_source`,
`session_medium`, `session_campaign`) as ranker TRAINING features only, keeping a context feature only if offline evaluation
(`ndcg@k` and `recall@k`) does not regress. There is no request-time campaign boost and `RecommendRequest` is unchanged. The first-party pipeline stays the only training source; GA4 is never read by recsys.

## What changes

- **team-analytics export** (`internal/export/export.go`, hot file shared with `add-ga4-export-import`): the exported
  tracking Parquet additionally carries `consent_personalization` (bool), `consent_version` (int), `item_list_id`, `position`,
  and `session_source` / `session_medium` / `session_campaign` (strings; empty when no GA client id or no session match).
  This slice owns adding the three session columns to the export: it joins `ga4_sessions` on `ga_client_id` inside `export.go`.
  `add-ga4-export-import` only builds the `ga4_sessions` table. `export.go` is a hot file: this slice is serialised after `add-ga4-export-import`.
- **platform-featurestore**: reads the new columns only when present; the governed `als_interactions` and `rank_training`
  datasets drop events with `consent_personalization = false` and `consent_version = 0` (no cookie = denied) and, for a buyer
  with a deny-then-grant history, keep only events after the latest event with `personalization = false`. The registry SQL changes, so
  new dataset versions are published: `als_interactions_v2` and `rank_training_v2` (new `registry/sql/*_v2.sql`, `features.yaml` and
  `features.lock` refreshed); the v1 files stay for rollback. `begin_checkout` and `view_cart` weights stay as is.
- **platform-recsys offline**: ranker training uses `item_list_id` + `position` for position debias (relates to archived change
  `wire-debiased-ctr-ranker-features`, which already debiases nearline CTR) and adds context training features
  `session_source/medium/campaign` behind a flag, kept only when the offline evaluation does not regress (see Decisions).
- **platform-recsys nearline** (`python -m recsys.nearline`): drops events whose consent denies personalisation before they touch
  per-user keys; adds `view_cart` and `begin_checkout` to the session/recent signals.
- **Serving**: an actor without personalisation consent gets the existing popular path; no per-user vectors, recents or cache
  are read for them. No serving code or wire change is made for context.
- Not changed: `RecommendRequest`, no new proto fields, RPCs or topics. GA4 export is never a training input.

## Decisions

- **Consent source of truth** is `TrackingEvent.consent.personalization` on each event; `version = 0` means no cookie = all denied.
- **Grant boundary** (one rule everywhere: featurestore and nearline): for a `user_key`, only events after the latest event with
  `personalization = false` are used. Denied events are never written to per-user keys, so those keys hold only post-grant events.
- **Context-feature gate**: the training run evaluates the ranker with and without context features through the existing
  evaluator (`recsys/evals`) and only enables them if `ndcg@k` and `recall@k` of the with-context run are not lower than the
  baseline; the decision is recorded in the run report. Context is a training feature only. Online confirmation is read from `recsys-online-evaluation`
  (`GetRecommendationPerformance`) per model version; it is observational and does not gate promotion.

## NFR

- Nearline consent filter adds no extra network call; per-event overhead < 1 ms (in-process field read); consumer lag target unchanged
  (seconds).
- Serving latency budget of `Recommend` unchanged (p95 < 200 ms); the denied path is cheaper than the personalised path.
- Training/export stay batch jobs; export remains atomic (write temp, rename) and a failed export keeps the previous file.
- Legacy Parquet without the new columns keeps working with identical results (tolerant read).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `consent.personalization` / `version` | proof of consent | filter training/nearline/serving | Decree 13/2023 consent | kept with the event, `tracking_events` retention |
| `user_key` (principal id or `anon:<id>`) | pseudonymous id | interaction rows | personalisation consent | existing dataset retention; denied buyers' rows are never materialised |
| `session_source/medium/campaign` | marketing attribution, non-identifying | ranker training context | analytics consent (derived from GA client id) | `tracking_events` retention; not stored in Redis per user; training only |
| `item_list_id`, `position` | operational | position debias | necessary / operational stats | `tracking_events` retention |

No email, phone or name is read or written. Denied events are excluded before any per-user aggregation and are not written to
`recs:nearline:user:*` keys.

## Threat model

- **Assets**: consent decisions, per-user interaction history, recommendation personalisation.
- **Abuse cases**: (a) a denied buyer's events leak into training or nearline; (b) forged consent in the request body; (c) a later
  grant retroactively legitimises older events; (d) context features leak campaign names into a user-visible response;
  (e) corrupt or legacy rows with no consent value treated as granted.
- **Authz rule**: consent comes only from the edge-stamped `consent` field (never client body, owned by `ga4-consent-contract`);
  recsys treats missing/`version = 0` as denied; context features are model inputs only and never returned on the `Recommend` wire.
