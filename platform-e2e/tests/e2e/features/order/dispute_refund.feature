@order @engagement
Feature: A buyer-favour dispute ruling refunds the rejected return
  team-order consumes DisputeResolved from engagement.events. A buyer-favour ruling moves the
  return to APPROVED and, for an order paid online, to REFUNDED, so team-payment refunds once; the
  refund is audited as return.dispute_refund before any change and waits for team-audit when it
  is down. Everything runs through the gateway with real users. (order-dispute-refund, initiative
  q6-trust-enforcement)

  Scenario: Dispute refund: Ruling reaches the refund end to end
    Given a seller and a buyer with a Paid-online order whose return was rejected and escalated
    When the admin rules the dispute BUYER_FAVOUR
    Then within 15 seconds the buyer reads the return as REFUNDED
    And the payment of the order shows one return refund of the return amount
    And exactly one return.dispute_refund audit event names the admin for the return

  Scenario: Dispute refund: Seller-favour ruling leaves the return untouched end to end
    Given a seller and a buyer with a Paid-online order whose return was rejected and escalated
    When the admin rules the dispute SELLER_FAVOUR
    Then the dispute reads RESOLVED with ruling SELLER_FAVOUR and its DisputeResolved fact is on engagement.events
    And for 10 seconds the return stays REJECTED
    And the audit log holds no return.dispute_refund event for the return
    And the payment of the order shows no refund

  @destructive
  Scenario: Dispute refund: Recovery after audit comes back
    Given a seller and a buyer with a Paid-online order whose return was rejected and escalated
    When team-audit is stopped
    And a buyer-favour DisputeResolved fact for the dispute is produced to engagement.events
    Then for 20 seconds the return stays REJECTED
    When team-audit is started again
    Then within 30 seconds the buyer reads the return as REFUNDED
    And exactly one return.dispute_refund audit event names the admin for the return
    And the engagement.events.order.dlq topic holds no record for the dispute
