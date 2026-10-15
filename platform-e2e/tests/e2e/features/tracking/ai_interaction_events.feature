@tracking @ai
Feature: AI surfaces report shown, accepted and edited as ai_interaction beacons with no free text
  The shopping assistant, the seller chat copilot, Magic Listing and the PDP review summary each send
  `ai_interaction` beacons (surface, action and at most a listing id) through the tracking queue to the
  gateway. team-ai runs in its default mock mode, never a real LLM (frontend-ai-interaction-events).
  Scenario names equal the spec scenario titles; the beacons are captured in the browser.

  # ── Shopping assistant ────────────────────────────────────────────────

  @needsBuyer
  Scenario: A reply with product cards reports shown once
    Given a buyer is logged in
    And the browser records AI beacons
    When the buyer asks the assistant for a product and a product card is shown
    And the buyer types in the assistant input box without sending
    Then exactly 1 "assistant" "shown" AI events are captured
    And zero "assistant" "accepted" AI events are captured

  @needsBuyer
  Scenario: Clicking a product card reports accepted with the listing
    Given a buyer is logged in
    And the browser records AI beacons
    When the buyer asks the assistant for a product and a product card is shown
    And the buyer clicks "Xem Ngay" on the first product card
    Then the buyer lands on the listing page of that card
    And exactly 1 "assistant" "accepted" AI events are captured
    And every captured "assistant" "accepted" AI event carries the id of the clicked card's listing
    When the buyer asks the assistant for a product again and clicks "Mua Ngay" on the first product card
    Then the buyer lands on the checkout page
    And exactly 2 "assistant" "accepted" AI events are captured
    And every captured "assistant" "accepted" AI event carries the id of the clicked card's listing

  @needsBuyer
  Scenario: Two replies report two shown events
    Given a buyer is logged in
    And the browser records AI beacons
    When the buyer asks the assistant for a product and a product card is shown
    And the buyer asks the assistant a second question and more product cards are shown
    Then exactly 2 "assistant" "shown" AI events are captured

  # ── Seller chat copilot ───────────────────────────────────────────────

  Scenario: Suggestions render and are reported once
    Given a seller has a chat thread with a buyer who asked about a listing
    And the browser records AI beacons
    When the seller opens the thread and clicks "Gợi ý AI"
    And the seller types in the composer without inserting a suggestion
    Then 3 copilot suggestions are shown
    And exactly 1 "chat_copilot" "shown" AI events are captured
    And zero "chat_copilot" "accepted" AI events are captured

  Scenario: Inserting a suggestion reports accepted
    Given a seller has a chat thread with a buyer who asked about a listing
    And the browser records AI beacons
    When the seller opens the thread and clicks "Gợi ý AI"
    And the seller clicks the first copilot suggestion
    Then the composer holds the first copilot suggestion
    And exactly 1 "chat_copilot" "accepted" AI events are captured
    And zero "chat_copilot" "edited" AI events are captured

  Scenario: Inserting a second suggestion reports a second accepted and re-arms edited
    Given a seller has a chat thread with a buyer who asked about a listing
    And the browser records AI beacons
    When the seller opens the thread and clicks "Gợi ý AI"
    And the seller clicks the first copilot suggestion
    And the seller types 3 more characters in the composer
    And the seller clicks the second copilot suggestion
    And the seller types 3 more characters in the composer
    Then exactly 2 "chat_copilot" "accepted" AI events are captured
    And exactly 2 "chat_copilot" "edited" AI events are captured

  Scenario: Changing the inserted suggestion reports edited once
    Given a seller has a chat thread with a buyer who asked about a listing
    And the browser records AI beacons
    When the seller opens the thread and clicks "Gợi ý AI"
    And the seller clicks the first copilot suggestion
    And the seller types 3 more characters in the composer
    Then exactly 1 "chat_copilot" "edited" AI events are captured
    # sending a suggestion as-is queues no further edited
    When the seller clicks the second copilot suggestion
    And the seller sends the composer text
    Then exactly 2 "chat_copilot" "accepted" AI events are captured
    And exactly 1 "chat_copilot" "edited" AI events are captured

  Scenario: Not the owner of the thread, no copilot and no copilot events
    Given a seller has a chat thread with a buyer who asked about a listing
    And the browser records AI beacons
    When the buyer of that thread opens it
    Then the "Gợi ý AI" button is absent
    And zero "chat_copilot" AI events are captured
    # control: the same browser session does report an AI event when a surface does fire
    When the buyer asks the assistant for a product and a product card is shown
    Then exactly 1 "assistant" "shown" AI events are captured

  # ── Magic Listing ─────────────────────────────────────────────────────

  @needsSeller
  Scenario: A generated draft reports shown once
    Given a seeded seller is logged in
    And the browser records AI beacons
    When the seller opens the new listing form for Magic Listing
    And the seller generates a Magic Listing draft
    And the seller types in the stock field
    Then exactly 1 "magic_listing" "shown" AI events are captured
    And zero "magic_listing" "accepted" AI events are captured

  @needsSeller
  Scenario: Applying reports accepted
    Given a seeded seller is logged in
    And the browser records AI beacons
    When the seller opens the new listing form for Magic Listing
    And the seller generates a Magic Listing draft
    And the seller applies all Magic Listing suggestions
    Then the description field is filled
    And exactly 1 "magic_listing" "accepted" AI events are captured
    And zero "magic_listing" "edited" AI events are captured

  @needsSeller
  Scenario: Changing an applied field reports edited once
    Given a seeded seller is logged in
    And the browser records AI beacons
    When the seller opens the new listing form for Magic Listing
    And the seller generates a Magic Listing draft
    # editing before any apply queues nothing
    And the seller edits the title
    Then zero "magic_listing" "edited" AI events are captured
    When the seller applies all Magic Listing suggestions
    And the seller edits the title
    And the seller edits the description
    Then exactly 1 "magic_listing" "edited" AI events are captured
    And exactly 1 "magic_listing" "accepted" AI events are captured

  @needsSeller
  Scenario: A new draft starts a new cycle
    Given a seeded seller is logged in
    And the browser records AI beacons
    When the seller opens the new listing form for Magic Listing
    And the seller generates a Magic Listing draft
    And the seller applies all Magic Listing suggestions
    And the seller edits the description
    And the seller generates a Magic Listing draft
    And the seller applies all Magic Listing suggestions
    And the seller edits the description
    Then exactly 2 "magic_listing" "shown" AI events are captured
    And exactly 2 "magic_listing" "accepted" AI events are captured
    And exactly 2 "magic_listing" "edited" AI events are captured

  Scenario: A guest cannot reach Magic Listing and emits nothing
    Given the browser records AI beacons
    When a guest requests the new listing page
    Then the guest is redirected to the login page and no Magic Listing card is shown
    And zero "magic_listing" AI events are captured
    # control: the same guest browser session does report an AI event when a surface does fire
    When the guest asks the assistant for a product and a product card is shown
    Then exactly 1 "assistant" "shown" AI events are captured

  # ── PDP review summary ────────────────────────────────────────────────

  Scenario: The summary card reports shown once
    Given a listing with verified reviews exists for AI events
    And the browser records AI beacons
    When a guest opens that listing page and the AI review summary card is shown
    And the page is resized and left idle
    Then exactly 1 "review_summary" "shown" AI events are captured
    And every captured "review_summary" "shown" AI event carries the listing id of that listing

  Scenario: No summary, no event
    Given a listing without reviews exists for AI events
    And a listing with verified reviews exists for AI events
    And the browser records AI beacons
    When a guest opens the listing without reviews and its reviews section has loaded
    Then no AI review summary card is shown
    And zero "review_summary" AI events are captured
    # control: the same session does report the event for a listing that has a summary
    When a guest opens that listing page and the AI review summary card is shown
    Then exactly 1 "review_summary" "shown" AI events are captured

  # ── Collector acceptance and privacy ──────────────────────────────────

  @needsBuyer
  Scenario: AI events are accepted by the collector and carry no free text
    Given a buyer is logged in
    And the browser records AI beacons
    When the buyer asks the assistant "SECRET-PROMPT-7" and a product card is shown
    Then at least 1 "assistant" "shown" AI events are captured
    And every /api/track batch holding an ai_interaction beacon was answered 2xx by the gateway
    And no captured beacon body contains "SECRET-PROMPT-7" or the assistant reply text
    And every captured AI event has exactly the keys surface and action in its properties
