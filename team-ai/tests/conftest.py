import pytest
from httpx import ASGITransport, AsyncClient

from app.bootstrap.application import create_app
from app.core.config import Settings
from tests.factories import build_test_settings


@pytest.fixture()
def test_settings() -> Settings:
    return build_test_settings(
        AUTH_SUBJECT="test-user",
        AUTH_ROLES="admin,developer",
    )


@pytest.fixture()
async def client(test_settings: Settings):
    app = create_app(settings=test_settings, init_resources=False)
    async with AsyncClient(
        transport=ASGITransport(app=app),
        base_url="http://test",
    ) as test_client:
        yield test_client


@pytest.fixture()
async def auth_headers(client: AsyncClient) -> dict[str, str]:
    return {"Authorization": "Bearer test-token"}


@pytest.fixture()
def loguru_records():
    """Every loguru record emitted during the test (``record["extra"]`` included)."""
    from loguru import logger

    records: list[dict] = []
    handler_id = logger.add(
        lambda message: records.append(message.record),
        level="DEBUG",
        format="{message}",
    )
    try:
        yield records
    finally:
        logger.remove(handler_id)


@pytest.fixture()
def caplog_loguru(loguru_records):
    """Rendered text of the records, as a live list (message + exception type)."""

    class _Lines(list):
        def __iter__(self):
            return iter(
                f"{r['message']} {r['exception'].type.__name__ if r['exception'] else ''}"
                for r in loguru_records
            )

    return _Lines(loguru_records)
