# Multi-stage build for the JVM/Kotlin gRPC service.
#
# NOTE: the build stage assumes generated proto code already exists under
# ./generated. Run `make proto` before `docker build`, or add a buf-generate
# step to this stage (needs the proto-vendor submodule checked out in context).

# ── Stage 1: build a runnable distribution with Gradle ────────────────────────
FROM gradle:8.10-jdk21 AS build
WORKDIR /home/gradle/src

# Cache dependencies first (build files change less often than source).
COPY --chown=gradle:gradle settings.gradle.kts build.gradle.kts ./
RUN gradle --no-daemon dependencies || true

# Copy the rest (source, generated/, resources) and assemble the app image.
COPY --chown=gradle:gradle . .
RUN gradle --no-daemon installDist

# ── Stage 2: slim JRE runtime, non-root ───────────────────────────────────────
FROM eclipse-temurin:21-jre AS runtime

# Run as an unprivileged user.
RUN groupadd --system app && useradd --system --gid app --home /app app
WORKDIR /app

COPY --from=build --chown=app:app /home/gradle/src/build/install/team-service ./

ENV GRPC_PORT=50053
EXPOSE 50053

USER app
ENTRYPOINT ["./bin/team-service"]
