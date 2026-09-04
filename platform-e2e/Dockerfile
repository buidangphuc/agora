FROM mcr.microsoft.com/playwright/python:v1.48.0-noble

WORKDIR /workspace

# Install dependencies first for layer caching
COPY pyproject.toml .
RUN pip install --no-cache-dir -e ".[dev]"

# Install Chromium browser binary and system dependencies
RUN playwright install chromium --with-deps

# Copy platform-e2e source
COPY . .

ENV BASE_URL=http://host.docker.internal:3000
ENV GATEWAY_URL=http://team-gateway-svc:8080
ENV HEADLESS=true
ENV PYTHONUNBUFFERED=1

CMD ["pytest", "-q", "--tb=short"]
