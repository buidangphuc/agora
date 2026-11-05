# promotion-admin-contract Specification

## Purpose
TBD - created by archiving change p10-gaps-contract. Update Purpose after archive.

## Requirements

### Requirement: Promotion admin contract is additive

The promotion proto SHALL declare `VoucherService.DeactivateVoucher` and `UpdateVoucher`, `FlashSaleService.ListCampaigns`, `CancelCampaign` and `UpdateCampaign`, the `CampaignStatus` enum, and the new fields of `Voucher` (13, 14), `FlashSaleCampaign` (9, 10), `CreateCampaignRequest` (7) and `ListActiveCampaignsResponse` (3), with the field numbers and types given in the initiative contract. Every existing field number, name and type SHALL remain unchanged. Enforcement and authz tests are owned by promotion-admin-vouchers and promotion-admin-campaigns.

#### Scenario: Voucher RPCs and fields exist
- **WHEN** the compiled promotion descriptor is loaded
- **THEN** `VoucherService` has `DeactivateVoucher(DeactivateVoucherRequest) returns (DeactivateVoucherResponse)` and `UpdateVoucher(UpdateVoucherRequest) returns (UpdateVoucherResponse)`, `Voucher` has `bool active = 13` and `Timestamp deactivated_at = 14`, and `UpdateVoucherRequest.ends_at` (2) and `quota` (3) are proto3 `optional`
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_VoucherRPCsAndFields`

#### Scenario: Campaign RPCs, enum and fields exist
- **WHEN** the compiled promotion descriptor is loaded
- **THEN** `FlashSaleService` has `ListCampaigns`, `CancelCampaign` and `UpdateCampaign` with the declared request and response messages, `CampaignStatus` has exactly the values UNSPECIFIED=0, SCHEDULED=1, ACTIVE=2, ENDED=3, CANCELLED=4, `FlashSaleCampaign` has `status = 9` and `cancelled_at = 10`, `CreateCampaignRequest.idempotency_key = 7`, `ListCampaignsResponse.flash_sale_disabled = 3` and `ListActiveCampaignsResponse.flash_sale_disabled = 3`
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_CampaignRPCsEnumAndFields`

#### Scenario: Existing promotion fields are untouched
- **WHEN** the promotion descriptor is compared with the descriptor on main before this change
- **THEN** no existing field number, name, type or RPC signature differs, and `buf breaking --against main` with FILE rules exits 0 with no findings
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_PromotionBreakingClean`

#### Scenario: Old payload decodes under the new schema
- **WHEN** a `Voucher`, `FlashSaleCampaign` and `CreateCampaignRequest` serialized with the pre-change schema are unmarshalled with the new schema
- **THEN** each decodes with no error, `active=false`, `status=CAMPAIGN_STATUS_UNSPECIFIED`, `idempotency_key=""`, and every pre-existing field keeps its value
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_OldPayloadDecodes`

#### Scenario: Renumbering an existing promotion field is rejected
- **WHEN** a temp copy of the proto created by the test changes the number of `FlashSaleCampaign.ends_at` from 8 to 11 and `buf breaking` runs against the original (the repo source is never edited)
- **THEN** `buf breaking` exits non-zero and reports a FIELD_SAME_NUMBER style finding naming `ends_at`
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_RenumberDetected`

#### Scenario: Pagination reuses the common message
- **WHEN** `ListCampaignsRequest` and `ListCampaignsResponse` are inspected
- **THEN** `page` is `platform.common.v1.PageRequest` (field 2) in the request and `platform.common.v1.PageResponse` (field 2) in the response, and `statuses` (field 1) is `repeated CampaignStatus` with no cap declared in the schema (the page size limit stays in common)
- **VERIFIED BY**: platform-core `packages/proto/platform/promotion/v1` `TestGapsContract_ListCampaignsShape`

### Requirement: Admin-only RPCs document their principal rule

The proto comments of the five new promotion RPCs and of the existing `CreateVoucher` and `CreateCampaign` SHALL be written per the initiative Contracts section as amended by Clarifications 2026-11-02 (quota rules; validation, including duplicate code and NOT_FOUND, runs before the audit append; idempotency lookup runs before validation). Each of the 7 comments SHALL state that the RPC is admin-only (scope `admin`) and, for write RPCs, the audit action written first (`voucher.deactivate`, `voucher.update`, `campaign.cancel`, `campaign.update`). Each comment SHALL name the ErrorInfo (domain `promotion.platform`) reasons that RPC can return: `DeactivateVoucher`, `UpdateVoucher`, `CreateVoucher`, `CancelCampaign`: `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE`; `ListCampaigns`: `PROMOTION_UNAVAILABLE`; `CreateCampaign` and `UpdateCampaign`: `AUDIT_UNAVAILABLE`, `PROMOTION_UNAVAILABLE` and `LISTING_UNAVAILABLE`. Only `CreateCampaign` and `UpdateCampaign` name `LISTING_UNAVAILABLE`. The `CreateVoucher` comment SHALL name ALREADY_EXISTS for a taken code. The `CreateCampaign` comment SHALL state that `idempotency_key` is required on the admin path, 8..64 chars `[A-Za-z0-9_-]`, and that ALREADY_EXISTS is returned for the same key with a different payload. The audit catalogue comment in `audit.proto` SHALL list the four new actions. Enforcement and authz tests are owned by promotion-admin-vouchers and promotion-admin-campaigns.

#### Scenario: Comments name the authz rule and audit action
- **WHEN** the proto source of the 7 RPCs and the audit catalogue are read
- **THEN** all 7 RPC comments contain "admin-only" and "promotion.platform", each contains exactly the reasons listed for it above (only `CreateCampaign` and `UpdateCampaign` contain "LISTING_UNAVAILABLE"), the `CreateVoucher` comment contains "ALREADY_EXISTS", the `CreateCampaign` comment contains "idempotency_key", "8..64", "[A-Za-z0-9_-]" and "ALREADY_EXISTS", the four new write RPC comments contain their audit action string, and the audit catalogue comment contains all 4 action strings
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_AdminRPCsDocumented`

