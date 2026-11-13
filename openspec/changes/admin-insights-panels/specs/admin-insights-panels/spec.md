## ADDED Requirements

### Requirement: Placement and search performance panel
The cockpit at `/admin/cockpit` SHALL show a placements panel built from `GetRecommendationPerformance` and `GetSearchPerformance` for a selected trailing window of 24, 72 or 168 hours (default 24), with one row per placement and `model_version` and the per-placement fallback share. The panel SHALL render values as received and SHALL show a dash, never 0, for a value it did not receive.

#### Scenario: Recommendation rows per model version
- **WHEN** an admin opens `/admin/cockpit` and the warehouse holds `home_feed` impressions served by two different `model_version` values
- **THEN** the placements panel `insights_placements` lists one row per placement and `model_version` pair with impressions, clicks, CTR, add-to-carts, purchases and conversion rate, and the `model_version` text equals the stored value verbatim
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/admin/insights_panels.feature` "Admin reads placement performance per model version"

#### Scenario: Search rows are shown in their own table
- **WHEN** the same window contains search impressions with a `request_id`-stamped `model_version`
- **THEN** a second table inside `insights_placements` lists the `GetSearchPerformance` rows and the recommendation table does not include them
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/PlacementsPanel.test.tsx` "renders search rows separately"

#### Scenario: Fallback share is shown per placement
- **WHEN** `GetRecommendationPerformance` returns a fallback share of 0.25 for `home_feed`
- **THEN** the panel shows "25.0%" next to `home_feed` as fallback share
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/PlacementsPanel.test.tsx` "renders fallback share as a percentage"

#### Scenario: Window selection is sent as hours
- **WHEN** the admin opens `/admin/cockpit?window=168`
- **THEN** both RPCs are called with `window_hours=168` and the panel states "7 ngày"
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "sends window_hours from the selector"

#### Scenario: Window outside the allowlist falls back to the default
- **WHEN** the admin opens `/admin/cockpit?window=9999`
- **THEN** both RPCs are called with `window_hours=24` and no request carries a value above 168
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "clamps unknown window to 24"

#### Scenario: No impressions in the window
- **WHEN** both RPCs return zero rows
- **THEN** the panel shows "Chưa có dữ liệu trong khoảng này" and renders no table rows and no zero metrics
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/PlacementsPanel.test.tsx` "empty result shows the empty state"

#### Scenario: Report service unavailable
- **WHEN** team-analytics returns `UNAVAILABLE` or the call times out after 5 s
- **THEN** the panel shows "Tạm thời không có số liệu", renders no numbers, and the rest of the cockpit still renders
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/PlacementsPanel.test.tsx` "unavailable shows a message without numbers"

#### Scenario: Placement report denied without admin scope
- **WHEN** the gateway answers `GetRecommendationPerformance` with `PERMISSION_DENIED` because the session lacks the `admin` scope
- **THEN** the page shows the 403 result and renders no placement numbers
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "maps PermissionDenied to forbidden"

### Requirement: Retention cohort panel
The cockpit SHALL show a retention panel from `GetRetentionCohorts` with one row per weekly cohort (cohort week, size, D1, D7, D30, 30-day repeat-purchase rate and the weekly-active series), for a selected span of 4, 12 or 26 weeks (default 12). The `weeks` value sent SHALL always be within 1..26.

#### Scenario: Cohort rows
- **WHEN** an admin opens `/admin/cockpit` and `GetRetentionCohorts` returns a cohort for week 2026-11-02 with size 40, d1 0.5, d7 0.25, d30 0.1 and repeat_purchase_30d 0.05
- **THEN** `insights_retention` shows a row "2026-11-02", size 40, "50.0%", "25.0%", "10.0%" and "5.0%"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/admin/insights_panels.feature` "Admin reads weekly retention cohorts"

