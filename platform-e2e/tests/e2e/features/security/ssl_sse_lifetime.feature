Feature: An SSE connection ends when its token expires or its session is revoked
  team-gateway authorizes /api/events/live when the connection opens and then watches it: for a
  room that needs a credential it sends one final `unauthenticated` event and closes the
  connection when the token's exp passes, or shortly after the token's session is revoked. Public
  rooms need no credential and are not cut. The revocation scenario runs on a private gateway
  (stack image and env) that checks revocation every second.
  Change: sse-stream-lifetime (edge-stream-and-http-policy). Needs the rebuilt gateway image.

  Scenario: An SSE connection is ended when its token expires
    Given a buyer token that expires in 3 seconds for the SSE route
    When the buyer opens the SSE route for their own user room and keeps it open
    Then the gateway sends an unauthenticated event and closes the connection about when the token expires

  Scenario: An SSE connection is ended when its session is revoked
    Given a buyer who is signed in on two devices and a gateway that checks revocation every second for the SSE route
    When the buyer opens the SSE route for their own user room on the other device and then revokes that device's session
    Then the gateway sends an unauthenticated event and closes the connection within 4 seconds of the revoke

  Scenario: A public SSE room is unaffected by token expiry
    Given an anonymous client with no credential
    When the client opens the SSE route for a public listing room and stays connected for 6 seconds
    Then the connection is still open and received no unauthenticated event
