@ai @seller @search
Feature: Product Tag Classifier & Taxonomy Filter Enrichment
  As a seller and buyer, AI classifies SPU and SKU-level product tags to enable precise faceted search.

  Scenario: SPU Level Tag Classification
    Given a seller provides product title "Tai nghe Bluetooth 5.3 chống ồn ANC sạc nhanh 65W GaN"
    When the seller requests tag classification for category "cat-electronics"
    Then the system returns canonical tags "bluetooth-5-3", "chong-on-chu-dong-anc", "sac-nhanh-65w-gan"
    And the tags are mapped to facet groups "connectivity", "feature", "power"

  Scenario: Granular SKU Level Hierarchical Classification
    Given a parent listing "iPhone 15 Pro Max Khung Titanium Chống nước IPX7"
    And child SKU variants "Titan Tự Nhiên / 256GB" and "Xanh Navy / 512GB"
    When hierarchical SKU classification is executed
    Then each SKU inherits common tags and receives specific variant facets for color and capacity
    And an OpenSearch nested document payload is generated

  Scenario: Candidate Tag Discovery from Raw Batch
    Given a batch of unclassified listings with emergent specs
    When offline exploration is executed with frequency threshold 2
    Then candidate tags are discovered and registered with status EXPLORING

  Scenario: Candidate Tag Promoted to Canonical Filter Facet
    Given an exploring candidate tag "cong-suat-100w"
    When promotion is executed with synonyms "sac 100w"
    Then the tag status becomes PROMOTED and is active for search filter facets
