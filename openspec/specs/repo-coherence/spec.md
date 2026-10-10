# repo-coherence Specification

## Purpose
Defines the workspace consistency checks `scripts/repo_doctor.py` enforces across the `team-*` and `platform-*`
directories, so documentation, configuration and code cannot silently drift apart.

## Requirements

### Requirement: Documented env vars are used by the code

`scripts/repo_doctor.py` SHALL report, for each `team-*` and `platform-*` directory, every env var documented in that
directory's `README.md` env table but referenced by none of its source files, naming the directory and the variable,
and SHALL exit non-zero when it finds one. Generated, vendored and dependency directories SHALL be ignored.

#### Scenario: An undocumented-in-code env var fails the doctor

- **WHEN** `repo_doctor` runs on a copy of the workspace whose `team-referral/README.md` env table gains the row
  `E2E_GHOST_SETTING`
- **THEN** it exits non-zero and names `team-referral` and `E2E_GHOST_SETTING`

#### Scenario: The real workspace passes the env check

- **WHEN** `repo_doctor` runs on the workspace
- **THEN** it reports no undocumented-in-code env var
