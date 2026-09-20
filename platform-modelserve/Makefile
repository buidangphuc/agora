.PHONY: install test lint run clean

PYTHON ?= python3

install:
	$(PYTHON) -m pip install -r requirements-dev.txt
	$(PYTHON) -m pip install -e .

test:
	pytest -v tests/

lint:
	ruff check .

format:
	ruff format .

run:
	$(PYTHON) -m modelserve

clean:
	rm -rf build dist *.egg-info .pytest_cache .ruff_cache
