---
"@fe-platform/ui": minor
"@fe-platform/tokens": minor
---

`Table` is generic over its row type and gains `render` cells, `align`, `sticky` (first column pinned below `md`) and `hideBelow` columns, `rowKey`, `rowTestId`, `captionHidden`, a loading state (`loading`, `loadingRows`, `aria-busy`) and an error state (`error`, `onRetry`, `retryLabel`). It now sits in a horizontal scroll container. Tokens add the matching `table` class maps.
