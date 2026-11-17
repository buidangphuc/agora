## Context

`StorefrontService` depends only on `store.StorefrontRepository`. Handler `UpsertStorefront`
(`team-domain/internal/rpc/storefront.go`) forces the owner, then delegates. Error rules live in
`team-domain/internal/rpc/errors.go` (`grpcx.Rule`).

## Decisions

- Validation stays in `StorefrontService.Upsert`; the handler only maps errors. New sentinel errors
  (`ErrTaglineInvalid`, `ErrBannerInvalid`, `ErrTooManyFeatured`, `ErrFeaturedInvalid`,
  `ErrThemeInvalid`) map to INVALID_ARGUMENT with a fixed short message, like `ErrDisplayNameInvalid`.
- Featured lookup: one batched read of the listings by id (add a `GetMany`-style method to the
  listing repository port if `Get` per id is the only option; the unit must not issue N queries).
  Foreign and missing collapse into `ErrFeaturedInvalid`.
- Banner check uses the configured `PublicBaseURL` (same value `GetPresignedUploadURL` builds
  `public_url` from), passed to `NewStorefrontService`. If it is empty (public base unset) the
  prefix is `<scheme>://<endpoint>/<bucket>/` as the storage code builds it; implementation reuses
  one helper so the two never drift.
- Legacy rows: the unchanged-banner exemption compares with `GetBySeller`; a first save has no
  stored value.
- Empty theme is accepted and stored as given (reads treat it as `default`); not normalised so
  existing rows and clients are unchanged.
- Validation order is fixed (tagline, banner, featured, theme) so one bad request gives one
  deterministic message.

## Risks

- A seller whose storage public base changes between environments has banners rejected on re-save
  unless unchanged; acceptable locally, revisit with env work.
