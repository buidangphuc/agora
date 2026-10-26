## MODIFIED Requirements

### Requirement: Edge reflection is off unless enabled, and never in staging or production

`team-gateway` SHALL serve gRPC reflection only when `GRPC_REFLECTION_ENABLED=true` (default `false`). It SHALL refuse to start with `GRPC_REFLECTION_ENABLED=true` unless `ENV` is `local` or `test`, so that staging, production and any other value are refused, exiting non-zero with a message naming the setting. A `EDGE_REFLECTION_ENABLED` variable left in an environment SHALL be ignored and SHALL NOT enable reflection.

#### Scenario: Reflection is not served by default

- **WHEN** a client posts a reflection request to `/grpc.reflection.v1.ServerReflection/ServerReflectionInfo` on a
  gateway started with `GRPC_REFLECTION_ENABLED` unset
- **THEN** the gateway answers HTTP 404
- **VERIFIED BY**: team-gateway `internal/edge` `TestReflectionNotServedByDefault`

#### Scenario: The gateway refuses reflection in production

- **WHEN** the team-gateway image is started with `ENV=production` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** the process exits non-zero and its log names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-gateway `internal/config` `TestStrictEnvRefusesReflection`

#### Scenario: Reflection is served locally when enabled

- **WHEN** the gateway starts with `ENV=local` and `GRPC_REFLECTION_ENABLED=true` and a client posts a reflection request
- **THEN** the gateway answers a status other than 404
- **VERIFIED BY**: team-gateway `internal/edge` `TestReflectionServedWhenEnabledLocally`

#### Scenario: A retired reflection key does not enable reflection

- **WHEN** the gateway starts with `ENV=local` and only `EDGE_REFLECTION_ENABLED=true` set
- **THEN** a reflection request is answered HTTP 404
- **VERIFIED BY**: team-gateway `internal/config` `TestRetiredReflectionKeyIsIgnored`
