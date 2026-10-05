## 1. Contract — platform-core

- [x] 1.1 Add `OrderShipped` to the order proto and, if needed, the chat recipient field or `MessageSent` event, additively; verify `buf lint` and `buf breaking` against feat/ui-system, vendor byte-identically to team-order, team-chat, team-notification and team-analytics, regenerate

## 2. Code — team-order

- [x] 2.1 `CreateShipment` writes the `OrderShipped` outbox row in the same transaction; verify a unit test for the row and a rollback test (Postgres test skips without `TEST_DATABASE_URL`), `go test ./...`

## 3. Code — team-chat

- [x] 3.1 The published message event carries the recipient (or participants); verify a publisher test and `go test ./...`

## 4. Code — team-notification

- [x] 4.1 `chat.events` consumer: CHAT notification for the non-sender, dedupe, prefs; verify unit tests (recipient, sender not notified, redelivery once, disabled pref skips)
- [x] 4.2 `order.events` consumer: ORDER notification on `OrderShipped` for the buyer, ignore other types, dedupe, prefs; verify unit tests including an `OrderPaidEvent` being ignored
- [x] 4.3 Wire both consumers in main with env (topics, groups) and compose; verify the env-example sync test and `go build ./...`

## 5. Code — team-analytics

- [x] 5.1 Confirm the order consumer skips `OrderShipped` and advances; verify a unit test

## 6. E2E — platform-e2e

- [ ] 6.1 Remove the strict xfail markers from "Seller reply notifies the buyer" and "Shipping the order notifies the buyer", add a chat-pref-disabled scenario; verify green against the agora stack (3 runs) and FEATURES entries `automated`
- [ ] 6.2 Run `openspec validate notify-chat-and-shipment --strict`; verify it is valid
