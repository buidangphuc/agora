## Purpose

Defines that every `team-notification` RPC acts on the authenticated caller's own inbox, alert subscriptions and preferences,
so no caller reads or modifies another user's data and no shared demo identity exists.

## ADDED Requirements

### Requirement: Notification RPCs require a signed-in caller and act on that caller only

`team-notification` SHALL require an authenticated principal on `ListNotifications`, `MarkAsRead`, `GetUnreadCount`,
`SubscribeAlert`, `UnsubscribeAlert`, `ListAlertSubscriptions`, `GetNotificationPrefs` and `UpdateNotificationPrefs`, SHALL
derive the user id only from the principal id (never from a constant or the request), and SHALL treat a caller without a
principal or with the anonymous principal as `UNAUTHENTICATED` with nothing read or written. A user SHALL NOT be able to read,
mark, unsubscribe or change another user's notifications, subscriptions or preferences.

#### Scenario: Anonymous callers are rejected on every notification RPC

- **WHEN** an anonymous caller calls each of the eight RPCs
- **THEN** every call is `UNAUTHENTICATED` and no row is read or written

#### Scenario: Two users have separate inboxes

- **WHEN** buyer A and buyer B each list notifications and unread counts after a notification was delivered only to A
- **THEN** A sees it and counts it as unread; B sees none and a count of zero

#### Scenario: A user cannot mark another user's notification as read

- **WHEN** buyer B calls `MarkAsRead` with the id of a notification owned by A
- **THEN** A's notification remains unread

#### Scenario: Subscriptions and preferences are per user

- **WHEN** buyer A subscribes to a price-drop alert and updates their preferences, then buyer B lists subscriptions and reads preferences
- **THEN** B sees no subscription of A's and their own default preferences

#### Scenario: Alert delivery reaches the subscriber

- **WHEN** a buyer subscribes to a price-drop alert on a listing and the seller lowers the price
- **THEN** the notification appears in that buyer's own inbox

### Requirement: A signed-in buyer's normal notification use is unchanged

The frontend notification center and alert toggle SHALL keep working for a signed-in user without contract changes: the same
RPCs, request shapes and response shapes SHALL be returned for the caller's own data.

#### Scenario: The notification center renders for a signed-in buyer

- **WHEN** a signed-in buyer opens the notification center
- **THEN** their notifications, alert subscriptions and preferences are returned, and a logged-out visitor sees empty lists
