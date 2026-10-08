## Purpose

Records that share-link creation is intentionally available to anonymous visitors and is bounded by the edge rate limit, so the public share button keeps working without opening an unbounded write path.

## ADDED Requirements

### Requirement: Share-link creation stays anonymous and is rate-limited at the edge

`team-sharing` SHALL keep accepting `CreateShareLink` and `ResolveShareLink` from callers without a session (the public
listing page offers sharing to logged-out visitors), and `team-gateway` SHALL continue to apply its per-caller rate limit to
`CreateShareLink` (caller key: the principal id, or the peer address for anonymous callers), answering `resource_exhausted`
once the limit is exceeded. `team-sharing` SHALL NOT trust any caller-supplied identity header for authorization.

#### Scenario: Anonymous visitor creates and resolves a share link

- **WHEN** an anonymous client calls `CreateShareLink` for target `listing` and then `ResolveShareLink` with the returned short code
- **THEN** the short code resolves to that target with OG meta

#### Scenario: A burst of anonymous creations is limited

- **WHEN** one anonymous client sends more `CreateShareLink` calls in a burst than the edge burst allowance
- **THEN** the calls beyond the allowance are answered `resource_exhausted` (HTTP 429)
