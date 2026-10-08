## Purpose

Defines how the feature store shows that its features are fresh and sane: per-feature freshness against a
declared SLO, distribution drift of feature values, and input quality gates that stop bad data from being
materialized. This is the light first version of `plans/mlops` P3-T4 for features (not models).

## ADDED Requirements

### Requirement: Every feature has a measured freshness against its SLO

The system SHALL expose, per materialized feature version and per store, the age of the newest successfully
written as-of time, and SHALL raise an alert when that age exceeds the version's freshness SLO for 5 minutes.
The alert SHALL name the feature reference, the store and the observed age.

#### Scenario: Freshness is visible per feature

- **WHEN** both materializers run normally
- **THEN** the metrics endpoint reports a freshness age for every non-pending feature version in each store,
  below its SLO

#### Scenario: A stalled online materializer raises an alert

- **WHEN** the online materializer stops and the freshness age of `item.trending_1h@v1` passes its SLO for 5
  minutes
- **THEN** the alert rule for online freshness fires for that reference

### Requirement: A down feature store or missing freshness series raises an alert

Value-comparing freshness rules are silent when their series are absent, so the system SHALL also alert when
team-analytics cannot be scraped or its online freshness series are missing: `TeamAnalyticsDown` SHALL fire
(severity warning) when `up{job="team-analytics"} == 0` for 5 minutes, and `FeatureFreshnessMissing` SHALL fire
(severity warning) when `absent(feature_freshness_seconds{store="online"})` holds for 10 minutes.

#### Scenario: Stopping team-analytics raises the target-down and missing-series alerts

- **WHEN** team-analytics is stopped while it was scraped and exporting online freshness series
- **THEN** `TeamAnalyticsDown` fires after 5 minutes of `up == 0` and `FeatureFreshnessMissing` fires once the
  series have been absent for 10 minutes, while `FeatureOnlineStale` stays silent

#### Scenario: A healthy feature store raises neither alert

- **WHEN** team-analytics is scraped and exports online freshness series normally
- **THEN** neither `TeamAnalyticsDown` nor `FeatureFreshnessMissing` fires

### Requirement: Feature distributions are monitored for drift

The system SHALL compute, once per offline partition, the population stability index of each numeric stable
feature version against the trailing 7-day reference of the same version, and SHALL raise a warning above 0.2.
The alert SHALL carry the feature reference and both windows. Drift SHALL never stop materialization or change
values.

#### Scenario: A shifted distribution raises a drift warning

- **WHEN** a synthetic day of traffic multiplies the views of half the catalog by ten
- **THEN** the drift value of `item.views_7d@v1` for that partition is above 0.2 and the drift warning fires
  naming the reference and the reference window

#### Scenario: A stable distribution raises nothing

- **WHEN** a synthetic day of traffic has the same distribution as the previous week
- **THEN** the drift value is below 0.1 and no drift warning fires

### Requirement: Input quality gates block materialization of bad data

The system SHALL check each materialization run's input before computing: share of rows with an empty entity
key, share of duplicate event ids, event volume against the trailing 7-day median for the same hour,
occurrence times in the future beyond 5 minutes, and any data-quality failure reported for the same source
events by the tracking pipeline. When a check fails, the features reading those source events SHALL NOT be
written for that run (the online store keeps its last values with their as-of times; the offline partition is
not published), and a quality alert SHALL name the check and the affected features.

#### Scenario: A volume collapse blocks the run

- **WHEN** the event volume of the last hour is below 10% of the trailing 7-day median for that hour
- **THEN** the run does not write the affected features, the online values keep their previous as-of times,
  and the quality alert names the volume check

#### Scenario: Future timestamps block the run

- **WHEN** more than 1% of the run's input rows have an occurrence time more than 5 minutes after their
  ingestion time
- **THEN** the run does not write the affected features and the quality alert names the timestamp check

#### Scenario: A healthy run passes every gate

- **WHEN** the input has normal volume, no empty keys and no future timestamps
- **THEN** every gate passes and the run writes its features
