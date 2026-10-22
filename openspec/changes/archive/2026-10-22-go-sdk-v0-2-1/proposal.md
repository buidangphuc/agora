## Why

The team-order pilot on go-sdk v0.2.0 found four gaps:

1. `outbox.Store.EnqueueTx` is a plain INSERT. team-order uses deterministic event ids and relied on `ON CONFLICT (event_id) DO NOTHING`, so a duplicate enqueue now aborts the business transaction.
2. `upstream.Dial` can only replace the principal for the whole connection. team-order mixes a forwarded buyer principal with a service principal on specific methods, so it needed a wrapper.
3. `flags.Client` cannot be built over a custom OpenFeature provider, which makes flag on/off untestable.
4. The consume loop's fetch-error handling has no test.

initiative: go-platform-sdk

## What Changes

- `outbox.EnqueueTx` becomes idempotent on `event_id`: a duplicate is a no-op.
- `upstream.WithServicePrincipal(ctx, id, scopes...)` sets a per-call service principal, which overrides forwarding for that call. Connection-level `AsService` stays.
- `flags.NewWithProvider(provider, timeout)` builds a client over any OpenFeature provider.
- A test is added for consume fetch errors.
- Released as tag `go-sdk/v0.2.1`, local.

## Non-goals

Service changes. Each service picks up v0.2.1 in its own migration.

## Impact

`platform-core/packages/go-sdk` only. Additive, except that `EnqueueTx` on a duplicate id now succeeds where it used to fail.

## Non-functional requirements

none

## Data classification

internal; unchanged.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Upstream identity | Spoofing via the per-call principal | Set only from service code; never read from inbound metadata |
