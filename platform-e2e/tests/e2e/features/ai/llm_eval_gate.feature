Feature: The eval gate runs the deterministic cases and skips judge cases without a judge
  `make -C team-ai eval` is run as a black box: its exit code and its printed report are the
  assertions. Change: ai-path-resilience (llm-inference-resilience, requirement 9; the rewritten
  gate grades the completions echo handler, not the chat streamer).

  Scenario: The eval gate scores the deterministic cases
    Given team-ai's eval gate with no judge configured
    When make eval runs in team-ai with no judge configured
    Then it exits zero with a pass rate of 100% and the judge cases skipped
