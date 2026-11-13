## Decisions

- Placement field: `experiment: {flag: str}` on `Placement` (extra keys still forbidden). Unknown flag name is not validated
  at load (Flipt is a runtime dependency); a missing flag evaluates as no match.
- Client: `adapters/flipt.py` implementing a port `ExperimentAssigner.assign(flag, entity) -> Assignment`, where
  `Assignment(variant: str | None, ranking_model: str | None, status: assigned|none|unassigned)`.
  Request body `{"namespaceKey":"default","flagKey":<flag>,"entityId":<hash>,"context":{}}`.
  Response mapping: `match=true` -> `assigned` with `variantKey`; `match=false` (flag disabled, no segment match) or HTTP 404
  -> `none`; timeout, connection error, 5xx or unparsable body -> `unassigned`.
- Entity: `sha256(salt + ":" + (user_id or anonymous_id)).hexdigest()`. Neither id present -> `none`, no call.
- Overlap: evaluation is launched as an `asyncio` task at the start of `recommend` and awaited with the timeout just before
  `_rank`; ladder retrieval proceeds meanwhile, which keeps the p95 add under 20 ms.
- Ranker override: the attachment `{"ranking": {"model": "cosine_rank"}}` is parsed into `RankingModel`; the service uses
  `placement.model_copy(update={"ranking": ...})` for the request only. `use_featurestore` is not overridable.
- Tier tracking: `_serve` returns the strategy that yielded items; `_rank_all` returns which ranker ran
  (`gbdt` when `trained is not None and features`, else `fixed_weights` for a gbdt placement, `similarity` otherwise).
  `RecommendationResult` gains `tier`, `ranker`, `arm` and a `wire_model_version` property that servicer and log use;
  the existing `model_version` field keeps the bare generation.
- Floor: `_popular_result` reports tier `popular` and ranker `similarity` (it uses `rank_by_similarity`); the unexpected-failure
  path reports tier `serving-fallback`. When the generation cannot be read the generation segment is `serving-fallback`.
- Existing tests and e2e that compare `model_version` to the bare generation or to `serving-fallback` are updated to prefix
  match; nothing else reads the string (analytics groups by it opaquely).
- Flags are created by an idempotent script, not at startup, so a running service never mutates Flipt.