#### Scenario: Weekly-active series is shown in order
- **WHEN** a cohort carries `weekly_active` of [1, 0.6, 0.4]
- **THEN** the row lists "100.0%", "60.0%", "40.0%" for weeks 0, 1 and 2 in that order
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/RetentionPanel.test.tsx` "renders weekly active in order"

#### Scenario: Span selection is sent as weeks
- **WHEN** the admin opens `/admin/cockpit?weeks=26`
- **THEN** `GetRetentionCohorts` is called with `weeks=26`
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "sends weeks from the selector"

#### Scenario: Weeks above the upper bound is not sent
- **WHEN** the admin opens `/admin/cockpit?weeks=27`
- **THEN** `GetRetentionCohorts` is called with `weeks=12`
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "clamps unknown weeks to 12"

#### Scenario: No cohorts yet
- **WHEN** `GetRetentionCohorts` returns an empty list
- **THEN** the panel shows "Chưa có nhóm đăng ký nào" and no zero percentages
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/RetentionPanel.test.tsx` "empty list shows the empty state"

#### Scenario: Retention report unavailable
- **WHEN** `GetRetentionCohorts` returns `UNAVAILABLE`
- **THEN** the panel shows "Tạm thời không có số liệu" with no numbers while the other three panels still render
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/InsightsSection.test.tsx` "one failing panel does not blank the others"

### Requirement: Experiment readout panel
The cockpit SHALL show an experiments panel from `GetExperimentReadout` for the flag key the admin enters and a range of 7, 14 or 30 days (default 7), with one row per variant (exposed, converted, rate, 95% interval) and the conversion window in days. The panel SHALL render the interval as received and SHALL NOT compute rates or intervals itself. The flag key SHALL match `^[a-z0-9][a-z0-9_.-]{0,63}$` before any call is made.

#### Scenario: Variant rows with interval
- **WHEN** the admin opens `/admin/cockpit?flag=home_feed_ranker` and the readout returns variant `control` exposed 200 converted 20 rate 0.1 ci_low 0.066 ci_high 0.149
- **THEN** `insights_experiments` shows a `control` row with 200, 20, "10.0%" and "6.6% – 14.9%"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/admin/insights_panels.feature` "Admin reads an experiment readout"

#### Scenario: Conversion window is stated
- **WHEN** the response has `window_days=7`
- **THEN** the panel states "Cửa sổ chuyển đổi: 7 ngày"
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/ExperimentsPanel.test.tsx` "shows the conversion window"

#### Scenario: Interval is not recomputed
- **WHEN** the response rate is 0.1 and ci_low and ci_high are 0.05 and 0.2
- **THEN** the panel shows "5.0% – 20.0%", the values received, not a value derived from exposed and converted
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/ExperimentsPanel.test.tsx` "renders the received interval"

#### Scenario: No flag entered
- **WHEN** the admin opens `/admin/cockpit` without a `flag` parameter
- **THEN** the panel shows "Nhập flag key để xem kết quả" and `GetExperimentReadout` is not called
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "makes no call without a flag"

#### Scenario: Malformed flag key is rejected
- **WHEN** the admin opens `/admin/cockpit?flag=` followed by 65 characters or a value containing a space or markup
- **THEN** the panel shows "Flag key không hợp lệ", `GetExperimentReadout` is not called and the value is rendered as escaped text
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/ExperimentsPanel.test.tsx` "rejects a malformed flag key"

#### Scenario: Flag key at the length boundary
- **WHEN** the flag key is exactly 64 characters matching the pattern
- **THEN** `GetExperimentReadout` is called with that key
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "accepts a 64 character flag"

#### Scenario: Flag with no exposures
- **WHEN** the readout returns no variants for a valid flag
- **THEN** the panel shows "Chưa có lượt hiển thị cho flag này" and no zero rows
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/ExperimentsPanel.test.tsx` "no variants shows the empty state"

#### Scenario: Upstream rejects the request
- **WHEN** `GetExperimentReadout` returns `INVALID_ARGUMENT`
- **THEN** the panel shows "Tham số không hợp lệ" with no upstream message text and the other panels still render
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "maps InvalidArgument to invalid without a message"

#### Scenario: Experiment readout denied without admin scope
- **WHEN** the gateway answers `GetExperimentReadout` with `PERMISSION_DENIED` because the session lacks the `admin` scope
- **THEN** the page shows the 403 result and renders no variant rows
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "maps PermissionDenied to forbidden"

