// TEMPLATE — JVM/Kotlin gRPC service seed (Hexagonal Architecture).
//
// This is a SEED, not a buildable service. The `generated/` source set below
// only exists AFTER you run `make proto` (buf generate against the vendored
// proto module — see proto-vendor/README.md). Until then the gRPC adapter that
// imports `com.platform.listing.v1.*` won't compile — that's expected.
//
// Rename `team-service` (settings.gradle.kts) before first commit.

import org.jetbrains.kotlin.gradle.tasks.KotlinCompile

plugins {
    kotlin("jvm") version "2.0.21"
    application
    // Lint/format — pick ONE as your merge gate (`make check`). ktlint is the
    // default; detekt is left commented as the alternative.
    id("org.jlleitschuh.gradle.ktlint") version "12.1.1"
    // id("io.gitlab.arturbosch.detekt") version "1.23.7"
}

group = "com.your-org.teamservice"
version = "0.1.0"

repositories {
    mavenCentral()
}

// ─── Pinned dependency versions ───────────────────────────────────────────────
val grpcVersion = "1.68.1"
val grpcKotlinVersion = "1.4.1"
val protobufVersion = "4.28.3"
val otelVersion = "1.43.0"
val otelInstrumentationVersion = "2.9.0"
val flywayVersion = "10.20.1"
val junitVersion = "5.11.3"

dependencies {
    // ── gRPC transport + runtime ──────────────────────────────────────────────
    implementation("io.grpc:grpc-kotlin-stub:$grpcKotlinVersion")
    implementation("io.grpc:grpc-protobuf:$grpcVersion")
    implementation("io.grpc:grpc-netty-shaded:$grpcVersion")
    // Health + reflection services (registered in Server.kt).
    implementation("io.grpc:grpc-services:$grpcVersion")
    implementation("com.google.protobuf:protobuf-kotlin:$protobufVersion")

    // Kotlin coroutines — grpc-kotlin generates suspend/Flow signatures.
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.9.0")

    // ── Observability (OpenTelemetry, exporter-swappable — ADR-0004) ──────────
    implementation("io.opentelemetry:opentelemetry-api:$otelVersion")
    implementation("io.opentelemetry:opentelemetry-sdk:$otelVersion")
    implementation("io.opentelemetry:opentelemetry-exporter-otlp:$otelVersion")
    // Optional but recommended: auto gRPC client/server spans. If this artifact
    // is unavailable in your mirror, drop it and keep the manual
    // TracingInterceptor — the seed does not depend on it to compile.
    implementation("io.opentelemetry.instrumentation:opentelemetry-grpc-1.6:$otelInstrumentationVersion")

    // ── Persistence (own DB only — DB-per-service) ────────────────────────────
    // Flyway drives schema migrations (src/main/resources/db/migration).
    implementation("org.flywaydb:flyway-core:$flywayVersion")
    implementation("org.flywaydb:flyway-database-postgresql:$flywayVersion")
    // Postgres driver — OPTIONAL until you swap InMemoryListingRepository for a
    // real adapter. Uncomment together with the JDBC/pool wiring.
    // implementation("org.postgresql:postgresql:42.7.4")
    // implementation("com.zaxxer:HikariCP:6.0.0")

    // ── Test ──────────────────────────────────────────────────────────────────
    testImplementation(kotlin("test"))
    testImplementation("org.junit.jupiter:junit-jupiter:$junitVersion")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

// ─── Source sets ──────────────────────────────────────────────────────────────
// Wire the buf-generated code (java + kotlin + grpc-java + grpc-kotlin) in as an
// extra source root. `make proto` writes it; `.gitignore` excludes it; it is
// regenerated on demand and NEVER hand-edited.
sourceSets {
    named("main") {
        java.srcDir("generated")
    }
}

application {
    // main() lives at the source root (Server.kt → top-level `ServerKt`).
    mainClass.set("ServerKt")
}

kotlin {
    jvmToolchain(21)
}

tasks.withType<KotlinCompile>().configureEach {
    compilerOptions {
        freeCompilerArgs.add("-Xjsr305=strict")
    }
}

tasks.test {
    useJUnitPlatform()
}

ktlint {
    // Generated code must never be linted — it's not ours.
    filter {
        exclude { it.file.path.contains("/generated/") }
    }
}
