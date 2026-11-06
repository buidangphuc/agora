## MODIFIED Requirements

### Requirement: Admin-only RPCs document their principal rule

The proto comments of the five new promotion RPCs and of the existing `CreateVoucher` and `CreateCampaign` SHALL be written per the initiative Contracts section as amended by Clarifications 2026-11-02 (quota rules; validation, including duplicate code and NOT_FOUND, runs before the audit append; idempotency lookup runs before validation). Each of the 7 comments SHALL state that the RPC is admin-only (scopes `admin` and `promotion.admin`) and, for write RPCs, the audit action written first (`voucher.deactivate`, `voucher.update`, `campaign.cancel`, `campaign.update`). Each comment SHALL name the ErrorInfo (domain `promotion.platform`) reasons that RPC can return: `DeactivateVoucher`, `UpdateVoucher`, `CreateVoucher`, `CancelCampaign`: `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE`; `ListCampaigns`: `PROMOTION_UNAVAILABLE`; `CreateCampaign` and `UpdateCampaign`: `AUDIT_UNAVAILABLE`, `PROMOTION_UNAVAILABLE` and `LISTING_UNAVAILABLE`. Only `CreateCampaign` and `UpdateCampaign` name `LISTING_UNAVAILABLE`. The `CreateVoucher` comment SHALL name ALREADY_EXISTS for a taken code. The `CreateCampaign` comment SHALL state that `idempotency_key` is required on the admin path, 8..64 chars `[A-Za-z0-9_-]`, and that ALREADY_EXISTS is returned for the same key with a different payload. The audit catalogue comment in `audit.proto` SHALL list the four new actions. Enforcement and authz tests are owned by promotion-admin-vouchers and promotion-admin-campaigns.

#### Scenario: Comments name the authz rule and audit action
- **WHEN** the proto source of the 7 RPCs and the audit catalogue are read
- **THEN** all 7 RPC comments contain "admin-only" and "promotion.platform", each contains exactly the reasons listed for it above (only `CreateCampaign` and `UpdateCampaign` contain "LISTING_UNAVAILABLE"), the `CreateVoucher` comment contains "ALREADY_EXISTS", the `CreateCampaign` comment contains "idempotency_key", "8..64", "[A-Za-z0-9_-]" and "ALREADY_EXISTS", the four new write RPC comments contain their audit action string, and the audit catalogue comment contains all 4 action strings
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_AdminRPCsDocumented`

#### Scenario: Missing authz comment is rejected
- **WHEN** the test builds a temp copy of `promotion.proto` with the phrase "admin-only", an RPC's listed ErrorInfo reason, or the idempotency_key rule removed from one RPC comment
- **THEN** `TestGapsContract_AdminRPCsDocumented` run against the temp copy fails naming the RPC
- **VERIFIED BY**: platform-core `packages/proto` `TestGapsContract_AdminRPCsDocumented`
