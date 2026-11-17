@fintech
Feature: The seller fee RPC is not reachable from the public gateway
  ChargeSellerFee is service-to-service only. No end-user token can book a fee against a wallet
  through the gateway. (payment-seller-fee-charge)

  Scenario: The gateway does not expose the fee RPC
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" and the seller is credited
    When the seller calls ChargeSellerFee through the gateway with a fresh reference
    Then the gateway answers HTTP 404 to the fee call
    And the seller's ledger has no SUBSCRIPTION_FEE entry
    And the seller's ledger is unchanged by the fee call
