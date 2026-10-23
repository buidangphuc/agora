## REMOVED Requirements

### Requirement: AddToCart prices a line at the active flash price

**Reason**: the requirement allowed a promotion-less mode (no promotion client configured), a runtime fallback that the go-sdk migration deletes.

**Migration**: replaced by the requirement "AddToCart prices a line at the active flash price through a required promotion client" below, which has the same pricing rules and no promotion-less mode.

## MODIFIED Requirements

### Requirement: Flash RPCs are sent as team-order's service principal

`upstream.DialAll` SHALL return a client set whose `Flash` client sends `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` with `x-principal-id=service-team-order`, `x-principal-type=service` and scope `promotion.reserve`, replacing any forwarded caller principal, and SHALL leave `GetActiveFlashSale` with the forwarded principal. `UPSTREAM_PROMOTION_ADDR` SHALL be required in every ENV, so the client set always exists.

#### Scenario: Advertised price: Hold RPCs carry the service principal
- **WHEN** the wrapped flash client sends a reserve, a commit and a release inside a buyer's request context
- **THEN** each outgoing call carries principal id `service-team-order`, type `service` and scope `promotion.reserve`, and not the buyer's id
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceFlashClientSendsServicePrincipal`

#### Scenario: Advertised price: Price read keeps the forwarded principal
- **WHEN** the wrapped flash client sends `GetActiveFlashSale` inside a buyer's request context
- **THEN** the outgoing call carries the buyer's principal unchanged
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceFlashClientReadNotWrapped`

#### Scenario: Advertised price: Unset promotion address yields no client
- **WHEN** `config.Load()` runs with `UPSTREAM_PROMOTION_ADDR` unset in any ENV
- **THEN** it returns an error naming `UPSTREAM_PROMOTION_ADDR`, so `DialAll` is never reached with an empty address
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: Advertised price: A buyer token cannot reserve flash stock end to end
- **WHEN** a buyer token calls `ReserveFlashSaleStock` directly through the gateway
- **THEN** the call is refused with `permission_denied`, `not_found` or `unimplemented` and `GetFlashSaleStock.remaining` is unchanged
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "Advertised price: A buyer token cannot reserve flash stock end to end"

## ADDED Requirements

### Requirement: AddToCart prices a line at the active flash price through a required promotion client

`CartService.AddToCart` SHALL resolve the unit price of the added line with one `FlashSaleService.GetActiveFlashSale(listing_id)` call. The unit price SHALL be the campaign `sale_price` when the campaign is active, its `variant_id` is empty or equals the line's variant_id, and `0 < sale_price < regular price` (the listing price, or the variant price when the variant price is above 0). In every other case the unit price SHALL be the regular price. When the call fails, the unit price SHALL be the regular price. team-order SHALL always be built with a promotion flash client: the constructor of the cart service SHALL return an error for a nil client, and `UPSTREAM_PROMOTION_ADDR` SHALL be required in a strict ENV. `Reorder` SHALL price each re-added item through the same path.

#### Scenario: Advertised price: Listing-wide flash price is added to the cart
- **WHEN** a buyer adds quantity 2 of a listing priced 500000 with an active campaign of empty variant_id and sale_price 350000
- **THEN** the cart line unit_price is 350000 and the cart subtotal is 700000
- **VERIFIED BY**: team-order `internal/order` `TestAddToCartFlashPriceListingWide`

#### Scenario: Advertised price: Variant campaign prices only its variant
- **WHEN** a buyer adds variant "v1" and variant "v2" of one listing, and the active campaign names variant "v1" with sale_price 300000 (regular 400000)
- **THEN** the "v1" line unit_price is 300000 and the "v2" line unit_price is 400000
- **VERIFIED BY**: team-order `internal/order` `TestAddToCartFlashPriceVariantOnly`

#### Scenario: Advertised price: Sale price at or above the regular price is ignored
- **WHEN** a buyer adds a listing priced 200000 whose active campaign has sale_price 200000, and another whose campaign has sale_price 250000
- **THEN** both cart lines have the regular unit_price
- **VERIFIED BY**: team-order `internal/order` `TestAddToCartFlashPriceNotBelowRegularIgnored`

#### Scenario: Advertised price: No active campaign keeps the regular price
- **WHEN** a buyer adds a listing for which `GetActiveFlashSale` returns active=false
- **THEN** the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/order` `TestAddToCartNoCampaignRegularPrice`

#### Scenario: Advertised price: Promotion error at add-to-cart falls back to the regular price
- **WHEN** a buyer adds a listing while `GetActiveFlashSale` returns an error
- **THEN** the call succeeds and the cart line unit_price equals the regular price
- **VERIFIED BY**: team-order `internal/order` `TestAddToCartPromotionErrorRegularPrice`

#### Scenario: Advertised price: A cart service without a promotion client is refused
- **WHEN** `order.NewCartService` is called with a nil flash client
- **THEN** it returns a non-nil error naming the flash client and builds no service
- **VERIFIED BY**: team-order `internal/order` `TestNewCartServiceRefusesNilFlashClient`

#### Scenario: Advertised price: Reorder re-prices through add-to-cart
- **WHEN** a buyer reorders a past order whose item was bought at the regular price while a flash campaign with sale_price 350000 is now active
- **THEN** the cart line unit_price is 350000
- **VERIFIED BY**: team-order `internal/order` `TestReorderUsesFlashPrice`

#### Scenario: Advertised price: Flash price is visible end to end after add-to-cart
- **WHEN** a buyer adds a listing with an active campaign (sale_price below the regular price) through the gateway
- **THEN** `GetCart` returns the line unit_price equal to the campaign sale_price
- **VERIFIED BY**: platform-e2e `features/order/advertised_price.feature` "Advertised price: Flash price is visible end to end after add-to-cart"
