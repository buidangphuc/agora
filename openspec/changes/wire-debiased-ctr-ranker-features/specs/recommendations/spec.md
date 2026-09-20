## ADDED Requirements

### Requirement: Ranker features use position-debiased CTR

Candidate feature extraction SHALL source `historical_ctr` from the nearline position-debiased
CTR when a value is available for that item, so that ranking is not driven by raw click-through
rates inflated by favourable display positions.

#### Scenario: Equal raw CTR, worse positions, higher debiased CTR

- **WHEN** two candidates have accumulated identical raw click-through rates, but one item's
  impressions occurred at consistently worse positions
- **THEN** the item shown at worse positions receives the higher `historical_ctr` feature value

#### Scenario: Debiased value changes the ranking score

- **WHEN** the candidates above are scored by the GBDT ranker
- **THEN** the item with the higher debiased CTR receives the higher ranking score

### Requirement: CTR source is recorded

The feature vector SHALL record which source produced `historical_ctr`, so that training-time and
serving-time feature provenance can be compared.

#### Scenario: Nearline data present

- **WHEN** the nearline store holds a debiased CTR for the candidate
- **THEN** the feature vector reports `ctr_source` as `"nearline"`

#### Scenario: Nearline data absent

- **WHEN** the nearline store holds no usable data for the candidate
- **THEN** `historical_ctr` retains its prior value
- **AND** the feature vector reports `ctr_source` as `"fallback"`

### Requirement: Serving path supplies the nearline source

The recommendation serving path SHALL provide the nearline signal source during candidate
enrichment, so that debiased CTR reaches the ranker at request time and not only in offline
training.

#### Scenario: Serving request enriches from nearline

- **WHEN** a recommendation request runs for a placement whose ranking model is `gbdt`
- **THEN** candidate features were built with the nearline source
- **AND** the response `explain` payload reports the nearline enrichment
