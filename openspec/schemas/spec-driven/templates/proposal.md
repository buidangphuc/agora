<!-- initiative: <id>   (only if this change is a slice of an initiative; else delete) -->

## Why

<!-- Explain the motivation for this change. What problem does this solve? Why now? -->

## What Changes

<!-- Describe what will change. Be specific about new capabilities, modifications, or removals. -->

## Capabilities

### New Capabilities
<!-- Capabilities being introduced. Use kebab-case for path segments you introduce
     (e.g., user-auth or identity/user-auth) that follow the project's existing
     spec organization. Each creates specs/<capability-path>/spec.md. -->
- `<capability-path>`: <brief description of what this capability covers>

### Modified Capabilities
<!-- Existing capabilities whose REQUIREMENTS are changing (not just implementation).
     Only list here if spec-level behavior changes. Each needs a delta spec file.
     Use the exact existing path under openspec/specs/. Leave empty if no requirement
     changes. A change with no capabilities at all (pure refactor, tooling, docs)
     must set `skip_specs: true` in its .openspec.yaml - openspec validate rejects
     a zero-delta change without that marker. Do not invent a requirement just to
     satisfy validation. -->
- `<existing-capability-path>`: <what requirement is changing>

## Impact

<!-- Affected code, APIs, dependencies, systems -->

## Non-functional requirements

<!-- Measurable targets: latency p95, availability, throughput, a11y, cost. "none" if N/A. -->

## Data classification

<!-- One value per data element: public | internal | personal | sensitive-personal
     (PDPA VN Decree 13/2023: personal vs sensitive personal data; OWASP ASVS 4.0.3 V8).
     For non-public data: retention, readers, controls (no logs/URLs, encryption, purge). -->

| Data element | Class | Retention | Controls |
|---|---|---|---|
| <!-- e.g. buyer phone --> | <!-- personal --> | <!-- 24m --> | <!-- masked in logs, TLS --> |

## Threat model

<!-- Short STRIDE table; only if the change touches auth, money, personal data or a new
     external surface. Otherwise: "none - no new trust boundary". -->

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| <!-- asset --> | <!-- e.g. Tampering --> | <!-- mitigation --> |
