## 1. Code - platform-core (proto first)

- [x] 1.1 In `packages/proto/platform/listing/v1/listing.proto` add `string display_name = 7` to `Storefront`, the `BatchGetStorefronts` RPC and `ShopSummary`, `BatchGetStorefrontsRequest`, `BatchGetStorefrontsResponse` (additive only, no renumbering); verify `cd packages/proto && buf lint` is clean
- [x] 1.2 Check compatibility with the baseline; verify `make -C platform-core breaking` (`buf breaking --against .git#ref=HEAD,subdir=packages/proto`) passes
- [x] 1.3 Regenerate stubs (`buf generate`) and publish to consumers per ADR-0001 without hand edits; verify `git diff` shows only generated files changed and the three consumers below build against them
  - Note: vendored listing.proto into team-domain, team-gateway, team-frontend and team-analytics (copies identical to base). Other consumers (team-ai, -payment, -engagement, -chat, -order, -notification, -identity, -search) hold older divergent copies and were left alone; they are additive-compatible and can re-vendor on their next sync.

## 2. Code - team-domain (owner)

- [x] 2.1 Regenerate `generated/` from the new proto; verify `go build ./...` and that no generated file was hand-edited
- [x] 2.2 Add `DisplayName` to `repository.Storefront`, persist it in the `config` JSONB (in-memory and Postgres repos, no migration); verify repository tests round-trip the name
- [x] 2.3 Validate in `StorefrontService.Upsert` (trim, 1-80 runes when non-empty, reject control chars -> `InvalidArgument`; seller id still forced to the principal); verify table tests for trim, too-long, control char, empty and spoofed `seller_id`
- [x] 2.4 Map `display_name` in `storefrontFromWire`/`storefrontToWire` so `GetStorefront` returns it; verify handler test "name returned for a storefront"
- [x] 2.5 Add `GetBySellers(ctx, ids)` to `StorefrontRepository` (`WHERE seller_id = ANY($1)`, one query) and the `BatchGetStorefronts` handler (dedupe, omit unknown, >100 ids -> `InvalidArgument`); verify tests for two sellers, unknown seller omitted, duplicates and 101 ids, and that the Postgres repo issues a single query
  - Note: no live Postgres here; "single query" verified by construction (one `WHERE seller_id = ANY($1)`) plus a handler test asserting exactly one repo call.
- [x] 2.6 Add `listing.shop-display-name` to `team-domain/FEATURES.yaml` (acceptance lines = the spec scenarios, `status: planned`); verify `make -C platform-e2e features-check` parses it
- [x] 2.7 Run the service gates; verify `go vet ./... && go test ./...` pass

## 3. Code - team-gateway

- [x] 3.1 Regenerate `generated/` and add the `BatchGetStorefronts` forwarder to `internal/edge/listing.go` using `callRead` (pass-through, no composition); verify `go build ./...`
- [x] 3.2 Add a forwarder test that the request reaches the upstream unchanged and the principal metadata is forwarded; verify `go test ./internal/edge/...`
- [x] 3.3 Run the gateway gates; verify `go vet ./... && go test ./...` pass

## 4. Code - team-frontend gateway wrappers

- [ ] 4.1 Regenerate `src/generated` (never hand-edit); verify `npx tsc --noEmit`
- [ ] 4.2 Add `src/lib/gateway/shops.ts`: `batchGetShopNames(sellerIds): Map<string,string>` (dedupe, chunk 100, one call per chunk, swallow errors incl. `Unimplemented` -> empty map) and pure `shopLabel(sellerId, displayName)`; verify Vitest cases for fallback "Shop #abc123", short id, empty name, dedupe, one call for two sellers and error swallowed
- [ ] 4.3 `listings.ts`: add `displayName` to `ViewStorefront`, map it in `getStorefront`, pass it in `upsertStorefront`; verify `listings.test.ts`
- [ ] 4.4 `cart.ts`: add `sellerDisplayName` to `ViewCartItem` and resolve names with one batch call after `getCart()`; verify `cart.test.ts` "two sellers -> exactly one batch call" and "batch failure -> items still returned"
- [ ] 4.5 `engagement.ts`: change `listFollowedSellers()` to return `{ sellerId, displayName }[]` (one batch call) and update the `/account/following` caller and `follow.test.ts`; verify `npx vitest run src/lib/gateway`
- [ ] 4.6 Hand the label to the UI phases: use `shopLabel()` in the existing shop header, cart group header and following list call sites that currently print `Shop #...` (remove ad-hoc strings only; no layout work, which stays in the `ui-phase-*` changes); verify `grep -rn "Shop #" src` finds only `shops.ts`
- [ ] 4.7 Run the frontend gates; verify `npx tsc --noEmit && npx vitest run && npm run lint`

## 5. E2E - platform-e2e + FEATURES.yaml

- [ ] 5.1 Update `team-frontend/FEATURES.yaml` (`shop.storefront` acceptance and new `shop.display-name-fallback`, `cart.shop-names`, `following.shop-names`) and `team-domain/FEATURES.yaml` (task 2.6) so each spec `#### Scenario:` maps 1:1 to an `acceptance` line; verify `make -C platform-e2e features-check`
- [ ] 5.2 Add `platform-e2e/tests/e2e/features/shop/shop_display_name.feature` with scenarios: name returned for a storefront, name change reflected, cart with two sellers shows both names, unknown seller falls back to "Shop #<6 chars>"; verify `--collect-only` finds all four
- [ ] 5.3 Add steps and page-object locators (seed a storefront with a name through the API using a seller token, seed a cart with two sellers, read the cart group headers); verify steps are resolved with no undefined-step errors
- [ ] 5.4 Run against the local stack and flip features to `status: automated` with `covered_by`; verify `make -C platform-e2e` shop tests pass and `make -C platform-e2e features-check` is green

## 6. Validation

- [ ] 6.1 Validate the change; verify `openspec validate shop-display-name --strict` reports valid
