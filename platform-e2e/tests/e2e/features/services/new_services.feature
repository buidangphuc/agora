Feature: New backend services API round-trips
  These four services have thin/no UI, so they are exercised directly through
  the team-gateway Connect/JSON API (POST /<package.Service>/<Method>).

  Scenario: Referral code creation is readable via GetMyReferral
    Given an authenticated buyer
    When the buyer creates a referral code
    Then GetMyReferral returns that same code
    And listing referral rewards succeeds

  Scenario: A created share link resolves back to its target with OG meta
    Given an authenticated seller
    When the seller creates a share link for target "listing" "listing_001"
    Then resolving the short code returns target "listing" "listing_001"
    And the resolved link carries OG meta

  Scenario: A written audit event is returned by QueryAuditLog
    Given an authenticated seller
    When an audit event is written for the seller
    Then querying the audit log returns that event

  Scenario: Submitting KYC yields a PENDING verification status
    Given an authenticated buyer
    When the buyer submits a KYC document
    Then the verification status is PENDING
