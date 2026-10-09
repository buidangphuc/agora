@recommendations @ai
Feature: Recommendation serving degrades, cold-starts and identifies itself (recs-serving-safeguards)
  team-ai answers Recommend even when its cache is down, cold-starts from the producer's popular
  list, refuses an in-process catalogue outside local, serves Recommend from every gRPC entrypoint,
  stamps each response with the placement and a fresh request id, and ranks with online features.

  # Destructive scenarios stop the stack's Redis, rewrite serving / feature-store keys (saved and
  # restored exactly in teardown), or start throwaway containers. They run in the serial lane.

  @destructive
  Scenario: Recommendations survive a Redis outage
    Given a buyer is logged in
    When the Redis used by team-ai is stopped and a buyer requests homepage recommendations through the gateway
    Then the call succeeds with model_version "serving-fallback"

  @destructive
  Scenario: A new buyer gets the popular list
    Given a buyer is logged in
    When the serving generation's popular list is L and a buyer with no recommendations of their own requests homepage recommendations
    Then the returned listing ids are the first items of L, in L's order

  @destructive
  Scenario: Production refuses the memory recommendation backend
    When the team-ai image is started with ENVIRONMENT=production, RECS_ENABLED=true and RECS_BACKEND=memory
    Then the process exits non-zero and its log names RECS_BACKEND

  @destructive
  Scenario: The standalone gRPC server answers Recommend
    Given a buyer is logged in
    When team-ai is started with scripts/run_grpc.py and a buyer requests recommendations through a gateway pointed at it
    Then the call succeeds instead of answering unimplemented

  Scenario: Two calls get distinct request ids and the served placement
    Given a buyer is logged in
    When a buyer requests homepage recommendations twice through the gateway
    Then both responses have placement_id "home_feed" and two different non-empty request_ids, and team-ai logged a recs.served line for each

  @destructive
  Scenario: Online features break a tie
    Given a buyer is logged in
    When two candidates have equal model scores and only the second has item_popularity features with ctr_7d 0.5
    Then the second ranks above the first in the response
