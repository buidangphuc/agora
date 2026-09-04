# [W3-T1] e2e — voucher redemption + flash sale

## Role
QA   # pytest-bdd + Playwright; scenarios trace 00-spec

## Objective
Automated e2e proving: a buyer applies a voucher at checkout and pays the discounted total; a flash-sale listing shows sale price + live stock. Flip promotion feature status to automated.

## Write-set (EXCLUSIVE)
- platform-e2e/tests/e2e/features/promo/**                     (edit/create — voucher + flash-sale .feature)
- platform-e2e/tests/e2e/step_definitions/promo_steps.py       (edit)
- platform-e2e/src/pages/vouchers_page.py                      (edit)
- team-promotion/FEATURES.yaml                                 (edit — status: automated)

## Read-only dependencies
- platform-e2e/src/pages/{listing_detail_page,checkout_page}.py (Wave 0 added slot locators)
- 00-spec.md §Contracts F1; tag-driven seeding conventions (existing promo_steps)

## Contracts / scenarios
- Given a shop voucher X% off min-spend M, When buyer with cart ≥ M applies it at checkout, Then order total = subtotal + shipping − discount and order is created.
- Given an expired/invalid voucher, When applied, Then the reason is shown and (per policy) checkout blocks.
- Given an active flash-sale campaign, When buyer views the listing, Then sale price + remaining-stock meter render.

## Acceptance criteria
- [ ] Feature file scenarios pass against the local stack (`make -C platform-e2e ...`).
- [ ] `make -C platform-e2e collect` shows all steps wired (no missing step defs).
- [ ] team-promotion/FEATURES.yaml entry flipped to automated with the scenario ids.

## Review (gate — different agent)
Peer agent against QA rubric (experts.md).

## Verify
```bash
docker compose -p platform-core up -d   # full stack must be up first
make -C platform-e2e collect
python -m pytest platform-e2e/tests/e2e -k promo -q
```

## Out of scope
- Do NOT edit shared page objects' slot locators (Wave 0). Do NOT touch other features' feature dirs/step files.
