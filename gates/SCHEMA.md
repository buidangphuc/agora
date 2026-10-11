# Gate question sets — format

A gate is a versioned set of closed questions answered per state unit (scenario, task, change).
File name: `gates/<gate>.v<N>.yaml`. A question set is reviewed like code; changing wording or a
threshold means a new `version` (old files stay so logged runs can be replayed).

```yaml
gate: feature                 # spec | task | feature | release
version: 1
pins:                         # standard editions the `source:` IDs refer to
  asvs: "4.0.3"
state_unit: scenario          # scenario | task | change — how decide.py slices the state
max_state_tokens: 8000        # decide.py refuses (ESCALATE_LLM) a unit above this
questions:
  - id: checks_store          # snake_case, unique in the file
    source: "ISO/IEC/IEEE 29119-4:2021 §5.2.1"   # standard + clause; required
    type: noul                # noul (yes/no) | choice | score
    severity: veto            # veto | normal
    instructions: "Does the test verify ...?"    # phrased so the answer is in the state
    # --- noul ---
    pass_when: true           # the answer that counts as pass
    threshold: 0.85           # p = P(answer == pass_when); p >= t pass, p <= 1-t fail, else uncertain
    # --- choice ---
    criteria:                 # ordered; every option has a description
      real: "Asserts persisted state ..."
      mock: "Asserts that a mock was called"
    pass_values: [real]
    min_confidence: 0.6       # top-choice confidence below this -> uncertain
    # --- score ---
    levels:                   # ordered lowest -> highest; every level has a description
      none: "..."
      full: "..."
    pass_min: full            # top level must be >= this level
    min_confidence: 0.6
```

## How decide.py turns answers into a decision

| Case | Result |
|---|---|
| any `veto` question fails | `BLOCK` (advisory mode: `WOULD_BLOCK`) |
| any question uncertain | `ESCALATE_LLM` for exactly those questions |
| a `normal` question fails | `FAIL` (reported, does not block) |
| all pass | `PASS` |
| script routing flags auth / payment / PII / prod release | `NEEDS_HUMAN` on top of the above |
| engine unreachable | `ENGINE_DOWN` → fallback to the LLM reviewer |

No averaging across questions. Precedence: `ENGINE_DOWN` > `BLOCK` > `ESCALATE_LLM` > `FAIL` > `PASS`;
`NEEDS_HUMAN` is a separate flag, never cleared by a pass.

## Engine wire format

decide.py converts a question set to the Kev/TypeSafe `questions` object and calls
`~/tools/kev/kev.py ask --questions @q.json --part NAME=@file ...`:

- noul → `{"type": "noul", "instructions": ...}` → answer `{"yes": p}` (p = P(yes))
- choice → `{"type": "choice", "instructions": ..., "criteria": {opt: desc}}` → `{"choice", "confidence", "probabilities"}`
- score → `{"type": "score", "instructions": ..., "criteria": [level desc, ...]}` → `{"score", "level", "confidence", "probabilities"}`

Machine-checkable items (routing by path glob, write-set, secrets, `buf breaking`, test file exists)
are scripts, never Kev questions.
