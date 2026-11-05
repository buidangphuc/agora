## Why

The buyer PDP (`team-frontend`, `/listing/[id]`) stops streaming for some page sizes: curl exits 18 at about
121 KB, the document never closes, Playwright `goto` times out, and five UI e2e tests (admin_cases_ui dismiss,
admin_listing_moderation_ui approve, listing_report_ui guest and buyer, seller_listing_status_ui) fail. The
`e2e-ui-rebaseline` investigation traced it to "A previously unvisited boundary must have exactly one root segment.
This is a bug in React." thrown by the React server renderer that Next 15.5.28 vendors
(19.2.0-canary-0bdb9206-20250818). With the same seller, title and category, descriptions of 7-18 characters fail
and 1-6 and 19-250 pass.

The root cause is React bug facebook/react#34668, fixed upstream by facebook/react#34694 (2025-10-02, one line in
`flushSegment`). Next 15.5.29 still vendors the same React; Next 16.4.5 vendors one with the fix.

## What Changes

- New `team-frontend/scripts/patch-react-dom-boundary.mjs` backports facebook/react#34694 into every Fizz bundle
  Next vendors (`next/dist/compiled/next-server/app-page*.runtime.{prod,dev}.js`, minified, and
  `next/dist/compiled/react-dom{,-experimental}/cjs/react-dom-server*.js`): `flushSegment` sets
  `segment.boundary = null` before marking the boundary flushed. It is idempotent and exits 1 when a bundle that
  holds the invariant matches neither the buggy nor the fixed form, so a Next upgrade forces a re-check.
- `npm run dev` and `npm run build` (and so the Docker build) run it after `patch-react-dom-ping.mjs`.
- Tests: a vitest that runs the upstream regression case on the vendored `react-dom/server` and checks that every
  bundle is patched; an e2e scenario that creates 40 listings with descriptions of 1-40 characters and asserts each
  PDP response completes with `</html>` and the Q&A section.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `ui-product-detail`: adds "The page streams to the end".

## Impact

- `team-frontend/scripts/patch-react-dom-boundary.mjs`, `team-frontend/package.json`,
  `team-frontend/src/lib/react-dom-boundary-patch.test.ts`, `team-frontend/FEATURES.yaml`.
- `platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature`,
  `platform-e2e/tests/e2e/step_definitions/d1_pdp_steps.py`.
- No app code, contract or data change.

## Non-goals

Upgrading to Next 16 (needs `getToken()`/`getPrincipal()` made async across about 44 files, `middleware` to
`proxy`, and TypeScript fixes); patching `team-frontend-seller` and `team-frontend-admin`, which vendor the same
React.

## Non-functional requirements

- Every PDP response completes with a closed document; no "failed to pipe response" in `team-frontend-svc` logs for the 1-40 sweep.
- No change in response size or time beyond noise: the patch only stops a second flush of an already written boundary.
- Build fails closed: `npm run build` stops when the vendored bundles no longer match, instead of shipping unpatched.

## Data classification

No new data is collected, stored, logged or sent. The e2e sweep creates test listings with filler descriptions only.

## Threat model

- Assets: the build output of `team-frontend` (vendored Next runtime under `node_modules`).
- Abuse cases: (1) a tampered or changed Next bundle is silently patched wrongly -> the script only rewrites the exact upstream-known form and exits 1 on anything else; (2) a page that truncates could hide security headers or scripts -> the fix makes documents complete, headers are unchanged.
- Authz rule: unchanged; the PDP stays public and talks only to the gateway.
