from unittest import mock
from unittest.mock import patch
from app.services import orders


def test_a(monkeypatch):
    with patch("app.services.orders.create"):
        pass
    monkeypatch.setattr(orders, "create", lambda: 1)
    monkeypatch.setattr("app.config.X", 1)
    mock.patch("app.x.y")


def test_b(page):
    page.route("**/api/orders", lambda r: r.fulfill(status=200, body="{}"))
