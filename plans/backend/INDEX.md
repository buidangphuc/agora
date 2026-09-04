# Plan Index — Close the real gaps (backend greenfield + thin UI)

| Wave | Task | Slug | Role | Writes | Verify | Status |
|------|------|------|------|--------|--------|--------|
| 0 | W0 | proto-contracts | SA | platform-core chat/v1 + engagement/v1; regen team-frontend/chat/engagement stubs | buf lint clean; `team-frontend tsc` clean | **done** |
| 1 | T1 | chat-search-backend | SE | team-chat/** | `go test ./...` | **done** (go test green) |
| 1 | T2 | recently-viewed-backend | SE | team-engagement/** | `make proto && make test` | **done** (make test green) |
| 1 | T3 | order-postpurchase-ui | SE | team-frontend src/lib/gateway/{orders,payment}.ts, src/app/account/orders/**, src/features/order/** | `npm run typecheck` + `npm run test -- src/features/order` | **done** (21 tests green) |
| 1 | T4 | listing-qa-ui | SE | team-frontend src/lib/gateway/engagement.ts, src/app/listing/[id]/**, src/features/listing/qa/** | `npm run typecheck` + `npm run test -- src/features/listing` | **done** (14 tests green) |
| 1 | T5 | openfeature-gitops | SRE | platform-gitops/argocd/apps/{team-order,team-frontend}.yaml | yaml lint | **done** (yaml valid) |

**Wave 0 notes:** Go 1.27 + buf 1.72 installed via brew (were absent). Proto vendored + stubs regenerated in team-chat (committed), team-engagement (`make proto`), team-frontend (`npm run proto`). Toolchain on PATH at /opt/homebrew/bin.
| 2 | W2 | integration | SE | team-gateway (2 forwarders), team-frontend (chat search UI, recently-viewed widget, autocomplete fix) | all host suites green | **done** (gateway make check + fe tsc/test green) |

**Wave 2 notes:** Gateway forwarders `SearchMessages`+`GetRecentlyViewed` added (make check green). FE: chat search wired to backend, "Vừa xem" home widget, `recordView` now called on listing view, autocomplete `{suggestions}` fix. Compose env batch = no-op (already wired in working tree). Frontend test: 151 pass / 4 fail = pre-existing `src/lib/track.test.ts` (unrelated). See 99-release.md for live-stack handoff.

Merge order after Wave 1: T1 → T2 → T5 → T3 → T4 (write-sets disjoint; order arbitrary).

## Dependency notes
- Wave 0 must complete before Wave 1 (T1/T2 vendor the new proto; T3/T4 need regenerated FE stubs).
- T3/T4 depend on already-shipped backend+gateway (no dependency on T1/T2).
- Wave 2 gateway forwarders depend on T1/T2 proto+service; greenfield FE depends on those forwarders.
- Live-stack e2e + coverage flips + `openspec archive` = handoff in `99-release.md` (needs running stack + deploy auth).
