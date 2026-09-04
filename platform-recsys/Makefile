# platform-recsys — offline PySpark ALS batch job.
# Spark/PySpark run in Docker/CI; the host targets (lint/compile/host-tests)
# need only python3. `run-local` needs PySpark + local Qdrant/Redis.

PY ?= python3
IMAGE ?= localhost:5001/platform-recsys:local
SAMPLE ?= /data/tracking_events.parquet

.PHONY: help
help:
	@echo "compile      - byte-compile all sources (syntax gate, no PySpark needed)"
	@echo "lint         - ruff + black --check"
	@echo "fmt          - black + ruff --fix"
	@echo "test         - full pytest (Spark tests auto-skip without PySpark)"
	@echo "test-host    - only the PySpark-free tests (config, weights, recommend, env drift)"
	@echo "check-env    - .env.example drift gate"
	@echo "sample       - generate a tiny Parquet warehouse at SAMPLE"
	@echo "run-local    - run the batch in local[*] (needs PySpark + Qdrant/Redis)"
	@echo "docker-build - build the Spark job image"
	@echo "docker-run   - run the job container on platform-core_default"

.PHONY: compile
compile:
	$(PY) -m py_compile $$(find recsys sample_data tests -name '*.py')

.PHONY: lint
lint:
	ruff check recsys tests sample_data
	black --check recsys tests sample_data

.PHONY: fmt
fmt:
	black recsys tests sample_data
	ruff check --fix recsys tests sample_data

.PHONY: test
test:
	$(PY) -m pytest -q

.PHONY: test-host
test-host:
	$(PY) -m pytest -q tests/test_config.py tests/test_weights.py tests/test_recommend.py tests/test_env_drift.py

.PHONY: check-env
check-env:
	$(PY) -m pytest -q tests/test_env_drift.py

.PHONY: sample
sample:
	$(PY) sample_data/generate_sample.py $(SAMPLE)

.PHONY: run-local
run-local:
	WAREHOUSE_DRIVER=duckdb WAREHOUSE_PARQUET_PATH=$(SAMPLE) $(PY) -m recsys

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE) .

.PHONY: docker-run
docker-run:
	docker run --rm --network platform-core_default \
		-e WAREHOUSE_DRIVER=duckdb \
		-e WAREHOUSE_PARQUET_PATH=/data/tracking_events.parquet \
		-e QDRANT_URL=http://qdrant:6333 \
		-e REDIS_HOST=redis -e REDIS_PORT=6379 \
		-v $(PWD)/data:/data \
		$(IMAGE)
