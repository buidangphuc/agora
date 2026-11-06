## MODIFIED Requirements

### Requirement: Admin app screens for stop and edit
The admin app SHALL let an admin stop a voucher and edit its end date and quota from `/admin/vouchers`, and the buyer app SHALL tell the buyer a stopped voucher no longer applies. Only sessions holding both `admin` and `promotion.admin` SHALL reach the voucher screens and run their server actions.

#### Scenario: V28 stop from the list
- **WHEN** an admin confirms "Stop" on an active voucher in the admin app
- **THEN** the voucher row SHALL show status "stopped" after refresh

#### Scenario: V28a buyer sees a stopped voucher as not applicable
- **WHEN** a buyer applies the stopped code in checkout in the buyer app
- **THEN** both VoucherModal components (`team-frontend/src/features/order` and `src/features/voucher`) SHALL show "Mã giảm giá đã ngừng áp dụng." and no raw reason string

#### Scenario: V28b other reasons keep their text
- **WHEN** the buyer applies a code refused with a reason other than `inactive`
- **THEN** the modal SHALL show the same text as before this change

#### Scenario: V29 edit end date and quota
- **WHEN** an admin saves a new end date and a higher quota on the voucher detail screen
- **THEN** the detail screen SHALL show the new end date and quota

#### Scenario: V29a money fields read-only
- **WHEN** an admin opens the edit screen
- **THEN** discount, min spend, max discount and scope SHALL be read-only

#### Scenario: V30 edit validation shown on the field
- **WHEN** an admin enters a quota below the used count or below the current quota, or an end date not after the start date
- **THEN** the form SHALL show an error on that field and send no request

#### Scenario: V30a quota field hidden for unlimited vouchers
- **WHEN** an admin opens the edit form of a voucher whose quota is 0 (unlimited)
- **THEN** the form SHALL not render the quota field

#### Scenario: V31 stopped voucher has no edit
- **WHEN** an admin opens a stopped voucher
- **THEN** the Stop and Edit controls SHALL be absent

#### Scenario: V32 non-admin sees no controls
- **WHEN** a signed-in user without scope admin opens `/admin/vouchers`
- **THEN** the app SHALL show the forbidden screen with no Stop or Edit control

#### Scenario: V32a admin token without promotion.admin is refused
- **WHEN** a session holding `admin` but not `promotion.admin` opens `/admin/vouchers`, `/admin/vouchers/new` or a voucher detail, or runs the create, stop or edit action
- **THEN** the app SHALL show the forbidden screen or return the forbidden result, and SHALL send no request to the gateway
- **VERIFIED BY**: team-frontend-admin `src/app/admin/vouchers/**/page.test.tsx` and `src/features/vouchers/actions.test.ts` "(admin token without promotion.admin)"

#### Scenario: V33 create dialog no longer claims immutability
- **WHEN** an admin opens the create confirmation dialog
- **THEN** the dialog text SHALL NOT state that a voucher cannot be changed after creation
