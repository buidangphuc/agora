## 1. Claim

- [x] 1.1 Failing tests: a token signed for a restricted user has `status=restricted`; an active user's token has no `status` key; login and refresh of a restricted user return such a token
- [x] 1.2 `SignWithSession` takes the status and writes the claim only for `restricted`; login, register and refresh pass the user's status; verify the tests pass

## 2. Docs

- [x] 2.1 Claim lists in ADR-0003 and `team-identity/README.md` (`SECURITY_AND_RELIABILITY.md` lists the gateway principal, which is unchanged)

## 3. Verification

- [x] 3.1 `go test ./...`, vet and lint in team-identity
- [x] 3.2 `openspec validate identity-restricted-status-claim --strict`
