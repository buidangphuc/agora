# Tasks

## Code track (team-frontend)

- [ ] 1.1 Create `analytics/gtm-container.json` with GA4 config + event tags, consent-gated, no purchase tag, measurement id as variable (Container scenarios)
- [ ] 1.2 Add `src/lib/analytics/ga4/userId.ts` (HMAC-SHA256 hex, server-only) and route `src/app/api/analytics/identity/route.ts` (consent + session checks, 204 paths, no-store) (user_id scenarios)
- [ ] 1.3 Extend `destinations/gtm.ts` and `AnalyticsProvider.tsx`: after consent update fetch identity and push `{ user_id }`; push `debug_mode` in staging; stop on withdrawal via `consent update` analytics_storage=denied or `a=0` (user_id, debug scenarios; edits hot files after add-consent-mode)
- [ ] 1.4 Build CSP in `next.config.mjs` headers with Google hosts (incl. `*.analytics.google.com`, `region1.google-analytics.com`) only when `NEXT_PUBLIC_GTM_ID` set (CSP scenarios)
- [ ] 1.5 Add `gaClientId` to `WireTrackBeacon`/`map.ts`/queue, parse `_ga` only with consent, bound to 64 chars (ga_client_id scenarios)
- [ ] 1.6 Document env vars `GA4_USER_ID_HMAC_KEY`, `NEXT_PUBLIC_GTM_ID`, `NEXT_PUBLIC_APP_ENV` in README; no values committed

## e2e track (platform-e2e `tests/e2e/features/tracking/`)

- [ ] 2.1 Container JSON structure, consent gating, no purchase tag, no secrets (Container scenarios, no key/secret values)
- [ ] 2.2 Identity endpoint: consent/anonymous/missing-key/param-ignored/no-store, p95 < 50 ms over 100 calls, withdrawal, HMAC equals reference (user_id scenarios)
- [ ] 2.3 CSP header with and without GTM id; assert 0 network requests to Google in CI (CSP scenarios, "No IDs in dev, CI and e2e")
- [ ] 2.4 Staging vs production `debug_mode` (debug scenarios)
- [ ] 2.5 Beacon `gaClientId` with/without consent, malformed cookie, per-event consent, length bound (ga_client_id scenarios)
