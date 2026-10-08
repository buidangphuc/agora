## ADDED Requirements

### Requirement: Deployed environments declare their strict ENV so boot guards are armed

The staging and prod overlays SHALL set `ENV` to `staging` and `production` respectively for every Go service that has an
environment-strict boot guard (`team-order`, `team-identity`, `team-payment`, `team-gateway`), so that a mis-set development
flag refuses to boot instead of running in a deployed environment. Local environments SHALL keep the default `local`.

#### Scenario: Staging and prod overlays set ENV

- **WHEN** the rendered staging and prod values of each of those four services are inspected
- **THEN** `ENV` is `staging` and `production` respectively, and the local render is unchanged

#### Scenario: A dev flag in a deployed render is refused at boot

- **WHEN** a staging render of `team-payment` sets `MOCK_PAYMENTS=true`
- **THEN** the pod exits at startup with an error naming `MOCK_PAYMENTS`

### Requirement: team-ai accepts ingress only from the edge and monitoring

The `team-ai` deployment SHALL have a default-deny ingress NetworkPolicy in staging and prod that allows only `team-gateway` and
the in-cluster Prometheus, like the other services (ADR-0010).

#### Scenario: A non-gateway pod cannot reach team-ai

- **WHEN** the rendered staging manifests are inspected
- **THEN** `team-ai` has a NetworkPolicy whose allowed sources are exactly `team-gateway` and `prometheus`
