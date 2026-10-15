@tracking @analytics
Feature: The edge collector accepts activation and loop beacons and enforces the AI interaction allowlists
  POST /api/track accepts sign_up, login, follow_seller, referral_redeem, daily_check_in and
  alert_subscribe beacons, and accepts ai_interaction only with exactly the properties surface and
  action taken from fixed allowlists (gateway-loop-event-intake).

  Scenario Outline: Loop intake: each activation and loop beacon becomes a tracking event
    When a visitor posts a single "<type>" beacon with no properties
    Then the response is 202 with accepted 1 and dropped 0
    And exactly one EventEnvelope with EventType <event_type> reaches analytics.events

    Examples:
      | type            | event_type |
      | sign_up         | 15         |
      | login           | 16         |
      | follow_seller   | 17         |
      | referral_redeem | 18         |
      | daily_check_in  | 19         |
      | alert_subscribe | 20         |

  Scenario: Loop intake: properties of a loop beacon reach the event
    When a visitor posts a single "follow_seller" beacon with properties {"seller_id": "seller-42"}
    And a visitor posts a single "daily_check_in" beacon with properties {"streak": "7"}
    Then every response is 202
    And the event of the 1st beacon carries exactly the properties {"seller_id": "seller-42"}
    And the event of the 2nd beacon carries exactly the properties {"streak": "7"}

  @needsBuyer
  Scenario: Loop intake: the envelope principal is the signed-in buyer
    When the signed-in buyer posts a single "login" beacon with the session cookie and a body userId "someone-else"
    Then the response is 202 with accepted 1 and dropped 0
    And the published EventEnvelope principal is the buyer
    And the published EventEnvelope principal is not "someone-else"

  Scenario: Loop intake: an anonymous caller is attributed to the anonymous principal
    When a visitor posts a single "sign_up" beacon with no properties
    Then the response is 202 with accepted 1 and dropped 0
    And the published EventEnvelope principal is of type anonymous

  Scenario: Loop intake: an oversized loop property is rejected
    When a visitor posts a single "referral_redeem" beacon whose property value has 257 characters
    And a visitor posts a single "referral_redeem" beacon whose property value has 256 characters
    And a visitor posts a single "referral_redeem" beacon whose properties has 20 keys
    And a visitor posts a single "referral_redeem" beacon whose properties has 21 keys
    Then the responses are 400, 202, 202 and 400
    And only the beacons answered 202 reach analytics.events

  Scenario: Loop intake: existing beacon types are unchanged
    When a visitor posts a single "view" beacon with no properties
    And a visitor posts a single "share" beacon with no properties
    And a visitor posts a single "favorite" beacon with no properties
    And a visitor posts a single "select_item" beacon with no properties
    Then every response is 202
    And the events of the beacons have EventTypes 1, 10, 9 and 2

  Scenario: Loop intake: an unknown type is still rejected
    When a visitor posts a single "teleport" beacon with no properties
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: a valid AI interaction becomes a tracking event
    When a visitor posts a single "ai_interaction" beacon for listing "lst-1" with properties {"surface": "assistant", "action": "accepted"}
    Then the response is 202 with accepted 1 and dropped 0
    And exactly one EventEnvelope with EventType 21 reaches analytics.events
    And the event of the 1st beacon carries listing id "lst-1"
    And the event of the 1st beacon carries exactly the properties {"surface": "assistant", "action": "accepted"}

  Scenario: Loop intake: an AI interaction with a surface outside the allowlist is rejected
    When a visitor posts a single "ai_interaction" beacon with properties {"surface": "search_bar", "action": "shown"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: an AI interaction with an action outside the allowlist is rejected
    When a visitor posts a single "ai_interaction" beacon with properties {"surface": "assistant", "action": "typed_a_prompt"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: an AI interaction missing surface or action is rejected
    When a visitor posts a single "ai_interaction" beacon with no properties
    And a visitor posts a single "ai_interaction" beacon with properties {"surface": "assistant"}
    And a visitor posts a single "ai_interaction" beacon with properties {"action": "shown"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: AI allowlist matching is exact
    When a visitor posts a single "ai_interaction" beacon with properties {"surface": "Assistant", "action": "shown"}
    And a visitor posts a single "ai_interaction" beacon with properties {"surface": "assistant ", "action": "shown"}
    And a visitor posts a single "ai_interaction" beacon with properties {"surface": "assistant", "action": "SHOWN"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: an invalid AI beacon does not drop the valid ones in a batch
    When a visitor posts a batch of two "ai_interaction" beacons, the first with properties {"surface": "review_summary", "action": "shown"} and the second with properties {"surface": "free_text", "action": "shown"}
    Then the response is 202 with accepted 1 and dropped 1
    And only the first beacon of the batch reaches analytics.events

  Scenario: Loop intake: an AI interaction with an extra property key is rejected
    When a visitor posts a single "ai_interaction" beacon with properties {"surface": "assistant", "action": "shown", "prompt": "hello"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons

  Scenario: Loop intake: an extra-key AI beacon in a batch is dropped and counted
    When a visitor posts a batch of two "ai_interaction" beacons, the first with properties {"surface": "assistant", "action": "shown"} and the second with properties {"surface": "assistant", "action": "shown", "note": "x"}
    Then the response is 202 with accepted 1 and dropped 1
    And only the first beacon of the batch reaches analytics.events

  Scenario: Loop intake: an AI beacon type with odd casing is still validated
    When a visitor posts a single " AI_Interaction " beacon with properties {"surface": "search_bar", "action": "shown"}
    Then every response is 400
    And nothing is published to analytics.events for the posted beacons
