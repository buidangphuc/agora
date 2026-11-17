# initiative-plan run — monetization
step reached: 5 (LOCK.md written, status pending; waiting for human sign-off)
date: 2026-11-13
spec: plans/monetization/00-spec.md (Locked 2026-11-13)

## Slices
| slice | owner | author | rounds | validate | spec_lint | spec-quality | decide | digest |
|---|---|---|---|---|---|---|---|---|
| monetization-contract | platform-core | done | 1 | pass | pass (1 warn) | pass | ENGINE_DOWN | f4e79137debd |
| order-paid-price-breakdown | team-order | done | 1 | pass | pass | pass | ENGINE_DOWN | d46c34ca75da |
| payment-settlement-commission | team-payment | done | 1 | pass | pass | pass | ENGINE_DOWN | d5732d11bc16 |
| payment-seller-fee-charge | team-payment | done | 1 | pass | pass (2 warn) | pass | ENGINE_DOWN | 6ca8f809d7d5 |
| seller-fee-breakdown-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | 874f4f22a608 |
| promotion-plan-billing | team-promotion | done | 1 | pass | pass (1 warn) | pass | ENGINE_DOWN | a100470118a5 |
| listing-plan-cap | team-domain | done | 1 | pass | pass | pass | ENGINE_DOWN | 7447b7996677 |
| seller-plan-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | 0448636b6787 |
| promotion-ads-serving | team-promotion | done | 1 | pass | pass | pass | ENGINE_DOWN | ca7ecb76bc52 |
| promotion-ads-campaign-mgmt | team-promotion | done | 2 | pass | pass | pass | ENGINE_DOWN | 0f165738f7b7 |
| buyer-sponsored-slots | team-frontend | done | 1 | pass | pass | pass | ENGINE_DOWN | 08195f3ebf82 |
| seller-ads-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | f79bddc721a6 |

## Cross-change check (cross_change_check.py, exit 1)
- ownership: `A seller clicking their own ad is not charged` in promotion-ads-serving and buyer-sponsored-slots
- ownership: `Budget below the bid is rejected` in promotion-ads-serving and seller-ads-ui
build order: monetization-contract -> listing-plan-cap -> order-paid-price-breakdown -> payment-seller-fee-charge -> payment-settlement-commission -> promotion-ads-serving -> buyer-sponsored-slots -> promotion-ads-campaign-mgmt -> promotion-plan-billing -> seller-ads-ui -> seller-fee-breakdown-ui -> seller-plan-ui

## Next
Human: answer LOCK.md Open issues (14 spec-changing questions), rename the 2 duplicate UI scenarios, re-run cross_change_check.py, recompute digests, then sign LOCK.md. Then initiative-build.
