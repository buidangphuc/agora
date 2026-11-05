## ADDED Requirements

### Requirement: Promotion admin procedures are gated on promotion.admin at the edge

`team-gateway` SHALL require the scope `promotion.admin` on `VoucherService/DeactivateVoucher`,
`VoucherService/UpdateVoucher`, `FlashSaleService/ListCampaigns`, `FlashSaleService/CancelCampaign` and
`FlashSaleService/UpdateCampaign` before forwarding. An anonymous caller SHALL receive `unauthenticated` (HTTP 401). A
verified caller without `promotion.admin`, including a token that carries `admin` only, SHALL receive
`permission_denied` (HTTP 403) and team-promotion SHALL receive no call.

#### Scenario: Promotion admin edge: a token with admin but without promotion.admin is refused

- **WHEN** a caller whose token carries `admin` but not `promotion.admin` calls `ListCampaigns` and `DeactivateVoucher` through the gateway
- **THEN** the gateway answers HTTP 403 to both calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestPromotionAdminProceduresNeedPromotionAdmin`

#### Scenario: Promotion admin edge: an anonymous caller is refused

- **WHEN** a request without a token calls `CancelCampaign` through the gateway
- **THEN** the gateway answers HTTP 401

#### Scenario: Promotion admin edge: the seeded admin lists campaigns

- **WHEN** the seeded admin calls `ListCampaigns` through the gateway
- **THEN** the request succeeds with a campaigns list
