## MODIFIED Requirements

### Requirement: Admin entry links

The admin app's cockpit page SHALL link to `/admin/listings` ("Kiểm duyệt tin đăng") and `/admin/cases`
("Hộp thư vụ việc"), using internal `next/link`. The buyer app (`ConsumerShell`) SHALL NOT render admin links:
admin pages are served only by the admin app at `ADMIN_BASE_URL`. A signed-in user without the `admin` scope who
opens an admin link on the admin app SHALL see the admin layout's "Không có quyền" page instead of the admin page.

#### Scenario: Moderation UI: Admin reaches the pages from the cockpit
- **WHEN** an admin opens `/admin/cockpit` and clicks "Hộp thư vụ việc"
- **THEN** the browser lands on `/admin/cases` and the "Báo cáo" tab is selected
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: Admin reaches the pages from the cockpit`

#### Scenario: Moderation UI: Cockpit shows both links
- **WHEN** an admin renders the cockpit view
- **THEN** it contains links to `/admin/listings` and `/admin/cases`
- **VERIFIED BY**: `team-frontend-admin/src/features/admin/CockpitView.test.tsx` `shows the moderation links`

#### Scenario: Moderation UI: A non-admin following an admin link is refused
- **WHEN** a signed-in buyer opens the admin link `/admin/cases` on the admin app
- **THEN** the page shows "Không có quyền" and no report table, because the admin layout refuses a principal without the `admin` scope before the cases page renders
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature` `Moderation UI: A non-admin following an admin link is refused`; `team-frontend-admin/src/app/admin/layout.test.tsx` `shows Không có quyền to a signed-in user without the admin scope`
