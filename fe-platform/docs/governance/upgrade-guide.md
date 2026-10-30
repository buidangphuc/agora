# Upgrading a consumer app

Consumer apps live outside the workspace and install only from the registry (`examples/consumer` is the reference).

1. Read the CHANGELOG of each package between your version and the target. Every major lists a `## Migration` section.
2. `npm install @fe-platform/tokens@<target> @fe-platform/ui@<target>` (and `api-client` if used). Keep the three on matching majors.
3. Run the consumer's `npm run check`. Its class check fails when a Tailwind class you use no longer exists; this is how a token rename is caught.
4. Apply the Migration steps (usually a rename), rerun `npm run check` and the build.
5. Keep `transpilePackages` listing the installed `@fe-platform/*` packages: they ship TypeScript source.
