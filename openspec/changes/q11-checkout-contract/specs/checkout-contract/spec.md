## ADDED Requirements

### Requirement: OrderService exposes a read-only checkout quote

`OrderService` in `platform/order/v1/order.proto` SHALL gain `PreviewCheckout(PreviewCheckoutRequest) returns (PreviewCheckoutResponse)` with messages `ShopVoucher {seller_id=1, code=2}`, `PreviewCheckoutRequest {address_id=1, item_ids=2 (repeated), shop_vouchers=3 (repeated ShopVoucher), platform_voucher_code=4}`, `CheckoutGroupQuote {seller_id=1, item_ids=2 (repeated), items_subtotal=3, shipping_fee=4, is_free_shipping=5, shop_voucher_code=6, shop_discount_amount=7, platform_discount_amount=8, total_amount=9, estimated_delivery_at=10 (Timestamp)}`, `VoucherQuote {code=1, scope=2 (CheckoutVoucherScope), seller_id=3, valid=4, reason=5, discount_amount=6}` (where `CheckoutVoucherScope` is a local enum in `order.proto`: `CHECKOUT_VOUCHER_SCOPE_UNSPECIFIED = 0`, `CHECKOUT_VOUCHER_SCOPE_SHOP = 1`, `CHECKOUT_VOUCHER_SCOPE_PLATFORM = 2`, the same numbers as `platform.promotion.v1.VoucherScope`; `order.proto` SHALL NOT import `promotion.proto`) and `PreviewCheckoutResponse {groups=1 (repeated), vouchers=2 (repeated), items_subtotal=3, shipping_fee=4, discount_amount=5, total_amount=6, shipping_quoted=7 (bool), shipping_address=8 (platform.identity.v1.Address)}`; every amount is `int64`. The RPC comment SHALL state that it reserves nothing, that a rejected voucher is reported in `vouchers[]` and not raised, that `FAILED_PRECONDITION` ("cart is empty") is returned when `item_ids` match no cart line, that it acts only for the authenticated buyer's own cart and addresses, and that its figures are those `CreateOrder` places for the same request. Existing RPCs and messages SHALL NOT change.

#### Scenario: Checkout contract: PreviewCheckout exists on OrderService
- **WHEN** the compiled `OrderService` descriptor is inspected
- **THEN** it lists every existing RPC unchanged plus `PreviewCheckout` with input `PreviewCheckoutRequest` and output `PreviewCheckoutResponse`
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/order/v1/checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Quote messages have fixed names, numbers and types
- **WHEN** the descriptor test reads `ShopVoucher`, `PreviewCheckoutRequest`, `CheckoutGroupQuote`, `VoucherQuote` and `PreviewCheckoutResponse`
- **THEN** every field name, number, type and repeated label equals the list above, `VoucherQuote.scope` is the local `platform.order.v1.CheckoutVoucherScope` with values UNSPECIFIED=0, SHOP=1, PLATFORM=2 (names and numbers matching `platform.promotion.v1.VoucherScope`), `order.proto` imports no `promotion.proto`, and no other field exists
- **VERIFIED BY**: Go descriptor test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Quote semantics are documented
- **WHEN** a reader opens the comments of `PreviewCheckout` and of the quote messages
- **THEN** they state: reserves nothing; rejected voucher reported not raised; `FAILED_PRECONDITION` "cart is empty" when `item_ids` match no cart line; groups sorted by `seller_id`, one per order `CreateOrder` places; `total_amount = items_subtotal + shipping_fee - shop_discount_amount - platform_discount_amount`, clamped at 0; free shipping decided per group; `estimated_delivery_at` unset when `shipping_quoted` is false; only the authenticated buyer's own cart and addresses; address fields are never logged
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Missing quote semantics are caught
- **WHEN** the test's comment helper runs against a fixture `PreviewCheckout` comment lacking "reserves nothing"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Quote messages round-trip
- **WHEN** a `PreviewCheckoutResponse` with two groups (seller ids `s-a`, `s-b`; `items_subtotal` 300000 each; `shipping_fee` 35000 each; `platform_discount_amount` 10000 and 20000), one `VoucherQuote{scope=CHECKOUT_VOUCHER_SCOPE_PLATFORM, valid=false, reason="min spend not met"}`, `shipping_quoted=true` and a `shipping_address` is serialised and parsed
- **THEN** every field parses back equal, including the empty `VoucherQuote.seller_id` and the nested address
- **VERIFIED BY**: Go round-trip test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Maximum amounts round-trip
- **WHEN** a `CheckoutGroupQuote` with `items_subtotal`, `shipping_fee` and `total_amount` all 9223372036854775807 is serialised and parsed
- **THEN** all three int64 values parse back unchanged (no float or narrowing)
- **VERIFIED BY**: Go round-trip test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Quote of an empty selection is documented as an error
- **WHEN** a reader checks the `PreviewCheckout` comment for the case that `item_ids` match no cart line
- **THEN** it states gRPC `FAILED_PRECONDITION` with the message "cart is empty", not an empty `groups` list
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1); enforcement is tested end to end in order-multishop-checkout

