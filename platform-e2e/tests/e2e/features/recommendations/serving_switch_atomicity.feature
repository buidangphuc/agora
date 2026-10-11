@recsys @recommendations
Feature: One serving pointer decides the Redis lists and the Qdrant vectors (serving-switch-atomicity)
  team-ai names the Qdrant collection of the generation `recs:v1:serving` names, instead of reading the
  `item_als_vectors` alias that recsys moves a moment before the pointer. A crash between the two
  switches, or a lagging alias, can no longer mix generation N lists with generation N+1 vectors.

  # Every scenario but the last rewrites the shared serving pointer or aliases, so it is @destructive
  # (serial lane). Each generation's item collection gets a marker point (listing id
  # `ssa-marker-<model_version>`) identical to the seed listing's vector, so the generation a
  # similar-items answer was read from is unambiguous: the marker is the nearest neighbour.

  @destructive
  Scenario: A similar-items request reads the vectors of the serving generation
    Given a buyer is logged in
    And two models have been promoted
    And each generation's item collection holds a marker next to the seed listing
    When the buyer opens similar items for the seed listing
    Then the answer is read from the second model's collection

  @destructive
  Scenario: An alias ahead of the pointer does not change what is served
    Given a buyer is logged in
    And two models have been promoted
    And each generation's item collection holds a marker next to the seed listing
    When the item alias is moved to the second model's collection while `recs:v1:serving` still names the first
    Then similar items through the gateway are read from the first model's collection

  @destructive
  Scenario: An alias behind the pointer does not change what is served
    Given a buyer is logged in
    And two models have been promoted
    And each generation's item collection holds a marker next to the seed listing
    When `recs:v1:serving` names the second model while the item alias still points at the first model's collection
    Then similar items through the gateway are read from the second model's collection

  @destructive
  Scenario: Without a pointer the alias is used
    Given a buyer is logged in
    And two models have been promoted
    And each generation's item collection holds a marker next to the seed listing
    When `recs:v1:serving` is absent and the item alias points at the first model's collection
    Then similar items through the gateway are read from the first model's collection

  @destructive
  Scenario: Serving and previous collections survive retention without an alias
    Given a buyer is logged in
    And two models have been promoted
    When a third model is promoted and the aliases are then deleted and a fourth model is promoted
    Then the item and user collections of the third and fourth model still exist
    And no collection of the first and second model remains

  @destructive
  Scenario: Rollback restores the earlier model's vectors
    Given a buyer is logged in
    And two models have been promoted
    And each generation's item collection holds a marker next to the seed listing
    When `python -m recsys rollback` runs
    Then similar items through the gateway are read from the first model's collection

  Scenario: The team-ai container does not hold listing's credentials
    When the running team-ai container's environment is inspected
    Then none of its POSTGRES values is a listing service credential
