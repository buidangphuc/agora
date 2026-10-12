@recommendations @recsys @batch @integration
Feature: Recommendation inputs honour personalisation consent (recsys-consent-and-context)
  Personalisation consent (TrackingEvent.consent.personalization, version 0 = no cookie = denied) decides which
  events may feed per-buyer recommendation data. The featurestore dataset job runs as the real image on synthetic
  warehouse inputs; the nearline consumer runs as the real recsys image against a scratch Kafka topic and the
  stack's Redis; the export scenario drives beacons through the gateway and reads the analytics volume. Needs the
  rebuilt platform-featurestore, platform-recsys and team-analytics images.

  Scenario: Denied buyer has no interaction rows
    Given a buyer u-deny with 10 view events all with consent_personalization false and a buyer u-ok with 3 with true
    When the governed datasets are built
    Then als_interactions has 0 rows for u-deny and at least 1 row for u-ok

  Scenario: Only post-grant events are used after a later grant
    Given a buyer u-late with 4 denied events at T1 and 2 granted events at T2 later than T1
    When the governed datasets are built
    Then the als_interactions row for u-late counts interactions 2 and its last_occurred_at is T2

  Scenario: Denied buyer is served popular recommendations
    Given a buyer is logged in
    When the signed-in buyer with no granted events calls Recommend for the home feed through the gateway
    Then the Recommend response lists at least 1 listing id

  Scenario: Denied event changes no per-user key
    Given a nearline topic holding a denied view of actor u-deny and then a granted view of actor u-ok
    When the nearline consumer drains the topic
    Then no recs:nearline:user key, session key or co-view key exists for the denied view
    And the consumer reports consent_denied 1
    And the granted actor's recents list the viewed listing at rank 1

  Scenario: Checkout events raise category affinity
    Given a nearline topic holding a view_cart then a begin_checkout of a books listing from a consented actor
    When the nearline consumer drains the topic
    Then the actor's category affinity for books is 9.0

  Scenario: Exported file has consent and context columns
    Given a GA4 export for today with a "spring" session and a "summer" session for two visitors
    And the ga4import job has run for the fixture date
    When one visitor posts 4 views and the other visitor posts 2 views, each with their GA client id
    And the export cycle completes
    Then the exported tracking Parquet has the seven consent and context columns
    And the spring visitor's exported rows carry the spring session source, medium and campaign and consent personalization false with version 1
