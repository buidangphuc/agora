## ADDED Requirements

### Requirement: CalculateShippingFee returns an estimated delivery date

`OrderService.CalculateShippingFee` SHALL return `estimated_delivery_days` and `estimated_delivery_at` for a non-empty city: 2 days when the city is Hồ Chí Minh or Hà Nội (the same city match as the fee rule), 4 days for any other city. `estimated_delivery_at` SHALL equal the service clock now plus `estimated_delivery_days` x 24 hours. For an empty city it SHALL return `estimated_delivery_days` 0 and leave `estimated_delivery_at` unset. The fee, `is_free_shipping` and `message` fields SHALL keep their existing values.

#### Scenario: Address and ETA: Hồ Chí Minh is delivered in 2 days
- **WHEN** the clock is 2026-10-16T05:00:00Z and `CalculateShippingFee` is called with city "Hồ Chí Minh" and items_subtotal 300000
- **THEN** `estimated_delivery_days` is 2 and `estimated_delivery_at` is 2026-10-18T05:00:00Z
- **VERIFIED BY**: team-order `internal/service` `TestShippingETAHoChiMinhTwoDays`

#### Scenario: Address and ETA: Hà Nội is delivered in 2 days
- **WHEN** the clock is 2026-10-16T05:00:00Z and `CalculateShippingFee` is called with city "Hà Nội" and items_subtotal 300000
- **THEN** `estimated_delivery_days` is 2 and `estimated_delivery_at` is 2026-10-18T05:00:00Z
- **VERIFIED BY**: team-order `internal/service` `TestShippingETAHaNoiTwoDays`

#### Scenario: Address and ETA: Any other city is delivered in 4 days
- **WHEN** the clock is 2026-10-16T05:00:00Z and `CalculateShippingFee` is called with city "Đà Nẵng" and items_subtotal 300000
- **THEN** `estimated_delivery_days` is 4 and `estimated_delivery_at` is 2026-10-20T05:00:00Z
- **VERIFIED BY**: team-order `internal/service` `TestShippingETAOtherCityFourDays`

#### Scenario: Address and ETA: The delivery date is a calendar date in Asia/Ho_Chi_Minh
- **WHEN** the clock is 2026-10-16T18:00:00Z (already 17/10 01:00 in Asia/Ho_Chi_Minh) and `CalculateShippingFee` is called with city "Đà Nẵng"
- **THEN** `estimated_delivery_at` formatted in Asia/Ho_Chi_Minh is the date 21/10/2026
- **VERIFIED BY**: team-order `internal/service` `TestShippingETAIsHoChiMinhCalendarDate`

#### Scenario: Address and ETA: Free shipping does not change the delivery time
- **WHEN** `CalculateShippingFee` is called with city "Đà Nẵng" and items_subtotal 600000
- **THEN** `shipping_fee` is 0, `is_free_shipping` is true and `estimated_delivery_days` is 4
- **VERIFIED BY**: team-order `internal/service` `TestShippingETAIndependentOfFreeShipping`

#### Scenario: Address and ETA: An empty city gets no delivery date
- **WHEN** `CalculateShippingFee` is called with an empty city
- **THEN** `estimated_delivery_days` is 0 and `estimated_delivery_at` is unset
- **VERIFIED BY**: team-order `internal/handler` `TestCalculateShippingFeeEmptyCityNoETA`

#### Scenario: Address and ETA: Fee amounts are unchanged
- **WHEN** `CalculateShippingFee` is called with city "Hà Nội" and items_subtotal 200000, then city "Đà Nẵng" and items_subtotal 200000, then city "Đà Nẵng" and items_subtotal 500000
- **THEN** the shipping fees are 20000, 35000 and 0 and `is_free_shipping` is false, false and true
- **VERIFIED BY**: team-order `internal/service` `TestShippingFeeAmountsUnchanged` (new; includes the Đà Nẵng 500000 free-shipping case the existing `TestOrderService_CalculateShippingFee` lacks)

#### Scenario: Address and ETA: The delivery date is quoted through the gateway
- **WHEN** a signed-in buyer calls `CalculateShippingFee` through the gateway with city "Đà Nẵng" and items_subtotal 300000
- **THEN** the response has `shipping_fee` 35000 and an `estimated_delivery_at` that is 4 calendar days after today in Asia/Ho_Chi_Minh
- **VERIFIED BY**: platform-e2e `features/order/address_eta.feature` "Shipping quote carries a delivery date"

### Requirement: CreateOrder resolves the shipping address fail closed

`OrderService.CreateOrder` SHALL resolve the shipping address from the calling buyer's own addresses (`AddressService.ListAddresses` with the forwarded principal) before reserving anything: the address whose id equals `address_id`, or the buyer's default address when `address_id` is empty. It SHALL fail with `FAILED_PRECONDITION` "address not found" when no such address exists, the buyer has none, or the resolved address has an empty `city` (after trimming spaces), and with `UNAVAILABLE` when the address service errors or no address client is configured. In every failure case it SHALL NOT reserve stock, hold a voucher, create a saga or change the cart. The order SHALL store the resolved address as its delivery snapshot.

#### Scenario: Address and ETA: The picked address is the order's address
- **WHEN** a buyer with addresses A (default) and B calls `CreateOrder` with `address_id` = B's id
- **THEN** the placed order's shipping address id and street are B's
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderUsesPickedAddress`

#### Scenario: Address and ETA: An empty address id uses the default address
- **WHEN** a buyer with addresses A and B (B is the default) calls `CreateOrder` with an empty `address_id`
- **THEN** the placed order's shipping address id is B's
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderEmptyAddressIDUsesDefault`

