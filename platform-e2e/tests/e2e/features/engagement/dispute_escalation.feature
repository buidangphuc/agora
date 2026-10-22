@engagement @order
Feature: A buyer escalates a rejected return to a dispute and an admin rules on it
  CreateDispute accepts a return_id that team-order reports as escalatable, ResolveDispute enforces
  the state machine and a ruling, the audit event is written first, DisputeResolved reaches
  engagement.events once, and the order and admin lists show the disputes. Everything runs through
  the gateway with real users; the return timers come from compose/order-inventory.override.yaml
  (auto-complete 12s, return window 20s). (engagement-dispute-escalation, initiative q6-trust-enforcement)

  Scenario: Dispute escalation: Return that cannot be escalated is refused
    Given a seller and a buyer exist
    And the buyer has order "o1" with an "approved" return "r1"
    When the buyer escalates return "r1" of order "o1"
    Then the dispute call is refused with code "failed_precondition"
    And the admin sees 0 disputes on order "o1"
    Given the buyer has order "o2" with a "rejected" return "r2"
    When the buyer escalates return "r2" of order "o2" as dispute "d2"
    Then the dispute call succeeds with dispute "d2" OPEN and linked to return "r2"
    And the admin sees 1 disputes on order "o2"

  Scenario: Dispute escalation: Second dispute on the same return is refused while one is open
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    Then the dispute call succeeds with dispute "d1" OPEN and linked to return "r1"
    When the buyer escalates return "r1" of order "o1"
    Then the dispute call is refused with code "already_exists"
    And the admin sees 1 disputes on order "o1"
    When the admin moves dispute "d1" to "INVESTIGATING"
    And the buyer escalates return "r1" of order "o1"
    Then the dispute call is refused with code "already_exists"
    And the admin sees 1 disputes on order "o1"

  Scenario: Dispute escalation: Open dispute cannot be resolved directly
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    And the admin moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then the dispute call is refused with code "failed_precondition"
    When the admin moves dispute "d1" to "RESOLVED"
    Then the dispute call is refused with code "failed_precondition"
    And dispute "d1" reads status "OPEN" with ruling "UNSPECIFIED"
    And dispute "d1" has 0 dispute.resolve audit events
    And dispute "d1" has 0 DisputeResolved facts on engagement.events
    When the admin moves dispute "d1" to "INVESTIGATING"
    Then the dispute call succeeds
    And dispute "d1" reads status "INVESTIGATING" with ruling "UNSPECIFIED"

  @destructive
  Scenario: Dispute escalation: Audit down blocks the ruling
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    And the admin moves dispute "d1" to "INVESTIGATING"
    And team-audit is stopped
    And the admin moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then the dispute call is refused with code "unavailable"
    And dispute "d1" reads status "INVESTIGATING" with ruling "UNSPECIFIED"
    And dispute "d1" has 0 DisputeResolved facts on engagement.events
    When team-audit is started again
    And the admin moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then the dispute call succeeds
    And dispute "d1" reads status "RESOLVED" with ruling "BUYER_FAVOUR"

  Scenario: Dispute escalation: The fact reaches Kafka once
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    And the admin moves dispute "d1" to "INVESTIGATING"
    Then dispute "d1" has 0 DisputeResolved facts on engagement.events
    When the admin moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then within 10 seconds engagement.events holds exactly 1 DisputeResolved envelope keyed by dispute "d1" for return "r1" with ruling "BUYER_FAVOUR"

  Scenario: Dispute escalation: Buyer and seller list the order's disputes newest first
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer opens a plain dispute "d1" on order "o1"
    And the buyer escalates return "r1" of order "o1" as dispute "d2"
    Then the buyer lists the disputes of order "o1" and gets "d2" then "d1"
    And the seller lists the disputes of order "o1" and gets "d2" then "d1"

  Scenario: Dispute escalation: Non-party is denied
    Given a seller and a buyer exist
    And a second buyer who did not place the order exists
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    And the second buyer lists the disputes of order "o1"
    Then the dispute call is refused with code "permission_denied"
    And the refused response holds no dispute
    And the buyer lists the disputes of order "o1" and gets "d1"

  Scenario: Dispute escalation: Admin lists open disputes oldest first
    Given a seller and a buyer exist
    And the buyer has a plain order "o1"
    When the buyer opens a plain dispute "d1" on order "o1"
    And the buyer opens a plain dispute "d2" on order "o1"
    And the buyer opens a plain dispute "d3" on order "o1"
    And the buyer opens a plain dispute "d4" on order "o1"
    And the admin moves dispute "d4" to "REJECTED"
    Then the admin lists OPEN disputes oldest first and finds "d1", "d2", "d3" in that order and not "d4"

  Scenario: Dispute escalation: Rejected return to resolved dispute end to end
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    And the admin moves dispute "d1" to "INVESTIGATING"
    And the admin moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then the dispute call succeeds
    And the buyer reads dispute "d1" as RESOLVED with ruling "BUYER_FAVOUR" and return "r1"
    And dispute "d1" has 2 dispute.resolve audit events whose last carries ruling "buyer_favour" and return "r1"

  Scenario: Dispute escalation: Return keeps its status until the refund consumer acts
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    Then the dispute call succeeds with dispute "d1" OPEN and linked to return "r1"
    When the admin moves dispute "d1" to "INVESTIGATING"
    Then the dispute call succeeds
    And return "r1" still reads REJECTED

  Scenario: Dispute escalation: A buyer cannot rule on their own dispute end to end
    Given a seller and a buyer exist
    And the buyer has order "o1" with a "rejected" return "r1"
    When the buyer escalates return "r1" of order "o1" as dispute "d1"
    Then the dispute call succeeds with dispute "d1" OPEN and linked to return "r1"
    When the buyer moves dispute "d1" to "RESOLVED" with ruling "BUYER_FAVOUR"
    Then the dispute call is refused with code "permission_denied"
    And dispute "d1" reads status "OPEN" with ruling "UNSPECIFIED"
    And dispute "d1" has 0 dispute.resolve audit events
    When the admin moves dispute "d1" to "INVESTIGATING"
    Then the dispute call succeeds
    And dispute "d1" has 1 dispute.resolve audit events