### Requirement: AI surface panel
The cockpit SHALL show an AI-surfaces panel from `GetAiSurfaceReport` for a range of 7, 14 or 30 days (default 7), with one row per surface and `model_version` giving shown, accepted, edited, dismissed and the accepted and dismissed share of shown. The share SHALL be a dash when `shown` is 0.

#### Scenario: Surface rows with shares
- **WHEN** the report returns surface `assistant` and `model_version` `mock:v1|hybrid-v2` with shown 10, accepted 4, edited 1, dismissed 2
- **THEN** `insights_ai_surfaces` shows that row with 10, 4, 1, 2, accepted "40.0%" and dismissed "20.0%"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/admin/insights_panels.feature` "Admin reads AI surface usage"

#### Scenario: Zero shown has no share
- **WHEN** a row has shown 0 and dismissed 3
- **THEN** the accepted and dismissed share cells show a dash
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/AiSurfacesPanel.test.tsx` "zero shown renders a dash share"

#### Scenario: Range is sent as a time window
- **WHEN** the admin opens `/admin/cockpit?range=30` at server time T
- **THEN** `GetAiSurfaceReport` and `GetExperimentReadout` are called with `to=T` and `from=T-30d`
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "range becomes from and to"

#### Scenario: Range outside the allowlist falls back
- **WHEN** the admin opens `/admin/cockpit?range=365`
- **THEN** the range used is 7 days
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "clamps unknown range to 7"

#### Scenario: No AI interactions in range
- **WHEN** the report returns no rows
- **THEN** the panel shows "Chưa có tương tác AI trong khoảng này" and no table rows
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/AiSurfacesPanel.test.tsx` "empty result shows the empty state"

#### Scenario: AI report unavailable
- **WHEN** `GetAiSurfaceReport` fails or times out after 5 s
- **THEN** the panel shows "Tạm thời không có số liệu" with no numbers
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/AiSurfacesPanel.test.tsx` "unavailable shows a message without numbers"

### Requirement: Admin-only server-side access to the insight reports
The five report calls SHALL run only in server code with the per-request session bearer, only for a principal whose scopes include `admin`, and SHALL NOT send the bearer, an upstream error message or a personal identifier to the browser. The panels SHALL display aggregates only.

#### Scenario: Admin sees all four panels
- **WHEN** a logged-in admin opens `/admin/cockpit`
- **THEN** `insights_placements`, `insights_retention`, `insights_experiments` and `insights_ai_surfaces` are present on the page
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/admin/insights_panels.feature` "Admin sees all four insight panels"

#### Scenario: Non-admin gets no report calls
- **WHEN** a logged-in user without the `admin` scope opens `/admin/cockpit`
- **THEN** the existing 403 result is shown, none of the four panels is rendered and no report RPC is called
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cockpit/page.test.tsx` "non-admin renders 403 and calls no report RPC"

#### Scenario: Gateway denies a report call
- **WHEN** the gateway answers a report RPC with `PERMISSION_DENIED` for a session that decoded as admin
- **THEN** the page shows the 403 result and no report numbers
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cockpit/page.test.tsx` "gateway permission denied renders 403"

#### Scenario: Anonymous visitor is redirected
- **WHEN** a visitor with no session opens `/admin/cockpit`
- **THEN** the response redirects to the login page and no report RPC is called
- **VERIFIED BY**: `team-frontend-admin/src/app/admin/cockpit/page.test.tsx` "guest is redirected to login"

#### Scenario: Expired session is redirected
- **WHEN** a report RPC answers `UNAUTHENTICATED`
- **THEN** the page redirects to the login page instead of rendering an empty panel
- **VERIFIED BY**: `team-frontend-admin/src/lib/gateway/insights.test.ts` "maps Unauthenticated to unauthenticated"

#### Scenario: Bearer and upstream text never reach the browser
- **WHEN** the cockpit page HTML is rendered while a report RPC fails with a message containing an internal address
- **THEN** the HTML contains neither the bearer token nor that message
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/insights/InsightsSection.test.tsx` "serialized output has no token or upstream text"
