# [W3-T4] e2e — price-drop / back-in-stock alerts

## Role
QA

## Objective
Automated e2e proving: user subscribes to a price-drop alert on a listing; when the seller lowers the price, a notification appears. Same for back-in-stock. Flip notification-alerts status to automated.

## Write-set (EXCLUSIVE)
- platform-e2e/tests/e2e/features/notification/**                 (edit/create — alerts .feature)
- platform-e2e/tests/e2e/step_definitions/notification_alert_steps.py (create — alert steps)
- platform-e2e/src/pages/notifications_page.py                    (edit)
- team-notification/FEATURES.yaml                                 (edit — status: automated)

## Read-only dependencies
- existing notifications_page + notification feature dir
- Wave 0 slot locator (AlertToggle) on listing_detail_page
- 00-spec.md §Contracts F3 alerts

## Contracts / scenarios
- Given buyer subscribed to price-drop on a listing, When seller lowers its price, Then a price-drop notification appears in the center.
- Given buyer subscribed to back-in-stock on an out-of-stock listing, When it restocks, Then a back-in-stock notification appears.

## Acceptance criteria
- [ ] Scenarios pass against local stack (allow for async event propagation with a bounded wait/poll).
- [ ] Steps wired (collect green); team-notification/FEATURES.yaml flipped to automated.

## Review (gate — different agent)
Peer agent against QA rubric.

## Verify
```bash
docker compose -p platform-core up -d
make -C platform-e2e collect
python -m pytest platform-e2e/tests/e2e -k "alert or notification" -q
```

## Out of scope
- No other feature dirs/step files. Do not edit shared slot locators (Wave 0).
