@tracking
Feature: Free text is scrubbed of personal data at the edge
  Before producing, the gateway masks emails, Vietnamese phone numbers and long digit runs in the
  free-text fields and reduces the referrer to scheme, host and path, so personal data never lands
  on analytics.events (tracking-ingest-integrity).

  Scenario: An email and a phone in a search query are masked
    When a visitor posts a view whose query is "a.b@example.com 0912 345 678 tủ lạnh 850000"
    Then the event on analytics.events has search_query "[email] [phone] tủ lạnh 850000"

  Scenario: A referrer loses its query string
    When a visitor posts a view whose referrer is "https://news.example.com/a/b?utm_source=x&email=a@b.co#top"
    Then the event on analytics.events has referrer "https://news.example.com/a/b"
