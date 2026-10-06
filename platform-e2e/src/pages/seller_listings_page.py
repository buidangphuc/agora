"""Seller Workplace (`/seller`): shell, KPI row, quick actions and product Table List.

Gated: requires `listing.write` scope. The shell locators (sidebar, collapse
toggle, mobile menu + Drawer, shop card) live here because every seller page
renders inside the same shell and `/seller` is its entry point.
"""

from __future__ import annotations

from playwright.sync_api import Locator

from src.constants import routes
from src.core.base_page import BasePage


class SellerListingsPage(BasePage):
    path = routes.SELLER
    name = "seller listings"

    # ── Shell (sidebar / Drawer / shop card) ─────────────────────────────
    @property
    def sidebar(self) -> Locator:
        return self.page.get_by_role("complementary")

    @property
    def collapse_toggle(self) -> Locator:
        return self.page.get_by_role("button", name="Thu gọn thanh bên")

    @property
    def expand_toggle(self) -> Locator:
        return self.page.get_by_role("button", name="Mở rộng thanh bên")

    @property
    def menu_button(self) -> Locator:
        return self.page.get_by_role("button", name="Mở menu người bán")

    @property
    def drawer(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def shop_name(self) -> Locator:
        """Shop card name (sidebar on desktop, Drawer on mobile): first visible one."""
        return self.page.get_by_test_id("seller-shop-name").first

    def nav_link(self, name: str) -> Locator:
        return self.page.get_by_role("link", name=name, exact=True).first

    def current_nav_links(self) -> Locator:
        """Every seller nav link that announces the current page."""
        return self.page.get_by_role("navigation", name="Kênh người bán").locator(
            'a[aria-current="page"]'
        )

    # ── Workplace ────────────────────────────────────────────────────────
    @property
    def kpi_row(self) -> Locator:
        return self.page.get_by_test_id("kpi-row")

    def kpi_cell(self, title: str) -> Locator:
        return self.kpi_row.get_by_text(title, exact=False).first

    @property
    def quick_actions(self) -> Locator:
        return self.page.get_by_role("heading", name="Thao tác nhanh").locator("xpath=../..")

    def quick_action(self, name: str) -> Locator:
        return self.quick_actions.get_by_role("link", name=name, exact=True)

    @property
    def recent_orders_empty(self) -> Locator:
        return self.page.get_by_text("Chưa có đơn hàng nào", exact=True)

    # ── Product Table List ───────────────────────────────────────────────
    @property
    def search_box(self) -> Locator:
        return self.page.get_by_role("searchbox", name="Tìm sản phẩm")

    @property
    def empty_products(self) -> Locator:
        return self.page.get_by_text("Shop chưa có sản phẩm", exact=True)

    @property
    def no_results(self) -> Locator:
        return self.page.get_by_text("Không có kết quả", exact=True)

    def product_row(self, title: str) -> Locator:
        return self.page.locator("tbody tr", has_text=title)

    def delete_button(self, title: str) -> Locator:
        return self.page.get_by_role("button", name=f"Xoá {title}")

    @property
    def delete_dialog(self) -> Locator:
        return self.page.get_by_role("dialog")

    @property
    def confirm_delete(self) -> Locator:
        return self.delete_dialog.get_by_role("button", name="Xoá sản phẩm")

    @property
    def cancel_delete(self) -> Locator:
        return self.delete_dialog.get_by_role("button", name="Huỷ")

    def toast(self, text: str) -> Locator:
        return self.page.get_by_text(text, exact=True)

    def listing_by_title(self, title: str) -> Locator:
        return self.page.get_by_text(title, exact=False)

    def has_listing(self, title: str) -> bool:
        return self.listing_by_title(title).first.is_visible()

    def is_displayed(self) -> bool:
        return "/seller" in self.page.url
