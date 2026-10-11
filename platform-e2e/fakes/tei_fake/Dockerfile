# Deterministic, fault-injecting TEI-compatible fake (changes add-platform-modelserve and
# add-hybrid-retrieval-platform e2e track). Build context: platform-e2e/fakes/tei_fake
FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
WORKDIR /app
COPY server.py .
EXPOSE 8110
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
  CMD python -c "import urllib.request as u; u.urlopen('http://127.0.0.1:8110/healthz', timeout=2)"
USER nobody
ENTRYPOINT ["python", "server.py"]
CMD ["--host", "0.0.0.0", "--port", "8110"]
