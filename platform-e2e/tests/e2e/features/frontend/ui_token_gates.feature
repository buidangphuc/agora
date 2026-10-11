@buyer
Feature: UI foundation - token gates and the frontend quality pipeline
  The design tokens, the token lint, the typed action result and the quality gate are repo
  checks of team-frontend (ui-foundation). They are exercised for real - the Tailwind build,
  the lint script, the TypeScript compiler, Vitest and `npm run check` - and the colour
  scenario renders the generated CSS in a real browser.

  Scenario: The brand scale resolves from the config
    When a component uses the "bg-primary-500" class built from the Tailwind config
    Then the computed background colour is "rgb(238, 77, 45)"

  Scenario: Text below 12px is not available
    When the built CSS served by the storefront is searched for type-scale font sizes
    Then every type-scale font size is at least 12px and the scale is present

  Scenario: A new arbitrary value fails the gate
    When a component with className "text-[9px]" is added to a copy of the token lint tree
    Then the token lint exits non-zero and names the file, line and "text-[9px]"

  Scenario: The current tree is clean
    When the token lint runs on the team-frontend tree
    Then the token lint exits zero and reports 0 violations

  Scenario: A failed action is typed as an error
    When a Server Action result is narrowed on a failure
    Then the compiler accepts the error branch and rejects reading data on it

  Scenario: Test script runs the unit suite
    When a developer runs "npm test" in team-frontend with no backend configured
    Then the Vitest suite runs on jsdom and every test file passes

  Scenario: The quality gate is green
    When a developer runs "npm run check" in team-frontend
    Then Biome, tsc, the token lint and Vitest all pass and the command exits 0
