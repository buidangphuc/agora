# [W3-T3] e2e — wishlist collections + richer reviews

## Role
QA

## Objective
Automated e2e proving: user creates a collection and adds a listing; a review with a photo + helpful vote + verified badge renders; shop rating summary shows. Flip engagement status to automated.

## Write-set (EXCLUSIVE)
- platform-e2e/tests/e2e/features/engagement/**                    (edit/create — collections + rich_reviews .feature)
- platform-e2e/tests/e2e/step_definitions/engagement_steps.py     (edit)
- platform-e2e/tests/e2e/step_definitions/review_ratings_steps.py (edit)
- platform-e2e/src/pages/favorites_page.py                        (edit)
- team-engagement/FEATURES.yaml                                   (edit — status: automated)

## Read-only dependencies
- existing engagement_steps / review_ratings_steps / favorites_page
- Wave 0 slot locators on listing_detail_page (reviews, wishlist button)
- 00-spec.md §Contracts F3 + F4

## Contracts / scenarios
- Create named collection, add/remove a listing, view items.
- Post a review with a photo; mark another user's review helpful (count increments once); verified-purchase badge shows for a delivered order.
- Shop page shows aggregate rating summary.

## Acceptance criteria
- [ ] Scenarios pass against local stack; steps wired (collect green).
- [ ] team-engagement/FEATURES.yaml flipped to automated.

## Review (gate — different agent)
Peer agent against QA rubric.

## Verify
```bash
docker compose -p platform-core up -d
make -C platform-e2e collect
python -m pytest platform-e2e/tests/e2e -k "engagement or review or collection" -q
```

## Out of scope
- No other feature dirs/step files. Do not edit shared slot locators (Wave 0).
