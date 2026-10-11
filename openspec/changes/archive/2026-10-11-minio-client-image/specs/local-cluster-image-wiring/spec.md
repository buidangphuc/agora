## ADDED Requirements

### Requirement: Third-party images are pinned

Every container and init container image in `platform-gitops` that is not built by the stack (not `localhost:5001`)
SHALL carry an explicit tag other than `latest`, so a withdrawn or moved upstream tag cannot turn a cached image into
an ImagePullBackOff and hold an ArgoCD sync hook.

#### Scenario: No third-party image floats on latest

- **WHEN** every manifest under platform-gitops/platform is read
- **THEN** each container and init container image outside localhost:5001 has a tag, the tag is not `latest`, and no image is `minio/mc`
