## MODIFIED Requirements

### Requirement: team-frontend has a unit test runner

`team-frontend` SHALL provide a unit test runner (Vitest + React Testing Library, jsdom environment)
invoked by a `test` script, so isolated tests run in seconds without the full stack. The runner SHALL
be wired so it can run in the existing quality pipeline alongside Biome and `tsc`. The quality gate
`npm run check` SHALL run Biome, `tsc --noEmit`, the token lint and Vitest, and SHALL exit 0 on the main
line of development.

#### Scenario: Test script runs the unit suite

- **WHEN** a developer runs `npm test` (`vitest run`) in `team-frontend`
- **THEN** the Vitest suite executes against jsdom and reports pass/fail, with no dependency on a running
  gateway or backend

#### Scenario: The quality gate is green

- **WHEN** a developer runs `npm run check` on the branch after this change
- **THEN** Biome, `tsc --noEmit`, the token lint and Vitest all pass and the command exits 0
