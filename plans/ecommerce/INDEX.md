# Plan Index — 4 ecommerce features (parallel, e2e each)

Spec: [00-spec.md](00-spec.md) (must be `## Locked`). Fan-out: subagents, 1 session. Gate each wave before the next.

| Wave | Task | Slug | Repo(s) | Writes (summary) | Verify | Status |
|------|------|------|---------|------------------|--------|--------|
| 0 | T1 | foundation | platform-core, team-promotion(new), team-gateway, root, gitops, frontend/e2e shells | all proto + skeleton + shared registrations | repo_doctor + buf + promotion build | done ✅ |

> W0 progress: ✅ all 5 proto changes (buf lint + breaking GREEN). ✅ team-promotion skeleton builds+vets+tests in Docker; team-order regenerated (promotion client + order voucher fields) builds. ✅ infra compose (postgres-promotion :5440), services compose (team-promotion :50061 + migrate + promotion.events + UPSTREAM_PROMOTION_ADDR on order — :50060 collision with team-ai fixed → :50061), AGENTS.md port table, team-promotion/FEATURES.yaml, repo_doctor clean. ⏳ gateway promotion registration + frontend/e2e shells (folded into pilot Wave 2/3). ⏳ gitops (release wave). PILOT = Voucher/Flash-sale first.
| 1 | T1 | promotion-service | team-promotion | internal/** + migrations | go test ./... | done ✅ (voucher+flashsale, bufconn e2e; envelope→proto EventEnvelope fixed by lead) |
| 1 | T2 | order-redemption | team-order | service/redemption+saga, upstream/promotion, mig 0005 | go test ./... | done ✅ (7 tests green in Docker) |
| 1 | T3 | search-facets | team-search | internal/** | go test ./... | done ✅ (build+vet+tests green; facets impl) |
| 1 | T4 | engagement-wishlist-reviews | team-engagement | wishlist.go + reviews.go + mig 0004/0005 | go test ./... | done ✅ (wishlist trio + review enrichment; 4 gates green) |
| 1 | T5 | notification-alerts | team-notification | alerts + consumer + mig 002 | go test ./... | done ✅ (alert CRUD + listing.events consumer, 10 tests green) |
| 2 | T1 | gateway-forwarders | team-gateway | edge/{promotion,search,engagement,notification,order}.go | go test ./... | done ✅ all 4 features (promotion+order+engagement+search; NotificationService wired end-to-end — was never edge-exposed; gates green) |
| 2 | T2 | fe-promotion | team-frontend | app/vouchers, features/voucher, components/flashsale, gateway/promotion.ts | typecheck+build | done ✅ (typecheck+build+135 tests green; e2e selectors exposed; team-promotion image builds; stack booting) |
| 2 | T3 | fe-search | team-frontend | app/search, features/search, gateway/search.ts | typecheck+build | done ✅ (facet sidebar; typecheck+build green) |
| 2 | T4 | fe-engagement | team-frontend | app/favorites,features/engagement,features/review | typecheck+build | done ✅ (wishlist+reviews UI; merged FE build EXIT=0) |
| 2 | T5 | fe-notification | team-frontend | app/notifications,features/notification,components/alerts | typecheck+build | done ✅ (alert toggle+subs UI; merged FE build EXIT=0) |
| 3 | T1 | e2e-promotion | platform-e2e, team-promotion | features/promo, promo_steps, vouchers_page, FEATURES.yaml | pytest -k promo | done ✅ 5 promo scenarios PASS on live stack; FEATURES.yaml automated |

> 🎉 PILOT (Voucher/Flash-sale) COMPLETE end-to-end. Features 2/3/4: backends+gateway+frontend all green; stack live-smoke GREEN (search facets 200; alerts subscribe/list 200; engagement auth-gated OK).
> **Integration fixes (lead, on live stack):** (1) team-search couldn't resolve `opensearch` — the infra opensearch was orphaned off `platform-core_default` (host :9200 held by unrelated listing-search-etl); aliased the running opensearch onto the network → team-search boots. (2) team-notification had NO DB/migrate/Kafka in compose + main.go never wired the alert service into the gRPC handler (only the consumer) → provisioned postgres-notification (:5441) + migrate job + KAFKA/listing.events env + wired WithAlertService; alerts now live. repo_doctor: no drift.
| 3 | T2 | e2e-search | platform-e2e, team-search | features/frontend/search_facets, search_sort_steps, search_page, FEATURES.yaml | pytest -k facet | done ✅ 2 scenarios PASS (stable ×4); found+fixed real bug: EnsureIndex not idempotent (cold-boot index race crash-looped indexer) |
| 3 | T3 | e2e-engagement | platform-e2e, team-engagement | features/engagement, ... | pytest | done ✅ collections+review-media+helpful+verified-purchase all PASS (xfail removed) |
| 3 | T4 | e2e-notification | platform-e2e, team-notification | features/notification, ... | pytest | done ✅ subscribe/list/unsub + price-drop & back-in-stock DELIVERY all PASS (consumer now handles ListingChanged; xfail removed) |
| 4 | T1 | convergence | all | 99-release.md + fixes | full suite + repo_doctor | done ✅ 16/16 feature e2e PASS; manifests valid 60/60; repo_doctor no drift; validate_plan OK; 99-release.md written |

## Dependency notes
- Wave 0 MUST finish + be green before any Wave 1 fan-out (it owns every shared/hot file: all proto, gateway registry, compose, port table, frontend/e2e shells).
- Wave 1 tasks are file-disjoint (different repos, or different files in team-engagement). T2 compiles against Wave-0 generated stubs, not T1's code.
- Wave 2 forwarders (T1) + 4 frontend tasks are disjoint (backend gateway repo vs per-feature frontend dirs; shared shells owned by Wave 0).
- Wave 3 e2e needs the full stack running; feature dirs/step files/page objects disjoint (shared slot locators owned by Wave 0).
- Wave 4 solo.

## Merge order per wave
Arbitrary within a wave (write-sets disjoint); be consistent: T1 → T2 → … Wave N+1 starts from post-merge state of Wave N.

## Status legend
todo | running | review | done | failed

## Per-task protocol
1. Subagent prompt = 00-spec.md + this packet + work-rules block ("only write in your Write-set; STOP and report if you must touch another file").
2. On completion: `python scripts/validate_plan.py plan-ecommerce/ --audit plan-ecommerce/<packet> --base <base-ref>` → diff outside write-set = reject.
3. Review gate by a different agent (route to the reviewers named in the packet). Verdict done | failed:<reason>.
4. Fail → 1 retry with reason appended; 2nd fail → mark failed here, escalate.
