@admin
Feature: Admin voucher screens in the browser (promotion-admin-vouchers)
  /admin/vouchers lets an admin stop a voucher and edit its end date and quota, shows distinct
  messages for a taken code, audit down and promotion down, and a stopped voucher tells a buyer it
  no longer applies. Scenario names echo the spec scenarios of
  openspec/changes/promotion-admin-vouchers. Every refusal is paired with a positive control.

  Scenario: V20 admin UI shows the duplicate message
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin has the create form filled with the code of that voucher
    When the admin submits the form
    Then the form shows "code already exists" and keeps the typed code

  Scenario: V20a no voucher added on duplicate
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin has the create form filled with the code of that voucher
    When the admin submits the form
    Then the voucher list holds exactly one row for that code

  @destructive
  Scenario: V26 admin UI audit-down message
    Given the seeded admin for the voucher admin checks
    And an admin is signed in the admin app
    And the admin has the create form filled with a fresh code
    And team-audit is stopped for the voucher admin checks
    When the admin submits the form
    Then the admin app shows "audit is down, nothing created"

  @destructive
  Scenario: V26a admin UI audit-down message on stop and edit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin has the stop dialog open on that voucher
    And team-audit is stopped for the voucher admin checks
    When the admin submits the form
    Then the admin app shows "audit is down, nothing changed"
    When the admin has the edit form filled with quota 8
    And the admin submits the form
    Then the admin app shows "audit is down, nothing changed"

  @destructive
  Scenario: V27 admin UI promotion-down message
    Given the seeded admin for the voucher admin checks
    And an admin is signed in the admin app
    And the admin has the create form filled with a fresh code
    And team-promotion is stopped for the voucher admin checks
    When the admin submits the form
    Then the admin app shows "promotion is down, nothing created"

  @destructive
  Scenario: V27b admin UI promotion-down on stop and edit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin has the stop dialog open on that voucher
    And team-promotion is stopped for the voucher admin checks
    When the admin submits the form
    Then the admin app shows "promotion is down, nothing changed"

  @destructive
  Scenario: V27c the two messages differ
    Given the seeded admin for the voucher admin checks
    And an admin is signed in the admin app
    And the admin has the create form filled with a fresh code
    And team-audit is stopped for the voucher admin checks
    When the admin submits the form
    Then the admin app shows "audit is down, nothing created"
    And the admin app does not show "promotion is down"

  Scenario: V28 stop from the list
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin has the stop dialog open on that voucher
    When the admin submits the form
    Then after a refresh the voucher row shows status "stopped"

  Scenario: V28a buyer sees a stopped voucher as not applicable
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And the admin has stopped the voucher
    And a promotion buyer has a qualifying cart and a saved address
    When the buyer applies the stopped code at checkout
    Then the checkout voucher alert reads "Mã giảm giá đã ngừng áp dụng." and no raw reason

  Scenario: V28b other reasons keep their text
    Given the seeded admin for the voucher admin checks
    And a promotion buyer has a qualifying cart and a saved address
    When the buyer applies an unknown code at checkout
    Then the checkout voucher alert is shown, is not the stopped text and holds no raw "inactive"

  Scenario: V29 edit end date and quota
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin opens the detail screen of that voucher
    When the admin has the edit form filled with quota 8
    And the admin submits the form
    Then the detail screen shows quota 8 and the new end date

  Scenario: V29a money fields read-only
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin opens the detail screen of that voucher
    When the admin opens the edit form
    Then the discount, min spend, max discount and scope are read-only
    And the end date and quota fields are editable

  Scenario: V30 edit validation shown on the field
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 2 used
    And an admin is signed in the admin app
    And the admin opens the detail screen of that voucher
    When the admin opens the edit form
    And the admin enters a quota of 1 and submits
    And the admin enters a quota of 3 and submits
    And the admin enters an end date before the start date and submits
    Then every attempt shows an error on its field
    And no UpdateVoucher request was sent

  Scenario: V30a quota field hidden for unlimited vouchers
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with unlimited quota
    And an admin is signed in the admin app
    And the admin opens the detail screen of that voucher
    When the admin opens the edit form
    Then the form renders no quota field
    And the end date field is editable

  Scenario: V31 stopped voucher has no edit
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And the admin has stopped the voucher
    And an admin is signed in the admin app
    When the admin opens the detail screen of that voucher
    Then the voucher shows status "stopped"
    And the Stop and Edit controls are absent

  Scenario: V32 non-admin sees no controls
    Given the seeded admin for the voucher admin checks
    And an active platform voucher with quota 5 and 0 used
    And an admin is signed in the admin app
    And the admin sees the voucher list with the Stop control
    When a signed-in seller opens the vouchers page
    Then the page shows the forbidden screen with no Stop or Edit control

  Scenario: V33 create dialog no longer claims immutability
    Given the seeded admin for the voucher admin checks
    And an admin is signed in the admin app
    And the admin has the create form filled with a fresh code
    When the admin opens the create confirmation dialog
    Then the dialog has text and does not state that a voucher cannot be changed
