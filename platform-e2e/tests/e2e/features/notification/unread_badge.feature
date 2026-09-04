@buyer @needsBuyer
Feature: Unread badge counter
  As a user, my unread notifications are displayed on the header badge.

  Scenario: Unread notification count is reflected on header
    Given a user has unread notifications
    Then the header badge shows the unread count
