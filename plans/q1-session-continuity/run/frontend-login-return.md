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
