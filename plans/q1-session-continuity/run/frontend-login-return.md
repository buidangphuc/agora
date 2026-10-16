# feature-run frontend-login-return
initiative: q1-session-continuity | base: e333dfac | worktree: .claude/worktrees/frontend-login-return | branch: asdlc/frontend-login-return | started: 2026-10-14T11:22:12Z
## Preflight   validate=p spec_lint=p digest=2ec945cfd7db (matches LOCK)
## E2E         author=running run=
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| L1 (1.1 returnUrl helpers + 1.5b gtm) | returnUrl guard, page_location | todo | 0 | | | | | |
| L2 (1.2-1.3 forms + actions) | login/register return | todo | 0 | | | | | |
| L3 (1.4-1.6 guards + links + scan) | guards, links | todo | 0 | | | | | |
| L4 (1.7-1.8 cart) | guest add / buy-now | todo | 0 | | | | | |
## Gates
## Escalations
- HUMAN DECISION 2026-10-14 (L1 BLOCKING: dataLayer {page_location} push is a no-op for GA4): change the GTM container — DLV `page_location` + fieldsToSet page_location on the GA4 Configuration tag in team-frontend/analytics/gtm-container.json, with a test asserting the mapping, and re-push on SPA navigation to /login. Container must be imported into the real GTM workspace on deploy.
## Tasks (final)
| task | status | rounds | red_first | task-reviewer | notes |
|---|---|---|---|---|---|
| L1 returnUrl helpers + GA4 | approved | 2 (+test-defect fix) | PASS | approve r2 | r1 BLOCKING: dataLayer push no-op → human chose GTM container mapping; client-safe returnUrlShared split |
| L2 forms + actions | approved | 1 | PASS | approve | actions.ts hot file: only 2 redirect lines + import |
| L3 guards + links + scan | approved | 2 | PASS | approve r2 | r1 BLOCKING: "use client" broken in ChatWithSellerButton → fixed + directives.test.ts guard |
| L4 guest cart | approved | 1 | PASS | approve | |
## Gates (final)  e2e=12/12 green (stack q1 + team-domain/team-order) features-check=pass spec_sync --strict=pass (12 e2e + 17 unit) vitest=1631/1631 typecheck=pass next build (docker)=pass feature-reviewer=pass 12/12 decide=ENGINE_DOWN (vacuous) privacy=pass
- privacy MINOR: page_referrer may still carry /login?returnUrl (consider Referrer-Policy same-origin on /login,/register); proposal wording "order ids not sent to analytics" only true for login page; guarded pages must not carry secrets in query.
- Doc nit: spec VERIFIED BY names src/features/auth/guards.test.ts, file is src/app/guards.test.ts (not changed — would need re-lock).
- Deploy: import team-frontend/analytics/gtm-container.json into the live GTM workspace.
