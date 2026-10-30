# Local package registry (dev only)

Verdaccio on `127.0.0.1:4873`, anonymous read and publish for `@fe-platform/*`, everything else proxied to npmjs for installs. Bound to loopback only; a shared registry must require authenticated publish (ADR-0004).

```bash
docker compose -f infra/registry/docker-compose.yaml up -d
npm ping --registry http://127.0.0.1:4873
```

Publish (owner): `npm run release:version`, review and commit, then `npm run release:publish`. The publish script supplies a dummy token for the loopback registry, because npm refuses to publish without one; it does not create git tags.

Stop: `docker compose -f infra/registry/docker-compose.yaml down` (add `-v` to wipe stored versions).

Consumers map the scope in their own `.npmrc`: `@fe-platform:registry=http://127.0.0.1:4873`.
