## Why

Gaps G8-G15 of the P10 backend initiative need new RPCs, fields and one event before the feature slices (promotion-admin-vouchers, promotion-admin-campaigns, identity-sso-handoff, identity-seller-onboarding) can be built in parallel. This change ships only the wire contract, field-additive, so every consumer can vendor and regenerate once and the feature slices share one frozen interface.

initiative: p10-backend-gaps

## What changes

- `platform/promotion/v1/promotion.proto`
  - `VoucherService`: add `DeactivateVoucher`, `UpdateVoucher` plus their request/response messages; `Voucher` gains `active = 13`, `deactivated_at = 14`.
  - New enum `CampaignStatus` (UNSPECIFIED, SCHEDULED, ACTIVE, ENDED, CANCELLED).
  - `FlashSaleService`: add `ListCampaigns`, `CancelCampaign`, `UpdateCampaign` plus messages; `FlashSaleCampaign` gains `status = 9`, `cancelled_at = 10`; `CreateCampaignRequest` gains `idempotency_key = 7`; `ListActiveCampaignsResponse` gains `flash_sale_disabled = 3`.
- `platform/identity/v1/identity.proto`
  - New enum `HandoffApp`; `AuthService` gains `CreateHandoffCode`, `RedeemHandoffCode`, `PeekHandoffCode` (public, read-only, does not consume the code), `BecomeSeller` plus messages.
  - New event message `UserRoleGranted` (topic `identity.events`).
- `platform/audit/v1/audit.proto`: the action catalogue comment lists the six new actions (`voucher.deactivate`, `voucher.update`, `campaign.cancel`, `campaign.update`, `auth.handoff_mint`, `auth.handoff_redeem`). Comment only, no wire change; every consumer that vendors `audit.proto` (glob `team-*/proto/platform/audit/v1`, 13 repos today) re-vendors it.
- Consumers re-vendor `proto/` and regenerate: team-promotion, team-identity, team-gateway, team-frontend, team-frontend-admin, team-frontend-seller, team-order, team-analytics (promotion and identity); team-chat, team-domain, team-engagement, team-notification, team-payment, team-search (identity only). Consumers are found by globbing `team-*/proto/platform/<domain>/v1`, never from a fixed list.
- No behaviour: RPC semantics are owned by the four feature slices. Nothing is renumbered, removed or retyped.

## NFR

- Wire compatibility: `buf lint` and `buf breaking` (FILE rules) against main report zero findings. Old clients and servers interoperate: absent new fields decode to zero values.
- Regeneration is deterministic: a second `make proto` run produces no diff.
- No runtime availability or latency impact (types only).

## Data classification

This change adds schema only and stores nothing. Fields that will carry personal or secret data once implemented, with the owning slice:

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `CreateHandoffCodeResponse.code`, `RedeemHandoffCodeRequest.code`, `PeekHandoffCodeRequest.code` | secret bearer credential | sign-in hand-off | contract | never stored raw; only SHA-256 stored, purged 24 h after expiry (identity-sso-handoff) |
| `BecomeSellerRequest.terms_version` | user-attributed consent record | proof of seller-terms acceptance | contract | account lifetime (identity-seller-onboarding) |
| `PeekHandoffCodeResponse.username` | personal identifier | show "continue as <username>?" before redeem | contract | never stored by the contract; not logged (identity-sso-handoff) |
| `UserRoleGranted.user_id` | personal identifier | downstream role reaction | contract | inherits the identity.events topic retention, 7 days (Kafka default), set by identity-seller-onboarding; never logged raw |
| `CreateCampaignRequest.idempotency_key` | non-personal token | safe retry | contract | 24 h (promotion-admin-campaigns) |

## Threat model

- Assets: hand-off code (bearer secret), seller role grant, admin write RPCs on vouchers and campaigns.
- Abuse cases: a contract that leaves a mint/redeem/admin RPC without a documented auth rule invites a gateway scope gap; a breaking field change corrupts in-flight Kafka or gRPC traffic during staggered deploys.
- Authz rule at contract level: the proto comments state the required principal for each new RPC (admin scope for the five promotion RPCs, authenticated user for `CreateHandoffCode` and `BecomeSeller`, public for `RedeemHandoffCode` and the read-only `PeekHandoffCode`). Enforcement and tests belong to the feature slices. Field numbers are append-only, so a stale peer cannot misread an existing field.
