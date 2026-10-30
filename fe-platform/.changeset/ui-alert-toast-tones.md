---
"@fe-platform/ui": minor
"@fe-platform/tokens": minor
---

New `Alert` (tones info, success, warning, danger; title, description, optional action, closable with `closeLabel`): `danger` is role=alert, the others role=status, and it stays server-safe unless `closable`. `ToastProvider` accepts `label`, `dismissLabel` and `position` ("top" or "bottom"); toast messages accept `tone` (danger toasts are role=alert, the others role=status) and `duration` (0 keeps the toast until dismissed). Tokens add `alert` and toast `viewport`, `tone` and `body` class maps.
