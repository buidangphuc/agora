@recommendations @ai
Feature: Serving ranks with the nearline position-debiased CTR (add-recsys-nearline-signals, wire-debiased-ctr-ranker-features)
  team-ai reads the position-debiased CTR that the recsys nearline consumer keeps in Redis
  (`recs:nearline:ctr:<listing_id>`) and ranks GBDT placements with it. The scenarios write that hash
  as the consumer would and read the order through the gateway. They need the team-ai image built with
  the nearline reader and `RECS_NEARLINE_REDIS_URL` pointing at the stack's Redis DB 0.

  Scenario: Serving ranks with the nearline CTR written by the consumer
    Given a buyer is logged in
    When two home-feed candidates have equal model scores and only the second has a nearline CTR in Redis
    Then a Recommend call through the gateway ranks the second above the first
    And with no nearline row for either, the order is the candidates' own order

  Scenario: Debiased value changes the ranking score
    Given a buyer is logged in
    When two candidates have equal model scores and equal raw click-through but the second's clicks came from worse positions
    Then the second ranks above the first in the Recommend response
