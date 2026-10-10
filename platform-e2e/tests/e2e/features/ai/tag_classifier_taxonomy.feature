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
    Given an exploring candidate tag with a wattage no earlier run used
    When promotion is executed with target category "cat-electronics" and a bound synonym
    Then the tag status becomes PROMOTED and is active for search filter facets

  Scenario: Anonymous tag requests are refused
    Given an exploring candidate tag with a wattage no earlier run used
    When a caller without credentials calls each of the five tag routes
    Then every call answers 401 and the taxonomy is unchanged

  Scenario: A buyer's token is refused
    Given an exploring candidate tag with a wattage no earlier run used
    When a signed-in buyer presents the gateway session token to a read route and to promote
    Then both answer 401 and the taxonomy is unchanged

  Scenario: A read-only service principal cannot mutate the taxonomy
    Given an exploring candidate tag with a wattage no earlier run used
    When the service principal classifies and lists tags, then calls explore and promote
    Then classify and list answer 200, explore and promote answer 403, and no candidate is registered or promoted

  Scenario: An admin principal explores and promotes
    When an admin principal explores a batch and promotes the discovered candidate
    Then both answer 200 and the tag is promoted and canonical

  @destructive
  Scenario: Promoted and exploring tags survive a restart
    Given a promoted tag with a bound synonym and a second candidate left exploring
    When team-ai is restarted
    Then the promoted tag is canonical with its synonym and the other is still an exploring candidate
