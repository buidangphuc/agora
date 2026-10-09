@modelserve @needsModelserveOverlay
Feature: Model serving router (platform-modelserve)
  The platform-modelserve router on :8100 fronts the inference runtimes. Here it runs from
  platform-e2e/compose/modelserve.override.yaml with a deterministic TEI fake as every upstream
  (the fake also plays vLLM), so each scenario observes the router's contract on the wire and the
  upstream's own request log. Change: add-platform-modelserve, capability model-serving.

  Scenario: Router serves embeddings conforming to team-ai parser
    When a client posts "/embed" to the router with two unique texts under the key "texts"
    Then the router answers 200 with embeddings that pass the team-ai vector validation at dimension 384

  Scenario: Router accepts OpenAI format embeddings request
    When a client posts "/v1/embeddings" to the router with the input "hello world" and a unique word
    Then the router answers 200 with an OpenAI list holding one 384 dimension embedding

  Scenario: Embedding cache hit skips upstream inference
    Given the router has embedded a unique text once and the upstream saw exactly one call for it
    When a client posts the same text to "/embed" again
    Then the router returns the same vector and the upstream saw no second call for that text

  Scenario: Multi-text embedding splits cache hits and misses
    Given the router has cached the vectors of two unique texts
    When a client posts those two texts and two new ones in an interleaved order
    Then the upstream received only the two new texts in their request order
    And the router returned four vectors in the request order
    And posting the same four texts again reaches the upstream not at all

  Scenario: Router proxies rerank requests
    When a client posts a rerank request for "shoes" with the texts "red sneakers" and "blue jacket" to the router
    Then the router returns the ranked results of the upstream with the sneakers first
    And the upstream received the rerank payload unchanged

  Scenario: Router proxies chat completions to vLLM
    When a client posts an OpenAI chat completion with a unique message to the router
    Then the router returns the completion of the vLLM upstream stand-in
    And the upstream received the chat payload unchanged

  @destructive
  Scenario: Backpressure on queue depth saturation
    When 40 slow embedding requests arrive at the router at the same moment
    Then some of them are answered 429 with a Retry-After header
    And the admitted ones still complete with 200
    And the router serves a normal request once the load has drained
