# [W1-T5] team-notification — price-drop / back-in-stock alerts

## Role
SE   # Kafka consumer + subscription store

## Objective
NotificationService gains alert subscriptions (Subscribe/Unsubscribe/List). A consumer of `listing.events` matches pricing-changed (price drop) and stock-changed (0→>0) events to subscriptions and creates notifications.

## Write-set (EXCLUSIVE)
- team-notification/internal/handler/alerts.go        (create + _test.go)
- team-notification/internal/service/alerts.go        (create + _test.go)
- team-notification/internal/repository/alerts.go     (create — Postgres + in-memory + _test.go)
- team-notification/internal/consumer/listing.go      (create — consume listing.events → match → notify + _test.go)
- team-notification/migrations/002_alert_subscriptions.up.sql / .down.sql (create)

## Read-only dependencies
- team-notification generated stubs (Wave 0), existing notification handler/consumer pattern
- platform-core listing.events schema (pricing/stock changed)
- 00-spec.md §Contracts F3 alerts

## Contracts
Subscription = (user_id, listing_id, AlertType). Consumer: on `PricingChanged` where new<old and a PRICE_DROP subscription exists → create NOTIFICATION_TYPE_PRICE_DROP; on `StockChanged` 0→positive with a BACK_IN_STOCK subscription → NOTIFICATION_TYPE_BACK_IN_STOCK. Commit Kafka offset only after handler success; poison → `<topic>.dlq` (mirror existing consumer). Idempotent: don't double-notify for the same event.

## Acceptance criteria
- [ ] Subscribe/Unsubscribe/List happy path; unsubscribe idempotent.
- [ ] Price-drop event with a matching subscription creates exactly one notification; no match → none.
- [ ] Back-in-stock only fires on 0→positive transition, not positive→positive.
- [ ] Offset committed only after success; malformed event routed to DLQ, not crash-loop.
- [ ] gofmt/go vet/go test clean; consumer tested with fabricated events.

## Review (gate — different agent)
Route to **contract-boundary-reviewer** (Kafka consumer, offset/DLQ discipline). Rubric = SE.

## Verify
```bash
# in Docker: (cd team-notification && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT edit proto/generated/main.go/grpcserver (Wave 0). No email/push channel (in-app only for now). No engagement changes. No gateway/frontend.
