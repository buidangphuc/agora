## MODIFIED Requirements

### Requirement: Workspace rule that admin mutations audit first

Root `AGENTS.md` SHALL gain a rule in section 3: an RPC called by a caller holding the `admin` (team-order: `order.admin`; team-promotion: `promotion.admin`) scope, including an admin who owns the resource, SHALL write a team-audit event (naming actor, `<entity>.<verb>` action and target) before applying the change, and SHALL return `Unavailable` with no change when the audit write cannot be made. `.claude/agents/auth-scope-reviewer.md` SHALL list the violation as a BLOCKING item and its CLEAN output SHALL name the admin-path RPCs it checked for an audit write.

#### Scenario: AGENTS.md carries the rule
- **WHEN** a reader opens section 3 of `AGENTS.md`
- **THEN** it holds the rule text naming `admin`/`order.admin`/`promotion.admin`, "including an admin who owns the resource", "team-audit event first", `Unavailable` and "no change", and the existing rules 1-5 are unchanged
- **VERIFIED BY**: grep assertions on `AGENTS.md` (tasks 3.2)

#### Scenario: Reviewer flags an unaudited admin mutation
- **WHEN** the auth-scope-reviewer reads a diff in which an RPC mutates state only because the caller holds `admin` and no audit write precedes the mutation
- **THEN** its definition instructs it to report a BLOCKING finding, and to report a BLOCKING finding as well when the audit failure is swallowed so that the mutation still proceeds
- **VERIFIED BY**: grep assertions on `.claude/agents/auth-scope-reviewer.md` for the check text and the fail-closed wording (tasks 3.3)

#### Scenario: Owner path is not flagged
- **WHEN** the reviewer reads a diff where the same RPC is reachable by a resource owner who lacks the `admin`/`order.admin`/`promotion.admin` scope
- **THEN** its definition states that path needs no audit write and is not a finding, while a caller holding the admin scope is audited even when they own the resource
- **VERIFIED BY**: grep assertion on `.claude/agents/auth-scope-reviewer.md` for the owner-path exemption (tasks 3.3)
