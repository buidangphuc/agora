## ADDED Requirements

Precondition: `GetTrackingQualityReport` is already admin-only (existing requirement in `openspec/specs/tracking-data-quality/spec.md`, `team-analytics/README.md`, `edge-route-policy`); this change adds reasons behind that gate.

### Requirement: Daily GA4 and first-party counts are reconciled
After each import, the system SHALL compare, per UTC day and per event type received by both pipelines (view, click, add-to-cart, begin_checkout), the GA4 count in `ga4_events_daily` with the first-party count in `tracking_events`. When the difference ratio `|ga4 - first_party| / max(first_party, 1)` exceeds `GA4_RECONCILE_MAX_DIFF_RATIO` for any type, `GetTrackingQualityReport` SHALL report status `DEGRADED` with reason `ga4_divergence` and the gauge `ga4_reconcile_diff_ratio{event_type}` SHALL be set so the alert rule in `platform-gitops` fires. A day with no import run SHALL yield reason `ga4_missing` and no divergence value.

#### Scenario: Divergence beyond the threshold raises the alert
- **WHEN** GA4 reports 50 views and first-party reports 100 for a day and the threshold is 0.35
- **THEN** the report status is `DEGRADED` with reason `ga4_divergence` and the gauge for `view` equals 0.5

#### Scenario: Divergence at the threshold does not alert
- **WHEN** GA4 reports 65 views and first-party reports 100 and the threshold is 0.35
- **THEN** the report contains no `ga4_divergence` reason

#### Scenario: Zero first-party events are handled
- **WHEN** first-party reports 0 clicks and GA4 reports 3
- **THEN** the ratio equals 3 and the report contains `ga4_divergence` without a division error

#### Scenario: A missing import is reported, not treated as divergence
- **WHEN** no import run exists for yesterday
- **THEN** the report carries reason `ga4_missing` and no `ga4_divergence` reason

#### Scenario: A missing import adds a reason only
- **WHEN** no import run exists for yesterday and the stream is otherwise healthy
- **THEN** the report status stays `OK` and `reasons` contains `ga4_missing`

#### Scenario: Reconciliation failure does not break the report
- **WHEN** reading `ga4_events_daily` fails
- **THEN** the report still returns the existing statuses and counts without GA4 reasons

#### Scenario: Only admins see reconciliation reasons
- **WHEN** an anonymous client and then a logged-in buyer call `GetTrackingQualityReport` through the gateway
- **THEN** the gateway answers HTTP 401 and then HTTP 403
