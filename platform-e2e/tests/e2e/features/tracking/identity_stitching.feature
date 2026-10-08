@tracking @analytics
Feature: Anonymous history is stitched to the account that logs in
  The warehouse views tracking_identity and tracking_events_resolved give every tracking event a
  user_key: the USER principal when present, else the only principal its anonymous id was
  seen with (an id seen with two accounts is ambiguous and not stitched), else anon:<anonymous_id> (tracking-ingest-integrity).

  Scenario: Pre-login browsing resolves to the user after login
    When a visitor posts a view anonymously with anonymousId X, then logs in as a new buyer and posts a view with the same anonymousId X
    Then in tracking_events_resolved both views have user_key equal to the buyer's id

  Scenario: An anonymous id seen with two accounts is not stitched
    When a visitor posts a view anonymously with anonymousId Z, then two different new buyers each post a view with the same anonymousId Z
    Then in tracking_events_resolved the anonymous view keeps "anon:Z" and each buyer's view has that buyer's id

  Scenario: A never-logged-in visitor keeps an anonymous key
    When a visitor posts a view anonymously with a fresh anonymousId Y and never logs in
    Then in tracking_events_resolved that view has user_key "anon:Y"
