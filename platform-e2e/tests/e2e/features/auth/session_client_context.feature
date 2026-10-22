@auth @buyer
Feature: Session client context on refresh
  As a signed-in shopper
  I want the Active sessions page to show the device that keeps my session alive
  So that I can spot a session being used from somewhere unexpected

  # change: frontend-session-client-context. The other scenarios of the change are unit-verified
  # (container-to-gateway headers and identity internals are not observable from a browser).
  Scenario: Session client context: rotation updates device and IP
    Given a registered buyer who signed in through the login form from user agent "AgoraTest/1"
    And the Active sessions page lists the session with device "AgoraTest/1"
    When the session cookie expires and the next page load comes from user agent "AgoraTest/2"
    Then the Active sessions page lists that session with device "AgoraTest/2"
    And the Active sessions page has no entry with device "AgoraTest/1"
    And the stored session has device "AgoraTest/2", a non-empty IP and a last-seen time later than its created time
