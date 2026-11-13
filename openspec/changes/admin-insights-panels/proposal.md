## Why

Admins can read only tracking quality in the cockpit. The placement, search, retention, experiment and AI-surface numbers that the docs-ai-data initiative produces have no screen: `GetRecommendationPerformance` and `GetSearchPerformance` are admin-gated at the edge but no UI reads them, and the three new report RPCs (`GetRetentionCohorts`, `GetExperimentReadout`, `GetAiSurfaceReport`) would be unreachable. This slice adds four read-only panels to `/admin/cockpit` so an admin can see which recommendation model, experiment arm or AI surface is working, with an explicit "no data" and "unavailable" state instead of zeros.

initiative: docs-ai-data

## What Changes

- `team-frontend-admin` gains an `analytics` Connect client in `src/lib/gateway/client.ts` (server-only, per-request bearer, like the existing clients) for `AnalyticsQueryService`, built from the vendored `proto/platform/analytics/v1/analytics.proto` refreshed by `docs-ai-data-contract`.
- New server-only module `src/lib/gateway/insights.ts`: one function per RPC (`fetchPlacementPerformance`, `fetchSearchPerformance`, `fetchRetentionCohorts`, `fetchExperimentReadout`, `fetchAiSurfaceReport`). Each returns `ok | empty | unauthenticated | forbidden | invalid | unavailable`, never throws, never returns an upstream error message, 5 s timeout.
- Four panels rendered under the existing tracking-quality panel on `/admin/cockpit`, in `src/features/admin/insights/`:
  1. Placements: rows of `GetRecommendationPerformance` (placement, `model_version`, impressions, clicks, CTR, add-to-carts, purchases, conversion) with the per-placement fallback share, and rows of `GetSearchPerformance` in a second table. Window selector 24 h / 72 h / 7 d (`window_hours` 24, 72, 168).
  2. Retention: one row per weekly cohort from `GetRetentionCohorts` (cohort week, size, D1, D7, D30, 30-day repeat-purchase rate, weekly-active series). Selector 4 / 12 / 26 weeks.
  3. Experiments: per-variant exposed, converted, rate and 95% interval from `GetExperimentReadout` for the flag key the admin types, with the conversion window in days. Range selector 7 / 14 / 30 days.
  4. AI surfaces: per surface and `model_version`, shown / accepted / edited / dismissed from `GetAiSurfaceReport`, plus accepted and dismissed share of shown. Same range selector.
- Selections are URL query parameters (`window`, `weeks`, `flag`, `range`) read by the server page, so a view is linkable and the page needs no client-side business logic. Unknown values fall back to the defaults (24 h, 12 weeks, 7 d).
- Display rules: shares are rendered as received (`x * 100` with one decimal), the Wilson interval is rendered as received and never recomputed, `model_version` is shown verbatim and never parsed, and a missing value is a dash, never `0`.
- Failure isolation: each panel loads independently; one unavailable panel does not blank the others or the existing cockpit.
- Vietnamese copy for headings, states and notes; metric names and `model_version` stay as is.

## Capabilities

### New Capabilities
- `admin-insights-panels`: the four admin cockpit report panels, their selectors, empty and unavailable states, and the admin-only server-side access path.

### Modified Capabilities
none

## Impact

- `team-frontend-admin`: `src/lib/gateway/client.ts`, new `src/lib/gateway/insights.ts`, new `src/features/admin/insights/*`, `src/app/admin/cockpit/page.tsx`, `FEATURES.yaml` (by the e2e-author), vitest tests beside each module.
- `platform-e2e`: one feature file under `features/admin/` and step definitions; seeding goes through the existing tracking collector and the existing admin login.
- Depends on `analytics-experiment-reports` (the three RPCs served and routed with `adminScope`) and on `docs-ai-data-contract` (vendored proto). No gateway, analytics or proto change here.

## Non-goals

No new RPC, no proto change, no gateway route. No charts or export. No flag list or flag management (the admin types the flag key; Flipt is not called from this app). No client-side computation of CIs, rates or cohort math. No change to the existing cockpit metrics poll or `TrackingQualityPanel`. No seller or buyer UI.

## Non-functional requirements

- Latency: the five report calls run in parallel on the server; each has a 5 s timeout; the cockpit page adds at most one report-RPC round trip (p95 target 2 s from the initiative) to first render, and the existing metrics fetch is not delayed by a failing panel.
- Availability: a report failure degrades only its own panel to "Tạm thời không có số liệu"; the cockpit remains usable.
- Limits: weeks sent are always within 1..26, `window_hours` within 1..168, flag key at most 64 characters; the app never sends a value the RPC rejects.
- Accessibility and stability: tables use the existing `Table`, `Card`, `Empty`, `Alert` components, fixed `en-US` number formatting so server and browser render identically (hydration-safe), every panel has a stable `data-testid`.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| Placement, search, retention, experiment and AI-surface rows | Aggregate counts and ratios, no ids (contract rule) | Admin decides which model, arm or surface to keep | Legitimate interest of operating the marketplace; the aggregation basis (all first-party events, no `consent_analytics` filter, U6) is the initiative's open DPO item, not decided here | Not stored: fetched per request, rendered, never persisted or logged |
| `flag` query parameter | Operational config key, not personal | Select the experiment | n/a | Only in the request URL and server access log; the validation pattern blocks free text beyond a key |
| Admin session bearer | Credential | Authenticate the report calls | Session | Server-side only; never in HTML, props, query strings or logs |

The panels render no user id, visitor id, listing id or free text. Small cohorts and small variants are shown with their size so a reader can see they are small; suppression thresholds belong to the report RPCs, not the UI.

## Threat model

- Assets: the admin session bearer; business-sensitive aggregates (conversion, retention).
- Abuse cases: (1) a non-admin or anonymous visitor opening the cockpit to read reports; (2) the bearer leaking to the browser through a client-side fetch or a serialized prop; (3) an upstream error string with internal addresses reaching the page; (4) a crafted `flag`, `weeks` or `window` parameter used to send an out-of-range or oversized request, or injected into markup; (5) a fabricated zero read as a real result.
- Authz rule: the page renders the panels only for a principal whose decoded scopes include `admin` (render gating); the gateway is the enforcement point with `adminScope` on all five RPCs and team-analytics enforces admin again. Anonymous is redirected to login, a verified non-admin gets the existing 403 result and no report call is made. Calls run only in server components with the per-request token. Errors map to fixed Vietnamese strings. Parameters are parsed against an allowlist or a strict pattern and rendered as React text.
