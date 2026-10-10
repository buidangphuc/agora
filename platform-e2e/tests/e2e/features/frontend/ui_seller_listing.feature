@seller
Feature: Seller listings - delete Modal and the validated listing studio
  OpenSpec change ui-phase-seller. A slow server is simulated in the browser by delaying the
  Next server-action POST; failures are provoked by real stack state changes.

  Scenario: Confirm shows pending then success
    Given a d2 seller has 2 published listings
    When the d2 seller opens the workplace
    And the d2 seller opens the delete Modal of the first listing
    And the d2 seller confirms the deletion while the server is slow
    Then the confirm button is busy and disabled and the dialog cannot be dismissed
    And the Modal closes, a success toast appears and the row is gone from the table

  Scenario: Failed delete keeps the Modal open
    Given a d2 seller has 2 published listings
    When the d2 seller opens the workplace
    And the d2 seller opens the delete Modal of the first listing
    And the first listing is deleted from another session
    And the d2 seller confirms the deletion
    Then the Modal stays open with an alert, an error toast appears and the confirm button is enabled

  Scenario: Required field error is announced
    Given a d2 seller has 0 published listings
    When the d2 seller opens the new listing studio
    And the d2 seller submits the form with an empty title
    Then the title form item shows an error linked by aria-describedby, the input is invalid and no request was sent

  Scenario: Submit is pending and non-repeatable
    Given a d2 seller has 0 published listings
    When the d2 seller opens the new listing studio
    And the d2 seller submits a valid listing while the server is slow
    Then the submit button shows a spinner with unchanged width, a second click does nothing and a saved toast appears

  Scenario: Create succeeds and the listing appears in the seller's list
    Given a d2 seller has 0 published listings
    When the d2 seller opens the new listing studio
    And the d2 seller creates a listing with valid data
    Then a success state offers a list link and the new listing appears on the seller list

  Scenario: Server error is surfaced
    Given a d2 seller has 1 published listings
    When the d2 seller opens the edit page of the first listing
    And the first listing is deleted from another session
    And the d2 seller changes the title and saves
    Then an alert at the top shows the message, an error toast appears, the typed values are kept and submit is enabled

  Scenario: Apply fills the controlled fields
    Given a d2 seller has 0 published listings
    When the d2 seller opens the new listing studio
    And the d2 seller types a title, applies every AI suggestion and reads the fields
    Then the title, description and price fields show the suggestion and submitting sends them