#### Scenario: Checkout contract: Quote is documented as buyer-only
- **WHEN** a reader checks the `PreviewCheckout` comment for who may call it
- **THEN** it states that only the authenticated buyer's own cart and addresses are used, that the request carries no buyer id, and that a seller or service principal is denied
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1); enforcement is tested end to end in order-multishop-checkout

### Requirement: CreateOrder carries scoped vouchers and a confirmed address

`CreateOrderRequest` SHALL gain `repeated ShopVoucher shop_vouchers = 5`, `string platform_voucher_code = 6` and `platform.identity.v1.Address confirmed_address = 7`; fields 1-4 SHALL be unchanged and `voucher_code = 4` SHALL be documented as the platform-voucher slot only (treated like `platform_voucher_code`, no SHOP routing) and refused with `INVALID_ARGUMENT` when `platform_voucher_code` is also set or it repeats a `shop_vouchers` code. The comments SHALL document: at most one shop voucher per `seller_id`; a shop voucher discounts only its seller's group; the platform voucher is computed once on the selection subtotal, its `min_spend` is checked on the whole selection, the discount is split across groups proportionally to `items_subtotal` with integer division, the remainder to the largest group (ties: lowest `seller_id`); a shop voucher and a platform share stack on one group, whose total is clamped at 0; the platform reservation id is `platform:<checkout_id>`; a voucher in the wrong slot (a SHOP code in `voucher_code` or `platform_voucher_code`, a PLATFORM code in `shop_vouchers`) or applied to another shop's group makes `CreateOrder` fail `FAILED_PRECONDITION` with the promotion reason, and a second `shop_vouchers` entry for one `seller_id` fails `INVALID_ARGUMENT`, nothing staying held in either case; address resolution fails closed: empty `address_id` means the default address, and an unknown id, no address, or addresses with none marked default (no first-address fallback) gives `FAILED_PRECONDITION` "address not found", a `confirmed_address` that differs from identity's copy in any of recipient_name, phone, street, ward, district, city gives `FAILED_PRECONDITION` "address changed", identity unreachable gives `UNAVAILABLE`, and nothing is reserved in any of these cases. The comments SHALL also state that the new fields act only for the authenticated buyer (user principal, own cart and own addresses; the request carries no buyer id) and that a seller or service principal sending them is denied.

#### Scenario: Checkout contract: New CreateOrder fields are documented as buyer-only
- **WHEN** a reader checks the comments of `shop_vouchers`, `platform_voucher_code` and `confirmed_address` for who may send them
- **THEN** they state that only the authenticated buyer's own cart and addresses are used, that the request carries no buyer id, and that a seller or service principal is denied (`PERMISSION_DENIED`)
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1); enforcement is tested end to end in order-multishop-checkout

