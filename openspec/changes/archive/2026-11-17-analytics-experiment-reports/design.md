## Context

The contract slice fixed the RPC shapes. This slice picks the SQL semantics. All reads use `tracking_events_resolved` (adds `user_key`: the event's user principal, else the stitched principal, else `anon:<anonymous_id>`) and `order_facts` (PAID lines with `buyer_id`), the same sources `RecommendationPerformance` uses. Time is UTC.

## Decisions

D1. One capability interface per RPC in `internal/query`, as `ChannelFunnelRepository` does: `RetentionRepository.RetentionCohorts(ctx, from time.Time, weeks int, now time.Time)`, `ExperimentRepository.ExperimentReadout(ctx, flag string, from, to time.Time, windowDays int)`, `AiSurfaceRepository.AiSurfaceReport(ctx, from, to time.Time)`. `now` is passed in so tests are deterministic. Row structs hold counts, not ratios; the handler derives rates and intervals.

D2. Defaults and bounds. `weeks` 0 becomes 12, above 26 is `InvalidArgument`. `from` of the retention report is truncated to its UTC Monday; unset means `monday(now) - weeks*7 days` (12 weeks ago for the default), and the report returns `weeks` consecutive cohort weeks starting there, so the newest one is the week before the current one. For the readout and the AI report, unset `to` is now and unset `from` is `to - 30 days`; `from > to` or a window over 366 days is `InvalidArgument`. An empty `flag` or one over 64 characters is `InvalidArgument`.

D3. Retention (decision U5). Cohort anchor = the week of the user's first `sign_up` event (`min(occurred_at)` per `user_key` where the key is an authenticated user, i.e. not `anon:`), truncated to Monday. A user is active on day N when they have any event on the UTC calendar day `signup_day + N`. `d1`, `d7`, `d30` = active members / members whose day N has fully elapsed at `now`; no member matured means 0. `weekly_active[k]` = members with any event in week `cohort_week + k` over the cohort size, listed for every week k that has fully elapsed (the cohort week is k = 0, so it is only listed once that week is over). Repeat purchase = cohort members with a second distinct paid `order_id` within 30 days after their first paid order, over cohort members whose first paid order is at least 30 days old at `now`; no such buyer means 0. Cohorts with `cohort_size` 0 are omitted. Users who signed up before `sign_up` events existed have no row and are excluded.

D4. Experiment readout (decision U7). Exposure = `event_type = 'experiment_exposure'` with `json_extract_string(properties,'$.flag') = flag`, `occurred_at` within `[from, to]`. Each visitor (`user_key`) counts once, in the variant of their earliest exposure to that flag; variant `unassigned` is excluded. Converted = the visitor has a PAID `order_facts` line with `buyer_id = user_key` and `occurred_at` in `[first_exposure, first_exposure + 7 days]`. Anonymous visitors count as exposed and cannot convert until stitched. `rate = converted / exposed`, 0 when exposed is 0. `window_days` is always 7. Rows are ordered by variant name.

D5. Wilson interval, z = 1.96: with `p = c/n`, `centre = (p + z^2/2n) / (1 + z^2/n)`, `half = z * sqrt(p(1-p)/n + z^2/4n^2) / (1 + z^2/n)`, `ci_low = max(0, centre - half)`, `ci_high = min(1, centre + half)`. `n = 0` gives 0 and 0. Lives in `internal/rpc` as a pure function with a table test.

D6. AI-surface report. Rows come from `event_type = 'ai_interaction'` in `[from, to]`, grouped by `properties.surface`, `COALESCE(model_version,'')`; counts per `properties.action` in `shown|accepted|edited|dismissed`. Events with another action or surface (not possible through the collector, possible from old rows) are ignored. Ordered by surface, then model_version.

D7. Collector allowlist (gateway). `experiment_exposure` joins `beaconEventTypes`; `validateBeacon` requires exactly the keys `flag` and `variant`, each matching `^[a-z0-9][a-z0-9_.-]{0,63}$`, same style as the `ai_interaction` check. team-analytics maps the enum to the lowercase `event_type` name. The accepted pattern is a design choice (the contract fixes only "exactly flag and variant, no free text") and covers Flipt flag keys and variant keys.

D8. Errors. `requireAdmin` first, then argument validation, then capability check, so a non-admin never learns about bounds. `ErrUnsupported` and warehouse errors go through the existing `s.fail`, giving `Unavailable`. No value from event data is placed in an error message or log line.

D9. No BigQuery implementation: `OpenWarehouse` for BigQuery yields a repository without these capabilities, hence `Unavailable`. The SQL is plain DuckDB (`date_trunc`, `json_extract_string`); porting it is a later slice.

## Risks

- Anonymous exposure rows are not stitched until sign-in, so early readouts undercount conversions; documented in the README.
- Small cohorts give wide intervals by design; the interval conveys it.
- Hot files shared with `analytics-forecast-serving`: serialised by initiative-build; new code lives in new files.

## Clarifications

- D4 (user, 2026-11-15): a visitor whose earliest exposure to the flag has variant `unassigned` is excluded from the readout entirely; they are not re-counted under a later exposure's variant.
