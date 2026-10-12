# initiative-build — ga4-analytics
started: 2026-10-12 | integration: .claude/worktrees/ga4-analytics (branch asdlc/ga4-analytics from origin/main 4ee39f7a) | wip_limit 3
preconditions: LOCK approved (Bùi Đăng Phúc, 2026-10-12), digests match, cross_change_check OK
## Waves (validate_plan: 6 packets, 4 waves, no conflicts)
| wave | slices | base | status |
|---|---|---|---|
| 0 | ga4-consent-contract (also vendors analytics.proto) | asdlc/ga4-analytics | merged fc50fc6e (feature gate pass; 4 funnel e2e deferred to add-ga4-export-import) |
| 1 | add-consent-mode, add-ga4-export-import | asdlc/ga4-analytics fc50fc6e | merged 715bbc0b + c92f6c5d; integration e2e 37 pass 2 skip (needed services only); spec_sync --strict pass x3 |
| 2 | add-ga4-destination, recsys-consent-and-context | asdlc/ga4-analytics 09a1f6d0 | merged 8b387614 (recsys) + 7c1ccd77 (destination); slice e2e green incl. env-gated variants; features-check + spec_sync --strict pass x5; cross_change_check OK |
| 3 | add-ga4-purchase-relay | asdlc/ga4-analytics 7c1ccd77 | merged 5d688977 + purge follow-up 353e52cb (relay e2e 30/30); slice e2e 53/53 (w3-e2e-final.log); features-check + spec_sync --strict ×6 + openspec validate ×6 pass; cross_change_check OK |
## Slice state files
plans/ga4-analytics/run/<slice>.md (feature-run)
## Notes
- write-sets derived from tasks.md by subagent; uncertain: new platform-core/FEATURES.yaml (tracking-consent owner), contract proto test language/location, consent footer file TBD, gitops placeholders, possible team-analytics/Dockerfile for cmd/ga4import
