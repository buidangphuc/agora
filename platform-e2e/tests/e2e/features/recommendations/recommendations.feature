@recommendations @buyer
Feature: "Gợi ý cho bạn" recommendations row (surface-recommendations)
  As a logged-in buyer, a "Gợi ý cho bạn" recommendations row is shown on the
  home page (context HOMEPAGE) and the product-detail page (context
  SIMILAR_ITEMS, seeded with the current listing). It is populated from team-ai
  (RecommendationService/Recommend) through the gateway using the caller's
  session — never a client-side mock and never a direct browser call to team-ai.
  When the service is UNAVAILABLE (e.g. RECS_ENABLED=false) the page still
  renders and the row is hidden rather than erroring.

  # Depends on serve-recommendations-teamai (team-ai answers Recommend) and the
  # gateway RecommendationForwarder; green run needs the live stack + team-ai
  # RECS_ENABLED=true with the training-job Qdrant/Redis data.

  @needsBuyer
  Scenario: Recommend routes through the gateway to team-ai
    Given a buyer is logged in
    When the frontend calls Recommend through the gateway with the caller's session
    Then the gateway forwards the request to team-ai over gRPC with the x-principal-* metadata
    And the gateway returns team-ai's product list unchanged with no gateway-side business logic

  @needsBuyer
  Scenario: Logged-in buyer sees a recommendations row sourced from team-ai
    Given a buyer is logged in
    When I navigate to the "home" page
    Then the "Gợi ý cho bạn" recommendations row is populated with product cards
    And the row is sourced from team-ai via the gateway, not a client-side mock

  @needsBuyer @needsListing
  Scenario: The product-detail recommendations row is seeded by the current listing
    Given a buyer is logged in
    And a listing has been seeded via the API
    When the buyer opens the seeded listing
    Then the "Gợi ý cho bạn" recommendations row is populated with product cards
    And the row is seeded with the current listing via the gateway to team-ai

  @needsBuyer
  Scenario: The browser never calls team-ai directly for recommendations
    Given a buyer is logged in
    When I navigate to the "home" page
    Then the recommendations came through the gateway Recommend RPC
    And the browser markup carries no team-ai address or recommendation service client

  @needsBuyer
  Scenario: Recommendations are unavailable without breaking the page
    Given a buyer is logged in
    When the recommendation service is unavailable
    And I navigate to the "home" page
    Then the home page still renders
    And the "Gợi ý cho bạn" row is hidden or empty rather than erroring the page
