Feature: The eval gate exercises the real streamer and cannot pass vacuously
  `make -C team-ai eval` is run as a black box: its exit code and its printed report are the
  assertions. The judge scenario passes one extra eval case through EVAL_EXTRA_CASES.
  Change: ai-path-resilience (llm-inference-resilience, requirement 9).

  Scenario: The eval gate runs the chat resilience cases
    Given team-ai's eval gate with no judge configured
    When make eval runs in team-ai with no judge configured and no judge cases
    Then it exits zero and its report lists the primary-429, all-fail and after-first-chunk cases as passed

  Scenario: A judge case without a judge fails the gate
    Given team-ai's eval gate with no judge configured
    And an extra eval set with one judge case
    When make eval runs with that eval set and JUDGE_CHAT_MODEL empty
    Then it exits non-zero, names that case, and reports a pass rate below 100%
