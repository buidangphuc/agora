Feature: A stream ends when its token expires or its session is revoked
  team-gateway authenticates a StreamChat when it opens and then watches the stream: it ends it
  with the Connect error code unauthenticated when the token's exp passes, or shortly after the
  token's session is revoked. The model provider is the e2e LLM fake told to hold every call
  open, so the stream is still open when the credential stops being valid. The revocation
  scenario runs on a private gateway (stack image and env) that checks revocation every second.
  Change: authz-residuals-2 (edge-stream-and-http-policy). Needs the rebuilt gateway image.

  Scenario: A stream is ended when its token expires
    Given a buyer token that expires in 3 seconds
    When the buyer opens a StreamChat that the model provider holds open
    Then the stream ends with the Connect error code unauthenticated before the provider's own timeouts end it

  Scenario: A stream is ended when its session is revoked
    Given a buyer who is signed in on two devices and a gateway that checks revocation every second
    When the buyer opens a StreamChat on the other device that the model provider holds open and then revokes that device's session
    Then the stream ends with the Connect error code unauthenticated within 4 seconds of the revoke

  Scenario: A stream that finishes before expiry is unaffected
    Given a buyer token that expires in 60 seconds
    When the buyer calls StreamChat and the provider answers at once
    Then the stream completes with chat text and no error
