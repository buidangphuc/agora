## 1. Contract — platform-core

- [ ] 1.1 Add the identity `GetPublicProfiles` RPC and messages additively; verify `buf lint` and `buf breaking` against feat/ui-system, vendor to team-identity and team-notification, regenerate

## 2. Code — team-identity

- [ ] 2.1 Implement `GetPublicProfiles` for service principals only (users get PermissionDenied), returning display names, max 100 ids; verify unit tests and `go test ./...`

## 3. Code — team-chat

- [ ] 3.1 Migration for `chat_outbox_events`, outbox store with the ordered claim query and its Postgres ordering test; verify `go test ./...`
- [ ] 3.2 SendMessage writes the event to the outbox in the message transaction; relayer to `chat.events` started in main with `OUTBOX_*`/`KAFKA_*` env; verify a rollback test and the relayer unit tests

## 4. Code — team-notification

- [ ] 4.1 Migration and Postgres implementations of the dedupe ledger and listing last-seen stores, wired for all consumers; verify Postgres tests (skip without `TEST_DATABASE_URL`) and unit tests
- [ ] 4.2 Sender display-name resolution (team-domain storefront for the seller, identity profiles otherwise, timeout and fallback); verify unit tests for seller, buyer and lookup failure

## 5. E2E — platform-e2e

- [ ] 5.1 Scenarios: a seller reply names the shop; a price drop after a team-notification restart is detected (`@destructive`, restarts the container named by `NOTIFICATION_CONTAINER`); a message sent while chat's relayer cannot publish is delivered later if feasible without stopping shared infra; FEATURES entries; verify green against the agora stack (3 runs) and flip to `automated`
- [ ] 5.2 Run `openspec validate notification-delivery-hardening --strict`; verify it is valid
