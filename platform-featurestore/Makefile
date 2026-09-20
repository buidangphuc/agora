.PHONY: test lint

PYTHON ?= python3

test:
	pytest -v tests/

lint:
	ruff check .

format:
	ruff format .
