## Clarifications

- 2026-11-12. Question: must the gateway answer HTTP 404 for the `GetListingStock` path? Answer: accept 404 OR 501 unimplemented. The requirement is that the gateway does not forward the call to team-search. team-gateway mounts the generated Connect handler with an embedded Unimplemented server, so the exact path answers 501. The trailing-slash path may answer 404.
- 2026-11-12 (Bùi Đăng Phúc): traceability-only edits so spec_sync --strict passes — added VERIFIED BY lines to "A repeated id yields a repeated entry" (TestGetListingStockRepeatedID) and "A document without projected stock has no stock" (TestGetListingStockNoProjectedStock, TestOpenSearchGetListingStockBatch); no scenario text changed.
