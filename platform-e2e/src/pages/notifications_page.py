"""Notification center (`/notifications`)."""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class NotificationsPage(BasePage):
    path = routes.NOTIFICATIONS
    name = "notifications"

    @property
    def heading(self) -> Locator:
        return self.page.get_by_text("Trung tâm thông báo", exact=False).first

    def is_displayed(self) -> bool:
        return "/notifications" in self.page.url

    # ── Alert subscriptions management (team-notification) ──────────────
    @property
    def alert_subscriptions(self) -> Locator:
        return self.page.get_by_test_id("alert-subscription")

    def alert_subscription(self, alert_type: str) -> Locator:
        """A subscription row by data-type ("price_drop" | "back_in_stock")."""
        return self.page.locator(f'[data-testid="alert-subscription"][data-type="{alert_type}"]')

    def unsubscribe(self, alert_type: str) -> None:
        self.alert_subscription(alert_type).get_by_role("button", name="Hủy").click()

    # ── Notification list (team-notification) ───────────────────────────
    @property
    def notification_items(self) -> Locator:
        return self.page.get_by_test_id("notification-item")

    def notifications_of_type(self, notif_type: str) -> Locator:
        """Notification rows by data-type, e.g. "price_drop" | "back_in_stock"."""
        return self.page.locator(f'[data-testid="notification-item"][data-type="{notif_type}"]')
