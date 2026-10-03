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

- [x] 4.1 Regenerate `src/generated` (never hand-edit); verify `npx tsc --noEmit`
- [x] 4.2 Add `src/lib/gateway/shops.ts`: `batchGetShopNames(sellerIds): Map<string,string>` (dedupe, chunk 100, one call per chunk, swallow errors incl. `Unimplemented` -> empty map) and pure `shopLabel(sellerId, displayName)`; verify Vitest cases for fallback "Shop #abc123", short id, empty name, dedupe, one call for two sellers and error swallowed
- [x] 4.3 `listings.ts`: add `displayName` to `ViewStorefront`, map it in `getStorefront`, pass it in `upsertStorefront`; verify `listings.test.ts`
- [x] 4.4 `cart.ts`: add `sellerDisplayName` to `ViewCartItem` and resolve names with one batch call after `getCart()`; verify `cart.test.ts` "two sellers -> exactly one batch call" and "batch failure -> items still returned"
- [x] 4.5 `engagement.ts`: change `listFollowedSellers()` to return `{ sellerId, displayName }[]` (one batch call) and update the `/account/following` caller and `follow.test.ts`; verify `npx vitest run src/lib/gateway`
- [x] 4.6 Hand the label to the UI phases: use `shopLabel()` in the existing shop header, cart group header and following list call sites that currently print `Shop #...` (remove ad-hoc strings only; no layout work, which stays in the `ui-phase-*` changes); verify `grep -rn "Shop #" src` finds only `shops.ts`
  - Note: the cart view has no per-shop group header yet (flat list), so only `sellerDisplayName` is exposed there; the header itself belongs to the `ui-phase-*` changes. PDP header, `/shop/<id>` header and following list now use `shopLabel()`.
- [ ] 4.7 Run the frontend gates; verify `npx tsc --noEmit && npx vitest run && npm run lint`
  - Not ticked: gates are red on the base branch independent of this change (`tsc`: 10 errors in CheckoutView, AddToCartButton, RecommendationsRow, SearchImpressions, track.test; `vitest`: 5 failures in `src/lib/track.test.ts`; `biome check`: 44 errors). This change adds none: tsc count unchanged (10), vitest failures unchanged (5), all 14 gateway test files pass, files it touched have no new biome findings.

## 5. E2E - platform-e2e + FEATURES.yaml

- [x] 5.1 Update `team-frontend/FEATURES.yaml` (`shop.storefront` acceptance and new `shop.display-name-fallback`, `cart.shop-names`, `following.shop-names`) and `team-domain/FEATURES.yaml` (task 2.6) so each spec `#### Scenario:` maps 1:1 to an `acceptance` line; verify `make -C platform-e2e features-check`
- [x] 5.2 Add `platform-e2e/tests/e2e/features/shop/shop_display_name.feature` with scenarios: name returned for a storefront, name change reflected, cart with two sellers shows both names, unknown seller falls back to "Shop #<6 chars>"; verify `--collect-only` finds all four
- [x] 5.3 Add steps and page-object locators (seed a storefront with a name through the API using a seller token, seed a cart with two sellers, read the cart group headers); verify steps are resolved with no undefined-step errors
  - Note: verified with `pytest --collect-only` (4 scenarios) plus a step-resolution check (0 undefined steps). The cart scenario asserts the BatchGetStorefronts lookup via the gateway because the cart view has no per-shop group header yet.
- [ ] 5.4 Run against the local stack and flip features to `status: automated` with `covered_by`; verify `make -C platform-e2e` shop tests pass and `make -C platform-e2e features-check` is green
  - Not ticked: the agora stack was not running and containers must not be started from this task. Run `make -C platform-e2e` shop tests against a live stack, then flip `listing.shop-display-name`, `shop.display-name-fallback`, `cart.shop-names` to `status: automated` with `covered_by`; `following.shop-names` still needs its own scenario.

## 6. Validation

- [x] 6.1 Validate the change; verify `openspec validate shop-display-name --strict` reports valid
