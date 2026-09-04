VENV ?= $(shell which .venv/bin/pytest >/dev/null 2>&1 && echo ".venv/bin/" || echo "")

.PHONY: install browsers check lint format collect smoke buyer seller auth wap test report clean

install:            ## Install deps + Playwright Chromium
	pip install -e ".[dev]"
	playwright install chromium

browsers:
	playwright install chromium

lint:
	$(VENV)ruff check .

format:
	$(VENV)black .

format-check:
	$(VENV)black --check .

check: lint format-check      ## Pre-PR gate (lint + format)

collect:            ## Dry-run: verify feature<->step wiring without a browser
	$(VENV)pytest --collect-only -q

smoke:
	$(VENV)pytest -m smoke --junitxml=test-results/junit.xml --html=test-results/report.html --self-contained-html

buyer:
	$(VENV)pytest -m buyer

seller:
	$(VENV)pytest -m seller

auth:
	$(VENV)pytest -m auth

wap:                ## Mobile emulation (override DEVICE as needed)
	DEVICE='iPhone 12' $(VENV)pytest -m smoke

test:               ## Full suite with report
	$(VENV)pytest --junitxml=test-results/junit.xml --html=test-results/report.html --self-contained-html

report:
	python scripts/generate_report.py

features:           ## Aggregate + validate repo FEATURES.yaml, print coverage
	$(VENV)python scripts/features.py

features-check:     ## CI gate: fail on any manifest/coverage error
	$(VENV)python scripts/features.py --strict

spec-check:         ## Spec↔e2e gate for an OpenSpec change: make spec-check CHANGE=<id>
	$(VENV)python scripts/features.py --strict
	$(VENV)python scripts/spec_sync.py $(CHANGE) --strict

docker-build:       ## Build isolated Docker image for E2E tests
	docker compose -f docker-compose.e2e.yaml build

docker-test:        ## Run all E2E tests inside isolated Docker container
	docker compose -f docker-compose.e2e.yaml run --rm e2e

clean:
	rm -rf test-results .pytest_cache .ruff_cache