#### Scenario: Missing authz comment is rejected
- **WHEN** the test builds a temp copy of `promotion.proto` with the phrase "admin-only", an RPC's listed ErrorInfo reason, or the idempotency_key rule removed from one RPC comment
- **THEN** `TestGapsContract_AdminRPCsDocumented` run against the temp copy fails naming the RPC
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_AdminRPCsDocumented`

### Requirement: Promotion consumers are regenerated

Every repo that vendors `platform/promotion/v1` (found by globbing `team-*/proto/platform/promotion/v1`; today team-promotion, team-gateway, team-order, team-analytics, team-frontend, team-frontend-admin, team-frontend-seller) SHALL hold a `proto/platform/promotion/v1/promotion.proto` byte-identical to the platform-core source and generated code that builds.

#### Scenario: Promotion copies match the source
- **WHEN** the vendored `promotion.proto` of each consumer found by the glob is compared with the platform-core source
- **THEN** all 7 copies are byte-identical and each contains `DeactivateVoucher` and `ListCampaigns`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `promotion-copies-match`

#### Scenario: A drifted or missing promotion copy is rejected
- **WHEN** one consumer's vendored copy differs from the source or is absent
- **THEN** the check exits non-zero and prints the repo name
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `promotion-copies-drift-rejected`

#### Scenario: Generated code compiles in Go consumers
- **WHEN** `go build ./...` runs in every Go consumer found by the glob after regeneration
- **THEN** every build exits 0 and the generated client for `VoucherService` exposes `DeactivateVoucher` and `UpdateVoucher`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `promotion-go-builds`

#### Scenario: Generated TypeScript compiles in frontend consumers
- **WHEN** `npm run typecheck` runs in every frontend consumer found by the glob after regeneration
- **THEN** every run exits 0 and the generated `FlashSaleService` exposes `listCampaigns`, `cancelCampaign` and `updateCampaign`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `promotion-ts-builds`

### Requirement: Audit catalogue copies are re-vendored

Every repo that vendors `platform/audit/v1/audit.proto` (found by globbing `team-*/proto/platform/audit/v1`; 13 repos today) SHALL hold a copy byte-identical to the platform-core source, which lists the 6 new actions in its catalogue comment. Enforcement and authz tests are owned by the feature slices that write these audit actions.

#### Scenario: Audit copies match the source
- **WHEN** the vendored `audit.proto` of each consumer found by the glob is compared with the platform-core source
- **THEN** every copy is byte-identical and contains all 6 strings `voucher.deactivate`, `voucher.update`, `campaign.cancel`, `campaign.update`, `auth.handoff_mint`, `auth.handoff_redeem`
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `audit-copies-match`

#### Scenario: A drifted or missing audit copy is rejected
- **WHEN** one consumer's vendored `audit.proto` differs from the source or lacks one of the 6 actions
- **THEN** the check exits non-zero and prints the repo name
- **VERIFIED BY**: platform-core `packages/proto` `gaps_consumer_copies_test.sh` check `audit-copies-drift-rejected`
