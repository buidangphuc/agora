@tracking @analytics
Feature: Anonymous history is stitched to the account that logs in
  The warehouse views tracking_identity and tracking_events_resolved give every tracking event a
  user_key: the USER principal when present, else the principal its anonymous id last logged in
  as, else anon:<anonymous_id> (tracking-ingest-integrity).

  Scenario: Pre-login browsing resolves to the user after login
    When a visitor posts a view anonymously with anonymousId X, then logs in as a new buyer and posts a view with the same anonymousId X
    Then in tracking_events_resolved both views have user_key equal to the buyer's id

  Scenario: A never-logged-in visitor keeps an anonymous key
    When a visitor posts a view anonymously with a fresh anonymousId Y and never logs in
    Then in tracking_events_resolved that view has user_key "anon:Y"