#### Scenario: Checkout contract: CreateOrderRequest fields have fixed numbers
- **WHEN** the descriptor test reads `CreateOrderRequest`
- **THEN** fields 1-4 are `address_id`, `item_ids`, `payment_method`, `voucher_code` unchanged and 5-7 are `shop_vouchers` (repeated `ShopVoucher`), `platform_voucher_code` (string) and `confirmed_address` (`platform.identity.v1.Address`)
- **VERIFIED BY**: Go descriptor test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Old readers skip the new request fields
- **WHEN** bytes of a `CreateOrderRequest` with fields 1-7 set are parsed with a schema that knows only fields 1 to 4
- **THEN** parsing succeeds, fields 1-4 are equal and fields 5 to 7 are skipped without error
- **VERIFIED BY**: Go test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Voucher routing is documented
- **WHEN** a reader opens the comments of `CreateOrderRequest.voucher_code`, `shop_vouchers` and `platform_voucher_code`
- **THEN** they state that `voucher_code` is the platform-voucher slot only, at most one shop voucher per `seller_id`, `min_spend` of the platform voucher on the whole selection, the integer-division split with the remainder to the largest group (ties lowest `seller_id`), stacking with a clamp at 0, the reservation id `platform:<checkout_id>`, and the codes: `FAILED_PRECONDITION` with the promotion reason for a voucher in the wrong slot or on another shop's group, `INVALID_ARGUMENT` for a duplicate `seller_id` or a `voucher_code` set with `platform_voucher_code` or repeating a `shop_vouchers` code
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Address failures are documented as fail closed
- **WHEN** a reader opens the comments of `CreateOrderRequest.address_id` and `confirmed_address`
- **THEN** they state that an empty `address_id` means the default address, `FAILED_PRECONDITION` "address not found" for an unknown id, no address, or no default with an empty `address_id` (no first-address fallback), `FAILED_PRECONDITION` "address changed" for a differing `confirmed_address` (fields recipient_name, phone, street, ward, district, city), `UNAVAILABLE` when identity is unreachable, and that nothing is reserved in any case
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1); enforcement is tested end to end in order-address-eta

#### Scenario: Checkout contract: Missing address-failure note is caught
- **WHEN** the test's comment helper runs against a fixture `confirmed_address` comment lacking "address changed"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: comment-text assertion in `checkout_compat_test.go` (tasks 2.1)

### Requirement: Shipping quote, order and voucher response carry the new fields

`CalculateShippingFeeResponse` SHALL gain `int32 estimated_delivery_days = 4` and `google.protobuf.Timestamp estimated_delivery_at = 5` (comment: computed by the server as now + `estimated_delivery_days`); `Order` SHALL gain `string checkout_id = 25`, `string platform_voucher_code = 26` and `int64 platform_discount_amount = 27` (comments: `checkout_id` is shared by every order of one checkout and equals the saga id; `platform_discount_amount` is part of `discount_amount`; `voucher_code` stays the shop voucher); `Order` fields 1-17 SHALL be unchanged and 18-24 SHALL NOT be used by this slice (reserved for q10-order-lifecycle); `platform.promotion.v1.ValidateAndReserveResponse` SHALL gain `VoucherScope scope = 5` and `string seller_id = 6` (the shop of a SHOP voucher, empty for PLATFORM) with fields 1-4 unchanged.

#### Scenario: Checkout contract: Shipping response fields are fixed
- **WHEN** the descriptor test reads `CalculateShippingFeeResponse`
- **THEN** fields 1-3 are `shipping_fee`, `is_free_shipping`, `message` unchanged and 4-5 are `estimated_delivery_days` (int32) and `estimated_delivery_at` (`google.protobuf.Timestamp`)
- **VERIFIED BY**: Go descriptor test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Order fields are fixed and 18-24 stay free
- **WHEN** the descriptor test reads `Order`
- **THEN** fields 1-17 are unchanged, 25-27 are `checkout_id` (string), `platform_voucher_code` (string) and `platform_discount_amount` (int64), and no field number between 18 and 24 is declared by this slice
- **VERIFIED BY**: Go descriptor test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Order round-trips and old readers skip the new fields
- **WHEN** an `Order` with `discount_amount` 30000, `platform_discount_amount` 10000, `checkout_id` `chk-1` and `platform_voucher_code` `PLAT10` is serialised, parsed, and parsed again with a schema that knows only fields 1 to 17
- **THEN** the full parse returns every field equal and the old-schema parse succeeds, skipping 25 to 27 without error
- **VERIFIED BY**: Go round-trip test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Voucher response reports scope and shop
- **WHEN** the descriptor test reads `platform.promotion.v1.ValidateAndReserveResponse`
- **THEN** fields 1-4 (`valid`, `reason`, `discount_amount`, `voucher_id`) are unchanged and 5-6 are `scope` (`VoucherScope`) and `seller_id` (string), and a response with `scope=VOUCHER_SCOPE_SHOP, seller_id=s-a` round-trips equal, and `CheckoutVoucherScope` has the same value names (after the prefix) and numbers as `VoucherScope`
- **VERIFIED BY**: Go descriptor and round-trip test `checkout_compat_test.go` (tasks 2.1)

