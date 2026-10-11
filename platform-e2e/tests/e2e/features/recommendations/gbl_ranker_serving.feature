@recommendations @ai
Feature: Serving ranks with the trained GBDT ranker of its generation (recsys-gbdt-trainer)
  team-ai reads `recs:v1:gen:<generation>:ranker`, the artifact the recsys job publishes for the serving generation, and
  ranks GBDT placements with it. The scenario writes that key as the job would and reads the order through the gateway.
  Needs the rebuilt team-ai image and a serving generation pointer in the stack Redis DB 0.

  Scenario: A published ranker artifact changes the home feed order
    Given a buyer is logged in
    When the serving generation carries a ranker artifact that scores the second of two home-feed candidates higher, and the candidates' online features exist
    Then a Recommend call through the gateway ranks the second above the first
    And without the artifact the same candidates were ranked in their own order

  Scenario: A ranker with another feature list is ignored
    Given a buyer is logged in
    When the serving generation carries a ranker artifact that would score the second candidate higher but lists other features
    Then every Recommend call through the gateway keeps the candidates' own order
