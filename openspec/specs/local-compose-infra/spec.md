# local-compose-infra Specification

## Purpose
The local compose infra stack (`platform-core/infra/docker-compose.yaml` and the root compose files) reports real health and pulls only pinned third-party images, so dependents can wait on `service_healthy`.

## Requirements

### Requirement: Infra healthchecks run in the image's shell

A compose healthcheck SHALL NOT use `CMD-SHELL` with a `/dev/tcp` probe, because `CMD-SHELL` runs `/bin/sh`, which on
the images used here has no `/dev/tcp`; such a probe uses `CMD` with `bash -c`. A service that waits on qdrant in the
root compose SHALL use `condition: service_healthy`.

#### Scenario: No healthcheck probes /dev/tcp through sh

- **WHEN** the healthchecks of the compose files are read
- **THEN** none uses `CMD-SHELL` with `/dev/tcp`, and the qdrant healthcheck is `CMD` with `bash`

#### Scenario: The qdrant dependency waits for health

- **WHEN** the depends_on of the root compose services is read
- **THEN** every map-form dependency on qdrant has `condition: service_healthy`

### Requirement: Compose third-party images are pinned

Every image in `platform-core/infra/docker-compose.yaml` SHALL carry an explicit tag other than `latest`, and none SHALL
be `minio/mc`, which is no longer published. Exceptions SHALL be listed by name in the test.

#### Scenario: No compose infra image floats on latest

- **WHEN** the images of platform-core/infra/docker-compose.yaml are read
- **THEN** each has a tag that is not `latest` (known exceptions: none), and `minio-init` runs `bitnamilegacy/minio-client`
