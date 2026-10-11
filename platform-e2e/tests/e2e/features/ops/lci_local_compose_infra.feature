Feature: Local compose infra probes and images
  Static, no docker: the compose files are parsed as YAML. (local-compose-infra)

  Scenario: No healthcheck probes /dev/tcp through sh
    When the healthchecks of the compose files are read
    Then none uses CMD-SHELL with /dev/tcp, and the qdrant healthcheck is CMD with bash

  Scenario: The qdrant dependency waits for health
    When the depends_on of the root compose services are read
    Then every map-form dependency on qdrant has condition service_healthy

  Scenario: No compose infra image floats on latest
    When the images of platform-core/infra/docker-compose.yaml are read
    Then each has a tag that is not latest except the known exceptions, and minio-init runs bitnamilegacy/minio-client
