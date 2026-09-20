## Context

The batch job currently writes serving artifacts and then stops. Introducing a gate that can
*reject* a run means the existing write order becomes unsafe: by the time a verdict exists, the
rejected generation has already replaced the previous one.

Three properties of the current loader make this concrete:

- `qdrant.py:_prune_stale` deletes every point **not** stamped with the current run's
  `model_version` — the previous generation is gone, not shadowed.
- `redis_cache.load_cache` writes `recs:v1:model_version` last, deliberately, so readers only
  see a fully-written generation. That ordering is good and should be preserved.
- Points are keyed `uuid5(namespace, source_id)`, stable across runs — so an upsert overwrites
  the previous generation's vector for the same listing in place.

## Decisions

**1. Evaluate before publishing, not after.**
The holdout is scored from the trained factors in memory, before any Qdrant upsert. A rejected
candidate never touches the serving stores. This is cheaper than the alternatives (no shadow
collection, no rollback path) and keeps the failure mode "nothing changed" rather than "changed
then reverted".

**2. Prune only after promotion.**
Upsert is idempotent per `(listing_id, model_version)`, but prune is destructive. Prune moves
behind the gate so it only ever removes a generation that a promoted run has superseded.

**3. `recs:v1:model_version` stays the last write.**
Unchanged from today, and now doubly meaningful: it flips only for a promoted generation, so the
key names the champion rather than the most recent run.

**4. First run with no champion is promoted unconditionally.**
Already the behaviour of `evaluate_and_promote` (`registry.py:74-77`). Keep it — a cold registry
must be able to bootstrap, and there is nothing to regress against.

**5. Thresholds are configuration, and the shipped default is not 0.0.**
`min_relative_improvement=0.0` is the right library default but the wrong operational one: it
promotes noise. The setting is added to `_FIELDS` so the deployed value is explicit and visible
in `cronjob.yaml`.

## Risks / Trade-offs

- **A rejected run leaves stale artifacts serving.** Mitigated by the existing 48h Redis TTL
  being longer than the nightly cadence — the floor degrades rather than empties — but repeated
  rejection needs to be visible, hence the summary reporting the decision.
- **Holdout cost.** Scoring adds a pass over the interaction window. Acceptable for a nightly
  batch; if it becomes material the window is already a setting.
- **The first real number may be unflattering.** ALS on sparse data may score poorly. That is
  the point: an unflattering number is information, a missing number is not.