#### Scenario: Checkout contract: Existing order messages are untouched
- **WHEN** the descriptor test compares the other `OrderService` messages (`OrderItem`, `GetOrderRequest`, `ListBuyerOrdersRequest`, `CancelOrderRequest`, `CalculateShippingFeeRequest`, `OrderPaidEvent`) with the committed expectations
- **THEN** all field names and numbers match and none is removed or renumbered
- **VERIFIED BY**: Go descriptor test `checkout_compat_test.go` (tasks 2.1)

### Requirement: The contract change is backward compatible and lints

The change SHALL pass `buf lint` and `buf breaking` against the committed base, and the generated code SHALL expose the new RPC and messages in every generated language.

#### Scenario: Checkout contract: Change is backward compatible
- **WHEN** `buf breaking` against the committed base (main / the stacked base) runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking` (tasks 1.4)

#### Scenario: Checkout contract: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.4)

#### Scenario: Checkout contract: Wire-breaking edit is rejected
- **WHEN** a mutated copy renumbers or removes any new field or RPC
- **THEN** `buf breaking` exits non-zero on the mutated copy, and the committed descriptor test, which pins every number, fails on the same edit
- **VERIFIED BY**: `make -C platform-core breaking` run on a mutated copy plus `checkout_compat_test.go` (tasks 1.4, 2.1)

#### Scenario: Checkout contract: New RPC and fields in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose `PreviewCheckout` and the fields `checkout_id`, `estimated_delivery_at` and `confirmed_address`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

### Requirement: Order and promotion proto copies are identical across repos

The updated `order.proto` SHALL be copied byte-identical to `proto/platform/order/v1/order.proto` in team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-gateway, team-notification, team-order, team-payment and team-search, and the updated `promotion.proto` SHALL be copied byte-identical to `proto/platform/promotion/v1/promotion.proto` in the five repos that already hold it (team-analytics, team-frontend, team-gateway, team-order, team-promotion); no repo gains a new proto file. Consumers SHALL NOT edit their copy, and each consumer slice SHALL re-run the `cmp` check before it is done.

#### Scenario: Checkout contract: Copies match the source
- **WHEN** each of the ten `order.proto` copies and five `promotion.proto` copies is compared with its `platform-core/packages/proto/platform/...` source
- **THEN** all comparisons report identical content (exit 0) and every `order.proto` copy contains `PreviewCheckout`
- **VERIFIED BY**: one-off slice-done `cmp` of all copies against the sources (tasks 2.3)

#### Scenario: Checkout contract: Drifted or missing copy is rejected
- **WHEN** any consumer copy differs from its source by one byte, or the file is missing
- **THEN** the `cmp` check exits non-zero (a missing file is a failure) and the contract slice is not done
- **VERIFIED BY**: one-off slice-done `cmp` of all copies against the sources (tasks 2.3)

#### Scenario: Checkout contract: Consumer copies build and lint
- **WHEN** `buf lint` and `buf build` run in the `proto` directory of each repo holding a copy
- **THEN** each exits 0, and no `order.proto` copy needs a `promotion.proto` beside it
- **VERIFIED BY**: `buf lint` and `buf build` per consumer proto directory (tasks 2.3)
