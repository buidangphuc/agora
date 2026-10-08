# Fault-injecting OpenAI-compatible fake provider (change ai-path-resilience, design D8).
# Build context: platform-e2e/fakes/llm_fake
FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
WORKDIR /app
COPY server.py .
EXPOSE 8099
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
  CMD python -c "import urllib.request as u; u.urlopen('http://127.0.0.1:8099/healthz', timeout=2)"
USER nobody
ENTRYPOINT ["python", "server.py"]
CMD ["--host", "0.0.0.0", "--port", "8099"]
