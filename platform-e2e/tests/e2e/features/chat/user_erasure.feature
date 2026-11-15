@chat
Feature: team-chat replaces an erased user's messages on UserErased
  A UserErased event on identity.events replaces the text of every message the user sent with a
  placeholder, empties rich payloads, keeps the thread and the counterparty's messages and fixes the
  thread preview. The event is produced to the real topic; the counterparty reads the result through
  GetThreadMessages and ListThreads.
  (change content-user-erasure)

  Scenario: The user's messages are replaced and the thread is kept
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    When a UserErased event for "U" is consumed
    Then the 3 messages of "U" read "Tin nhắn đã bị xóa" with sender name "Người dùng đã xóa", the 2 messages of "S" are unchanged and the thread still lists for "S"

  Scenario: A rich message payload is emptied
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent an image message with a payload in a thread with "S" on "L"
    When a UserErased event for "U" is consumed
    Then the image message of "U" has an empty payload and reads "Tin nhắn đã bị xóa"

  Scenario: The thread preview no longer shows erased text
    Given a seller "S" with a listing "L"
    And a seller listing "L2" owned by "S"
    And a buyer "U" who sent the last message of a thread with "S" on "L"
    And a buyer "U" whose thread with "S" on "L2" ends with a message from "S"
    When a UserErased event for "U" is consumed
    Then the preview of the thread on "L" reads "Tin nhắn đã bị xóa" and the preview of the thread on "L2" is unchanged

  Scenario: Other users' messages are untouched
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    And a buyer "B" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    And the thread of "B" is recorded
    When a UserErased event for "U" is consumed
    Then the 3 messages of "U" read "Tin nhắn đã bị xóa" with sender name "Người dùng đã xóa", the 2 messages of "S" are unchanged and the thread still lists for "S"
    And the messages and thread of "B" are unchanged

  Scenario: Erasure is reachable only through the identity topic
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    And a buyer "X"
    When a UserErased record for "U" is produced to a topic other than identity.events
    And "X" calls chat RPCs naming "U" and the thread of "U"
    Then nothing of "U" is erased or changed

  Scenario: A replayed event changes nothing
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    When a UserErased event for "U" is consumed
    Then the 3 messages of "U" read "Tin nhắn đã bị xóa" with sender name "Người dùng đã xóa", the 2 messages of "S" are unchanged and the thread still lists for "S"
    When the same UserErased event for "U" and then one with a new event id are consumed
    Then the chat erasure consumer completes both without error and "U" is unchanged

  Scenario: A malformed event goes to the DLQ
    Given a seller "S" with a listing "L"
    And a buyer "U" who sent 3 messages in a thread with "S" on "L" where "S" sent 2
    When a UserErased event with an empty user_id and a record that does not decode are produced
    Then both records are written to the chat DLQ and nothing is applied to "U"