#### Scenario: Address and ETA: An unknown address id is refused and nothing is reserved
- **WHEN** a buyer calls `CreateOrder` with an `address_id` that is not in the buyer's address list
- **THEN** the call fails `FAILED_PRECONDITION` with message "address not found", zero stock reservations exist and the cart still holds all its lines
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderUnknownAddressRefused`

#### Scenario: Address and ETA: A buyer without any address is refused
- **WHEN** a buyer with no saved address calls `CreateOrder` with an empty `address_id`
- **THEN** the call fails `FAILED_PRECONDITION` with message "address not found" and zero stock reservations exist
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderNoAddressRefused`

#### Scenario: Address and ETA: Without a default address an empty address id is refused
- **WHEN** a buyer whose addresses are all non-default calls `CreateOrder` with an empty `address_id`
- **THEN** the call fails `FAILED_PRECONDITION` with message "address not found" and zero stock reservations exist
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderNoDefaultAddressRefused`

#### Scenario: Address and ETA: An address with an empty city is refused
- **WHEN** a buyer whose only address is the default and has city "" (or only spaces) calls `CreateOrder` with an empty `address_id`
- **THEN** the call fails `FAILED_PRECONDITION` with message "address not found", zero stock reservations exist, no order is placed and no shipping fee is charged
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderEmptyCityAddressRefused`

#### Scenario: Address and ETA: Another buyer's address id is refused
- **WHEN** buyer 1 calls `CreateOrder` with the `address_id` of an address owned by buyer 2
- **THEN** the call fails `FAILED_PRECONDITION` with the same message "address not found" as an unknown id and no order exists
- **VERIFIED BY**: platform-e2e `features/order/address_eta.feature` "A buyer cannot place an order at another buyer's address"

#### Scenario: Address and ETA: An identity outage never places an order
- **WHEN** a buyer calls `CreateOrder` while `ListAddresses` returns an error
- **THEN** the call fails `UNAVAILABLE`, zero stock reservations exist and the cart still holds all its lines
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderIdentityUnavailableRefused`

#### Scenario: Address and ETA: A missing address client refuses the order
- **WHEN** a buyer calls `CreateOrder` on a team-order started without an address client
- **THEN** the call fails `UNAVAILABLE` and zero stock reservations exist
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderNilAddressClientRefused`

#### Scenario: Address and ETA: A non-user principal is refused before any identity call
- **WHEN** a service principal calls `CreateOrder`
- **THEN** the call fails `PERMISSION_DENIED` and `ListAddresses` is called 0 times
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderNonUserNoAddressLookup`

### Requirement: CreateOrder refuses an address changed since confirm

When `CreateOrderRequest.confirmed_address` is set, `CreateOrder` SHALL compare its `recipient_name`, `phone`, `street`, `ward`, `district` and `city` for exact equality with the resolved address and, when any differs, SHALL fail with `FAILED_PRECONDITION` "address changed" before reserving anything, and SHALL log the refusal with the address id only. When `confirmed_address` is unset it SHALL skip the comparison. It SHALL NOT compare `id`, `user_id` or `is_default`, and it SHALL NOT write recipient, phone or street to any log line.

#### Scenario: Address and ETA: An unchanged confirmed address places the order
- **WHEN** a buyer calls `CreateOrder` with `confirmed_address` equal to identity's copy of the address in the six delivery fields
- **THEN** the order is placed with that address as its shipping snapshot
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderConfirmedAddressMatches`

#### Scenario: Address and ETA: An address edited after confirm is refused
- **WHEN** a buyer calls `CreateOrder` with a `confirmed_address` whose city is "Hà Nội" while identity's copy has city "Đà Nẵng"
- **THEN** the call fails `FAILED_PRECONDITION` with message "address changed", zero stock reservations exist and the cart still holds all its lines
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderEditedAddressRefused`

#### Scenario: Address and ETA: Each delivery field counts as a change
- **WHEN** a buyer calls `CreateOrder` six times, each with a `confirmed_address` that differs from identity's copy in exactly one of recipient_name, phone, street, ward, district, city
- **THEN** all six calls fail `FAILED_PRECONDITION` with message "address changed" and no order is placed
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderEachAddressFieldChangeRefused`

#### Scenario: Address and ETA: Metadata differences are not a change
- **WHEN** a buyer calls `CreateOrder` with a `confirmed_address` equal in the six delivery fields but with a different `is_default` and an empty `user_id`
- **THEN** the order is placed
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderConfirmedAddressIgnoresMetadata`

#### Scenario: Address and ETA: A request without a confirmed address is not compared
- **WHEN** a buyer calls `CreateOrder` without `confirmed_address`
- **THEN** the order is placed with identity's copy of the address
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderNoConfirmedAddressSkipsCompare`

#### Scenario: Address and ETA: Address details are never logged
- **WHEN** a buyer calls `CreateOrder` with a changed `confirmed_address` and the handler logs the refusal
- **THEN** the captured log output contains the address id and contains none of the recipient name, phone and street values
- **VERIFIED BY**: team-order `internal/handler` `TestCreateOrderAddressRefusalLogsIDOnly`

#### Scenario: Address and ETA: An edited address is refused end to end and a retry succeeds
- **WHEN** a buyer confirms address X, edits X through `AddressService`, places the order with the old `confirmed_address`, then places it again with the refreshed address
- **THEN** the first call fails `FAILED_PRECONDITION` "address changed" with no order created, and the second creates the order with the edited address as its snapshot
- **VERIFIED BY**: platform-e2e `features/order/address_eta.feature` "An address edited after confirm is refused"
