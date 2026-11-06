## MODIFIED Requirements

### Requirement: Audit action naming rule and catalogue are documented

The `WriteAuditEventRequest` comment SHALL state the action naming rule `<entity>.<verb>`, state that every call by a caller holding `admin` (team-order: `order.admin`; team-promotion: `admin` and `promotion.admin`) to a catalogue RPC is audited even when that caller owns the resource, and list the catalogue: `review.hide`, `kyc.review`, `dispute.resolve`, `listing.admin_update`, `listing.admin_delete`, `order.force_fail`, `order.admin_status_update`, `return.admin_status_update`, `shipment.admin_create`, `payment.admin_refund`, `voucher.platform_create`, `flash_sale.admin_create`, `ad_campaign.admin_create`. The wire of `WriteAuditEventRequest`, `WriteAuditEventResponse` and `AuditEvent` SHALL NOT change.

#### Scenario: Naming rule and catalogue are present
- **WHEN** a reader opens the `WriteAuditEventRequest` comment
- **THEN** it contains the text `<entity>.<verb>` and each of the 13 action strings above
- **VERIFIED BY**: 14-string comment assertion in `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Write wire is unchanged
- **WHEN** the descriptors of `WriteAuditEventRequest` (`actor_id = 1`, `action = 2`, `target_type = 3`, `target_id = 4`, `metadata = 5`), `WriteAuditEventResponse` (no fields) and `AuditEvent` (fields 1-7) are inspected
- **THEN** every field keeps its name, number and type and no field is added or removed
- **VERIFIED BY**: descriptor assertion in `auditquery_roundtrip_test.go` (tasks 2.1) plus `make -C platform-core breaking`

#### Scenario: Action missing from the catalogue is caught
- **WHEN** the comment on `WriteAuditEventRequest` lacks the naming rule or any one of the 13 action strings
- **THEN** the comment assertion fails and the contract slice is not done
- **VERIFIED BY**: 14-string comment assertion in `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Uniform admin-path wording and non-admin exclusion
- **WHEN** a reader opens the `WriteAuditEventRequest` comment
- **THEN** it states that a caller holding the `admin` (team-order: `order.admin`; team-promotion: `admin` and `promotion.admin`) scope is audited even when they own the resource, and that a caller without that scope (permission denied for the admin path) is never audited
- **VERIFIED BY**: comment-text assertion in `auditquery_roundtrip_test.go` (tasks 2.1)
