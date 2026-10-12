# feature-run add-consent-mode
initiative: ga4-analytics | base: fc50fc6ed32a15d4d6f0e5b9df694377bedabcb8 | worktree: .claude/worktrees/add-consent-mode | branch: asdlc/add-consent-mode | packet: plans/ga4-analytics/wave1-task1-add-consent-mode.md | started: 2026-10-12
## Preflight   validate=pass spec_lint=pass spec_sync=uncovered (expected); 1 cross-check issue: tasks.md e2e-cls lacks capability: (fails --strict)
## E2E         author=done run=red as expected (9 red: no banner dialog, field 26 not stamped) files=6 commit 395b3a4e; round 2: pre-seed consent cookie in ga4_datalayer e2e
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
| fe-consent-module | 10 cookie-format scenarios | approved | 1 | e8f33515 | PASS | none | approve (minor: dup keys last-wins = LOCK open Q; 256 cap counts UTF-16 not bytes) | - |
| fe-banner | first visit no Google, no CLS, returning no banner, keyboard | approved | 1 | ad900a14 | PASS | none | approve (minor: no focus move on reopen, act() warning) | - |
| fe-gtm-gating | accept loads GTM+update, signals map, returning once, rejected never, empty id off, revoke stops + no push w/o consent | approved | 3 | 3e68f7ff | PASS (exempt setup tests) | none | approve r3 (r1 arrays→Arguments, r3 privacy gate) | - |
| fe-beacon-independent | rejected visitor still beacons, consent change later only (guard) | approved | 1 | 7455f4b5 | PASS (Red-First-Exempt guard) | none | approve | - |
| gw-consent-stamp | 13 gateway cookie/stamp scenarios | approved (+3 gaClientId scenarios, re-lock 6f364dd36d11) | 1 | 1f5750e6 | PASS | none | approve (minor: comment says chars, check is bytes) | contract-boundary PASS |
## Gates       r3: slice e2e 16/16 (consent + ga4_datalayer, after privacy gate); e2e=15/15 slice, regression 190 pass (4 fail = funnel, other slice) features-check=pass spec_sync --strict=pass (9 e2e + 29 unit) feature-reviewer=pass (38/38) privacy=PASS r2 (gate fixed; NEEDS_HUMAN items listed) decide=ENGINE_DOWN (LLM fallback)
## Escalations
- spec-clarify (human-approved): tasks.md e2e-cls + capability: consent; re-lock digest 132381ced383 (commit with e2e checkpoint)
- human (2026-10-12): add VERIFIED BY lines for unit-only scenarios + not-testable FEATURES entries via spec-clarify after code tasks land, then re-lock
- typecheck: team-frontend src/lib/gateway/session.ts UnsafeUnwrappedCookies error also on main tree (pre-existing, out of scope)
- human (2026-10-12): plan gap 2 — gateway copies beacon gaClientId → TrackingEvent.ga_client_id (blank unless analytics) → added to gw-consent-stamp via spec-clarify; re-lock after
- spec-clarify VERIFIED BY (38 lines / 26 scenarios) committed; re-lock ad216e58bc03
- privacy r1 BLOCKING: pushDataLayer not gated on analytics consent → human: block ecommerce pushes while a=0 → spec-clarify + fe-gtm-gating round 3 (dataLayer.ts) + e2e (ga4_datalayer ecommerce scenarios need a=1 seed)
- privacy NEEDS_HUMAN: lawful basis for first-party beacon (anonymousId/sessionId) without consent; concrete retention + deletion path; cross-border disclosure (Google); banner copy; minor: gaClientId format check, ts max-age not enforced at edge
- tooling: red_first.sh buf generate in team-frontend fails (protoc-gen-es not on PATH; plugins live in node_modules/.bin) — WARN only
- 2026-10-12: user — don't run the full stack; regression for consent deferred to integration converge with only needed services
- merged into asdlc/ga4-analytics as 715bbc0b (audit clean vs fc50fc6e)
