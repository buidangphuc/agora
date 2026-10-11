Feature: repo_doctor flags documented env vars that no source references
  scripts/repo_doctor.py reads each team-* and platform-* README env table and reports every
  variable none of that directory's source files references. Static: no stack needed.
  (port-edge-authz-residuals / repo-coherence)

  Scenario: An undocumented-in-code env var fails the doctor
    Given a copy of the workspace whose team-referral README env table gains the row E2E_GHOST_SETTING
    When repo_doctor runs on the copy
    Then it exits non-zero and names team-referral and E2E_GHOST_SETTING

  Scenario: The real workspace passes the env check
    When repo_doctor runs on the workspace
    Then it reports no undocumented-in-code env var
