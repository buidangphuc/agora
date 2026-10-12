# feature-run add-ga4-destination
initiative: ga4-analytics | base: 09a1f6d0 | worktree: .claude/worktrees/add-ga4-destination | branch: asdlc/add-ga4-destination | started: 2026-10-12T09:58:01Z
## Preflight   validate=p spec_lint=p spec_sync=23 uncovered | digest matches LOCK
## E2E         author=done run=red as expected (26 fail, 3 skip env-gated) files=4 commit 9b3396ff
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
## Gates       e2e=24 pass, 1 fail fixed by 1.5 R3 (unit), 5 env-gated skips; W1 consent/dataLayer/discovery regressions green; emit_tracking variant-change fail pre-existing; features-check=pass; spec_sync --strict=pass (24 e2e + 1 unit)
## Escalations
| 1.1 container | Container | approved | 1 | 9b3396ff | pass | - | approve (NITs: gaawe tags lack sendEcommerceData; empty measurement-id constant to fill at import) | - |
| 1.2 identity route | user_id | approved | 2 | ccb4fef9 | pass | (manual: auth) | approve | auth-scope R1 BLOCKING (unverified JWT sub) → spec-clarify R1 + re-lock 88f6794906cb → closed R2 |
| 1.2b gateway principal | Gateway principal, Forged session | approved | 1 | 2fbe2b5e | pass | (manual: auth, gateway) | approve (NITs: dead 405 branch; server-only import) | auth-scope pass, contract-boundary pass |
| 1.3 user_id push + debug | user_id client, debug | approved | 3 | e1440144 | pass | - | approve R3 (R1: container didn't read user_id/debug_mode + APP_ENV not build-arg; R2: identity re-fire duplicated page_view → split GA4 Identity tag sendPageView=false) | NEEDS_HUMAN: pipeline must pass --build-arg NEXT_PUBLIC_APP_ENV=staging for staging only |
| 1.4 CSP | CSP | approved | 2 | 372a7ccb | pass | - | approve R2 (R1: dev eval/ws, missing public URL build args, blob:) | NEEDS_HUMAN: pipelines + e2e compose must pass NEXT_PUBLIC_GATEWAY_URL/MEDIA_BASE_URL build args; tagassistant.google.com for GTM preview |
| 1.5 gaClientId | ga_client_id | approved | 3 (converge e2e: non-numeric parts, GA2 prefix) | 254c547e | pass | - | approve (NITs: regex accepts non-numeric parts; e2e regex looser) | - |
| 1.6 README | - | approved | 1 | 6d3ffed4 | exempt (docs) | - | orchestrator check: statements match code | - |
- privacy-reviewer: needs-human, no BLOCKING. NEEDS_HUMAN: banner text must name Google/GA4 + cross-device user_id purpose; cross-border transfer dossier (Decree 13 art. 25); GA4 14-month retention + Google Signals off are console settings to confirm; GA4_USER_ID_HMAC_KEY provisioning (ExternalSecrets) + rotation. MINOR: push {user_id: undefined} on withdrawal; confirm /api/track logs never print body. NIT: proposal text says identity derives from cookie (now gateway-verified).
- feature-reviewer (ENGINE_DOWN, checklist): fail (strict) — 5 automated entries only skipped (unconfigured-environment, missing-key, no-ids-in-dev-ci-e2e, staging debug, production debug) + malformed-_ga e2e not re-run after R3 fix. TODO: variant builds (no GTM+no key; APP_ENV=staging; APP_ENV=production) and re-run, after recsys converge frees docker.
- 2026-10-12 PAUSED (user: subagent quota): env-gated variant e2e run (A staging, B production, C no GTM/no key) was stopped mid-run; containers stopped; agora-team-frontend:local may be a VARIANT build — rebuild standard (GTM-E2E0001 + gateway/media args) before reuse.
- variant e2e (gd3-A/B/C): staging 6/6 (debug + all malformed _ga rows incl GA2), production 1/1, no-GTM/no-key 3/3 → feature-reviewer open items closed
