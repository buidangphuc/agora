# initiative-plan run — seller-centre
step reached: 4b + 5 (cross-check run, LOCK.md written, status pending; waiting for human)
date: 2026-11-13
spec: plans/seller-centre/00-spec.md (Locked 2026-11-13)

## Slices
| slice | owner | author | rounds | validate | spec_lint | spec-quality | decide | digest |
|---|---|---|---|---|---|---|---|---|
| seller-centre-contract | platform-core | done | 1 | pass | pass | pass | ENGINE_DOWN | 8b8008504400 |
| domain-storefront-customisation | team-domain | done | 1 | pass | pass | pass | ENGINE_DOWN | b790361fd8dd |
| domain-my-listings-filter | team-domain | done | 1 | pass | pass | pass | ENGINE_DOWN | 594ca0b3e4b9 |
| domain-bundle-integrity | team-domain | done | 1 | pass | pass | pass | ENGINE_DOWN | cf9bc98fe270 |
| seller-shop-customisation-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | b5a0802622cc |
| shop-page-featured-theme | team-frontend | done | 1 | pass | pass | pass | ENGINE_DOWN | 670decbcf356 |
| promotion-seller-self-service | team-promotion | done | 1 | pass | pass | pass | ENGINE_DOWN | 6f7920975566 |
| seller-marketing-hub | team-frontend-seller | done | 3 | pass | pass | pass | ENGINE_DOWN | 489cfc0c1a8d |
| seller-getting-started | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | fa2485d9f803 |
| order-bundle-checkout | team-order | done | 1 | pass | pass | pass | ENGINE_DOWN | 0c3095790822 |
| shop-bundle-purchase-ui | team-frontend | done | 1 | pass | pass | pass | ENGINE_DOWN | b4e35900d5a9 |
| order-seller-list-paging | team-order | done | 1 | pass | pass | pass | ENGINE_DOWN | a6ca5ff27811 |
| seller-server-paging-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | 5c12a87c8f08 |
| seller-bulk-ship | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN | 34c482424858 |

decide ENGINE_DOWN: decide.py needs `--units`/`--out`; default python3 lacks `yaml`. Reviewers used their own checklists.

## Cross-change check
Command: `platform-e2e/.venv/bin/python scripts/asdlc/cross_change_check.py --slices plans/seller-centre/slices.yaml` → exit 1, 12 violations (1 schema, 11 ownership). Details in plans/seller-centre/LOCK.md.
Build order: seller-centre-contract -> domain-bundle-integrity -> domain-my-listings-filter -> domain-storefront-customisation -> order-bundle-checkout -> order-seller-list-paging -> promotion-seller-self-service -> seller-marketing-hub -> seller-server-paging-ui -> seller-bulk-ship -> seller-shop-customisation-ui -> seller-getting-started -> shop-page-featured-theme -> shop-bundle-purchase-ui

## Next
- Fix the 12 cross-check violations (add `initiative: seller-centre` to domain-my-listings-filter/proposal.md; rename duplicate scenario titles), re-run the check, recompute digests in LOCK.md.
- Human answers the NEEDS_HUMAN questions in LOCK.md, then sets status/approved-by/date and ticks approve per slice.
- Then initiative-build.
