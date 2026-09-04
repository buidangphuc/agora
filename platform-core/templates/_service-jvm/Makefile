# team-service (JVM/Kotlin) — developer entrypoints.
#
# `check` is the local MERGE GATE: run it (green) before opening a PR.

# Load .env if present so `make run` / `make migrate` see local config.
ifneq (,$(wildcard .env))
include .env
export
endif

GRADLE ?= ./gradlew

.PHONY: proto run check test lint migrate build clean help

## proto: generate gRPC/proto code into ./generated from the vendored module.
##        Requires the proto-vendor submodule to be initialised & pinned
##        (see proto-vendor/README.md) and `buf` installed.
proto:
	buf generate

## run: start the gRPC server (reads GRPC_PORT from env, default 50053).
run:
	$(GRADLE) run

## test: run the JUnit5 test suite.
test:
	$(GRADLE) test

## lint: ktlint check (swap for detekt if you chose it in build.gradle.kts).
lint:
	$(GRADLE) ktlintCheck

## check: MERGE GATE — lint + tests must pass before merge.
check: lint test

## build: assemble a runnable distribution (build/install/team-service).
build:
	$(GRADLE) installDist

## migrate: apply Flyway migrations to DATABASE_URL. OPTIONAL — enable once the
##          Postgres deps + Flyway Gradle plugin are wired in build.gradle.kts.
##          Until then this is a documented placeholder.
migrate:
	@echo "migrate: enable Flyway (see build.gradle.kts + V1__init.sql) then run:"
	@echo "  $(GRADLE) flywayMigrate -Dflyway.url=$$DATABASE_URL"
	# $(GRADLE) flywayMigrate

## clean: remove build outputs.
clean:
	$(GRADLE) clean

help:
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'
