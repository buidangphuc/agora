## ADDED Requirements

### Requirement: FlashSaleService exposes service-only stock hold RPCs

`FlashSaleService` in `platform/promotion/v1/promotion.proto` SHALL gain `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` with request/response messages `ReserveFlashSaleStockRequest {reservation_id=1, campaign_id=2, listing_id=3, variant_id=4, quantity=5 (int64), expected_sale_price=6 (int64)}`, `ReserveFlashSaleStockResponse {reserved=1 (bool), reason=2}`, `CommitFlashSaleStockRequest {reservation_id=1}`, `CommitFlashSaleStockResponse {committed=1}`, `ReleaseFlashSaleStockRequest {reservation_id=1}`, `ReleaseFlashSaleStockResponse {released=1}`. Existing RPCs and messages SHALL NOT change. The RPC comments SHALL state that the RPCs are service-only (team-order's service principal with scope `promotion.reserve`, not routed through the gateway), idempotent, and describe the hold lifecycle: an uncommitted hold expires 15 minutes after reserve, Commit is called in checkout Phase B, and Release also returns committed units (`stock_sold -= quantity`).

#### Scenario: Flash stock contract: Hold RPCs exist on FlashSaleService
- **WHEN** the compiled `FlashSaleService` descriptor is inspected
- **THEN** it lists the existing four RPCs unchanged plus `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` with the request and response types above
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/promotion/v1/flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Message fields have fixed names and numbers
- **WHEN** the descriptor test reads the six new messages
- **THEN** every field name, number and type equals the list above, and no other field exists
- **VERIFIED BY**: Go descriptor test `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Existing messages are untouched
- **WHEN** the descriptor test compares `FlashSaleCampaign`, `GetActiveFlashSaleRequest/Response` and `GetFlashSaleStockRequest/Response` with the committed expectations
- **THEN** all field names and numbers match (`GetFlashSaleStockResponse` is `remaining=1`, `stock_cap=2`) and none is removed or renumbered
- **VERIFIED BY**: Go descriptor test `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Hold messages round-trip
- **WHEN** a `ReserveFlashSaleStockRequest` with `reservation_id` `ord-1:lst-1:` (empty variant), `quantity` 2 and `expected_sale_price` 199000, and a `ReserveFlashSaleStockResponse` with `reserved=false, reason=insufficient_stock`, are serialised and parsed
- **THEN** every field parses back equal, including the empty `variant_id`
- **VERIFIED BY**: Go round-trip test `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Maximum quantity and price round-trip
- **WHEN** a `ReserveFlashSaleStockRequest` with `quantity` and `expected_sale_price` both 9223372036854775807 is serialised and parsed
- **THEN** both int64 values parse back unchanged (no float or narrowing)
- **VERIFIED BY**: Go round-trip test `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Old readers skip the new request fields
- **WHEN** bytes of a `ReserveFlashSaleStockRequest` are parsed with a schema that knows only fields 1 to 3
- **THEN** parsing succeeds and fields 4 to 6 are skipped without error
- **VERIFIED BY**: Go test `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Service-only is documented
- **WHEN** a reader opens the comments of the three new RPCs
- **THEN** each contains "SERVICE-ONLY", the Reserve comment contains "Idempotent on reservation_id", Release states that it also returns committed units (`stock_sold -= quantity`) and that an unknown id returns `released=false`, and Commit states that a released or expired hold returns `committed=false`
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Missing service-only note is caught
- **WHEN** the test's comment helper runs against a fixture RPC comment lacking "SERVICE-ONLY"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Non-service callers are documented as denied
- **WHEN** a reader opens the comments of the three new RPCs
- **THEN** each states that a caller that is not team-order's service principal with scope `promotion.reserve` (a browser, a buyer or seller token) gets `PERMISSION_DENIED`, that a call with no principal gets `UNAUTHENTICATED`, and that no gateway route exists
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1); enforcement is tested end to end in promotion-flash-stock-hold

### Requirement: Reserve refusal reasons are a closed documented set

`ReserveFlashSaleStockResponse.reason` SHALL be empty when `reserved` is true and otherwise exactly one of `not_active`, `price_changed`, `insufficient_stock`. The contract comments SHALL define each value and SHALL state that malformed input (empty ids, `quantity <= 0`) is `INVALID_ARGUMENT`, not a reason. The `reservation_id` comment SHALL state the shape `<order_id>:<listing_id>:<variant_id>`, one hold per order line, non-empty, at most 200 bytes. The `GetFlashSaleStockResponse.remaining` comment SHALL state `stock_cap - stock_sold - active (unexpired, uncommitted) holds`.

#### Scenario: Flash stock contract: Hold lifecycle is documented
- **WHEN** a reader opens the comments of the three new RPCs
- **THEN** they state that an uncommitted hold expires 15 minutes after reserve, that Commit is called in checkout Phase B (not on OrderPaid), that Release of a committed hold returns the units (`stock_sold -= quantity`), that a Release of an already released hold, or of an expired uncommitted hold, returns `released=true` and changes no counter while only an unknown id returns `released=false`, and that Commit of a released or expired hold returns `committed=false`
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Reserve replay after release
- **WHEN** a reader opens the `ReserveFlashSaleStock` comment
- **THEN** it states "A replay returns the stored outcome and ignores the other request fields." and that a Reserve replay of a released or expired hold returns `reserved=false` with `reason=not_active`, and that callers use fresh order ids on every checkout attempt
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Reasons are documented as a closed set
- **WHEN** a reader opens the comments of `ReserveFlashSaleStock` and `ReserveFlashSaleStockResponse.reason`
- **THEN** the text contains each of `not_active`, `price_changed`, `insufficient_stock` with its meaning (inactive or unknown campaign, price differs from `expected_sale_price`, `cap - sold - holds < quantity`) states the set is closed, and states the checks run in the order `not_active`, `price_changed`, `insufficient_stock` with the first failure winning
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Invalid input is not a reason
- **WHEN** a reader opens the `ReserveFlashSaleStock` comment
- **THEN** it states that an empty `reservation_id`, `campaign_id` or `listing_id`, a `reservation_id` over 200 bytes, or `quantity <= 0` return `INVALID_ARGUMENT` and carry no `reason`, and that an empty `variant_id` is valid
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Reservation id shape is documented
- **WHEN** a reader opens the `reservation_id` comment of `ReserveFlashSaleStockRequest`
- **THEN** it contains `<order_id>:<listing_id>:<variant_id>`, the empty-variant form `<order_id>:<listing_id>:` and the 200-byte limit
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

#### Scenario: Flash stock contract: Remaining meter definition is documented
- **WHEN** a reader opens the `GetFlashSaleStockResponse.remaining` comment
- **THEN** it contains `stock_cap - stock_sold - active (unexpired, uncommitted) holds`
- **VERIFIED BY**: comment-text assertion in `flashstock_compat_test.go` (tasks 2.1)

### Requirement: The contract change is backward compatible and lints

The change SHALL pass `buf lint` and `buf breaking` against the committed base, and the generated code SHALL expose the new RPCs and messages in every generated language.

#### Scenario: Flash stock contract: Change is backward compatible
- **WHEN** `buf breaking` against the committed base (main / the stacked base) runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking` (tasks 1.2)

#### Scenario: Flash stock contract: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.2)

#### Scenario: Flash stock contract: Wire-breaking edit is rejected
- **WHEN** a mutated copy renumbers or removes any new field or RPC
- **THEN** `buf breaking` exits non-zero on the mutated copy, and the committed descriptor test, which pins every number, fails on the same edit
- **VERIFIED BY**: `make -C platform-core breaking` run on a mutated copy plus `flashstock_compat_test.go` (tasks 1.2, 2.1)

#### Scenario: Flash stock contract: New RPCs in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

### Requirement: Promotion proto copies are identical across repos

The updated `promotion.proto` SHALL be copied byte-identical to `proto/platform/promotion/v1/promotion.proto` in team-promotion, team-order, team-gateway, team-analytics and team-frontend. Consumers SHALL NOT edit their copy, and each consumer slice SHALL re-run the `cmp` check before it is done.

#### Scenario: Flash stock contract: Copies match the source
- **WHEN** each of the five copies is compared with `platform-core/packages/proto/platform/promotion/v1/promotion.proto`
- **THEN** all five comparisons report identical content (exit 0) and each copy contains the three new RPCs
- **VERIFIED BY**: one-off slice-done `cmp` of the five copies against the source (tasks 2.3)

#### Scenario: Flash stock contract: Drifted or missing copy is rejected
- **WHEN** any consumer copy differs from the source by one byte, or the file is missing
- **THEN** the `cmp` check exits non-zero (a missing file is a failure) and the contract slice is not done
- **VERIFIED BY**: one-off slice-done `cmp` of the five copies against the source (tasks 2.3)

#### Scenario: Flash stock contract: Consumer copies lint
- **WHEN** `buf lint` runs in the `proto` directory of each of the five copy owners
- **THEN** each exits 0
- **VERIFIED BY**: `buf lint` per consumer proto directory (tasks 2.3)
