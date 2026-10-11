## ADDED Requirements

### Requirement: Local compose gives team-ai no literal foreign database credentials

The root compose file SHALL NOT hand team-ai the database credentials of another service. team-ai's `POSTGRES_USER`,
`POSTGRES_PASSWORD` and `POSTGRES_DB` SHALL come from `TEAM_AI_POSTGRES_*` variables with placeholder defaults, documented
in the root `.env.example`.

#### Scenario: The team-ai container does not hold listing's credentials

- **WHEN** the running `team-ai-svc` container's environment is inspected
- **THEN** none of its `POSTGRES_*` values is `listing_svc`, `listing_pass` or `listing_db`
