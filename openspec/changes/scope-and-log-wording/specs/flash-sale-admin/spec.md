## MODIFIED Requirements

### Requirement: Admin campaign screens

The admin app `/admin/campaigns` SHALL list every campaign with a status filter (all, scheduled, active, ended, cancelled), paging, a cancel action with a confirmation, and an edit action enabled only for scheduled campaigns. The `/admin/campaigns/new` form SHALL generate one idempotency key per form session and reuse it on every retry of the same submit, SHALL validate price, stock cap and window client-side matching the server rules, and SHALL map ALREADY_EXISTS, INVALID_ARGUMENT, `LISTING_UNAVAILABLE`, `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE` to distinct messages, reusing the shared mapping from promotion-admin-vouchers. A `flash_sale_disabled` response SHALL show a banner "flash sale is switched off" instead of an empty-list message. The screens and their server actions SHALL be reachable only by sessions holding both `admin` and `promotion.admin`, the scopes team-promotion requires for these RPCs.

#### Scenario: Filter and cancel from the list
- **WHEN** an admin opens `/admin/campaigns`, picks the filter "active" and confirms cancel on one row
- **THEN** the row shows status cancelled and the active filter no longer lists it
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Admin cancels a campaign from the list screen"

#### Scenario: Edit enabled only for scheduled
- **WHEN** an admin views a scheduled row and an ended row
- **THEN** the edit action is enabled on the scheduled row and absent or disabled on the ended row
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/CampaignRow.test.tsx` "edit only when scheduled"

#### Scenario: Create retry reuses the key
- **WHEN** the create request times out and the admin presses submit again on the same form
- **THEN** the second request carries the same idempotency_key and the list shows one new campaign
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "A create retry after a timeout makes one campaign"

#### Scenario: Server errors map to distinct messages
- **WHEN** create returns ALREADY_EXISTS, INVALID_ARGUMENT for `sale_price`, and UNAVAILABLE with each of `LISTING_UNAVAILABLE`, `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE`
- **THEN** five different messages are shown and none contains a raw backend message
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/campaignMessages.test.ts` "maps each error"

#### Scenario: Kill-switch banner
- **WHEN** the list response has flash_sale_disabled true
- **THEN** the banner "flash sale is switched off" is visible and the empty-list message is not
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/CampaignListView.test.tsx` "shows kill-switch banner"

#### Scenario: Non-admin reaches no campaign screen
- **WHEN** a seller or an anonymous visitor opens `/admin/campaigns`
- **THEN** the visitor is redirected to the admin login or shown forbidden and no campaign data is requested
- **VERIFIED BY**: team-frontend-admin `src/app/admin/campaigns/page.test.tsx` "forbids non-admin"

#### Scenario: Admin token without promotion.admin reaches no campaign screen
- **WHEN** a session holding `admin` but not `promotion.admin` opens `/admin/campaigns` or `/admin/campaigns/new`, or runs the create, cancel or update action
- **THEN** the app shows forbidden or returns a failed result and no campaign request is sent
- **VERIFIED BY**: team-frontend-admin `src/app/admin/campaigns/**/page.test.tsx`, `src/features/campaigns/actions.test.ts` and `src/features/admin/campaignActions.test.ts` "(admin token without promotion.admin)"
