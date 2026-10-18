# proto/ — vendored platform contract

Pinned copy of platform-core's proto module (ADR-0001). Input to `buf generate` →
`../generated/` (gitignored). team-identity OWNS the `platform.identity.v1` services
(`AuthService`, `AddressService`, `SessionService`, `PublicProfileService`, `AccountAdminService`)
and reuses `platform.common.v1.Principal`.
