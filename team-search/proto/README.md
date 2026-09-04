# proto/ — vendored platform contract

Pinned copy of platform-core's proto module (package `platform.*`), input to
`buf generate` → `../generated/` (gitignored). Per ADR-0001, team-search vendors
the proto sources and generates its own Go code; platform-core never writes here.

This service (team-search) OWNS `platform.search.v1.SearchService`. It also
vendors `common`, `events`, and `listing` so it can consume `listing.events`
(unmarshal `platform.listing.v1.ListingChanged` out of the envelope).

Regenerate: `make proto` then `go mod tidy`.
