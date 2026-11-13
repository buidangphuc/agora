@notification
Feature: team-notification deletes an erased user's notification data on UserErased
  A UserErased event on identity.events deletes the user's notifications, alert subscriptions and
  notification preferences and nothing of another user. The event is produced to the real topic;
  everything is read back through ListNotifications, ListAlertSubscriptions and GetNotificationPrefs.
  (change content-user-erasure)

  Scenario: The user's notification data is deleted
    Given a seller "S" with a listing "L"
    And a buyer "U" with notifications, 2 alert subscriptions on "L" and a preferences row
    When a UserErased event for "U" is consumed
    Then "U" has no notifications, no alert subscriptions and default preferences

  Scenario: Other users' notifications are untouched
    Given a seller "S" with a listing "L"
    And a buyer "U" with notifications, 2 alert subscriptions on "L" and a preferences row
    And a buyer "B" with notifications, 2 alert subscriptions on "L" and a preferences row
    And the notification data of "B" is recorded
    When a UserErased event for "U" is consumed
    Then "U" has no notifications, no alert subscriptions and default preferences
    And the notification data of "B" is unchanged

  Scenario: Erasure is reachable only through the identity topic
    Given a seller "S" with a listing "L"
    And a buyer "U" with notifications, 2 alert subscriptions on "L" and a preferences row
    And a buyer "X"
    When a UserErased record for "U" is produced to a topic other than identity.events
    And "X" calls notification RPCs naming "U" and a notification of "U"
    Then nothing of "U" is erased or changed

  Scenario: A replayed event changes nothing
    Given a seller "S" with a listing "L"
    And a buyer "U" with notifications, 2 alert subscriptions on "L" and a preferences row
    When a UserErased event for "U" is consumed
    Then "U" has no notifications, no alert subscriptions and default preferences
    When the same UserErased event for "U" and then one with a new event id are consumed
    Then the notification erasure consumer completes both without error and "U" is unchanged

  Scenario: A malformed event goes to the DLQ
    Given a seller "S" with a listing "L"
    And a buyer "U" with notifications, 2 alert subscriptions on "L" and a preferences row
    When a UserErased event with an empty user_id and a record that does not decode are produced
    Then both records are written to the notification DLQ and nothing is applied to "U"
