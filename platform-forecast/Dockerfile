FROM python:3.12-slim AS base
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends libgomp1 \
    && rm -rf /var/lib/apt/lists/*
COPY pyproject.toml ./
COPY forecast ./forecast
RUN pip install --no-cache-dir .
RUN useradd --system --uid 1001 --no-create-home app

FROM base AS test
RUN pip install --no-cache-dir '.[dev]'
COPY tests ./tests
USER 1001
CMD ["pytest", "-q", "tests/"]

FROM base AS job
USER 1001
ENV INPUT_DIR=/data
ENTRYPOINT ["python", "-m", "forecast"]
CMD ["run"]
