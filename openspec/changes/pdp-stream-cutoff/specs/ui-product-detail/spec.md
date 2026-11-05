## ADDED Requirements

### Requirement: The page streams to the end

`/listing/[id]` SHALL stream its whole HTML document, closing `</html>` and including the "Đánh giá" (`#reviews`)
and "Hỏi đáp" (`#qa`) sections, whatever the size of the listing's content. The React server renderer vendored by
Next SHALL carry the upstream fix facebook/react#34694, which detaches a Suspense boundary from its segment once the
boundary is written, so a boundary whose fallback segment is still suspended is never flushed a second time.

#### Scenario: The page streams to the end whatever the description length

- **WHEN** a visitor requests the product pages of one seller's published listings whose descriptions are 1 to 40
  characters long
- **THEN** every response completes with the whole HTML document, including the reviews and Q&A sections, and
  not one is cut off before `</html>` (no "failed to pipe response" error)
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature` `The page streams to the end whatever the description length`
