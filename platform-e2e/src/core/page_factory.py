"""Page factory (mirrors bds `PageFactory`).

Resolves a human-readable `PageName` (used in Gherkin) to a page object, caching
one instance per scenario. Generic steps navigate/assert by name; typed call
sites use the returned object directly.
"""

from __future__ import annotations

from playwright.sync_api import Page

from src.constants import PageName
from src.core.base_page import BasePage
from src.pages import (
    AddressesPage,
    AssistantPage,
    CartPage,
    ChatPage,
    CheckoutPage,
    CockpitPage,
    FavoritesPage,
    HomePage,
    ListingDetailPage,
    LoginPage,
    NotificationsPage,
    OrderDetailPage,
    RegisterPage,
    SearchPage,
    SellerAnalyticsPage,
    SellerEditListingPage,
    SellerListingsPage,
    SellerNewListingPage,
    SellerOrdersPage,
    ShopProfilePage,
    VouchersPage,
)

_REGISTRY: dict[PageName, type[BasePage]] = {
    PageName.HOME: HomePage,
    PageName.LOGIN: LoginPage,
    PageName.REGISTER: RegisterPage,
    PageName.FAVORITES: FavoritesPage,
    PageName.NOTIFICATIONS: NotificationsPage,
    PageName.CHAT: ChatPage,
    PageName.ADDRESSES: AddressesPage,
    PageName.SELLER_EDIT_LISTING: SellerEditListingPage,
    PageName.SEARCH: SearchPage,
    PageName.LISTING_DETAIL: ListingDetailPage,
    PageName.SHOP_PROFILE: ShopProfilePage,
    PageName.CART: CartPage,
    PageName.CHECKOUT: CheckoutPage,
    PageName.ACCOUNT_ORDERS: OrderDetailPage,
    PageName.ORDER_DETAIL: OrderDetailPage,
    PageName.VOUCHERS: VouchersPage,
    PageName.ASSISTANT: AssistantPage,
    PageName.SELLER_LISTINGS: SellerListingsPage,
    PageName.SELLER_NEW_LISTING: SellerNewListingPage,
    PageName.SELLER_ORDERS: SellerOrdersPage,
    PageName.SELLER_ANALYTICS: SellerAnalyticsPage,
    PageName.ADMIN_COCKPIT: CockpitPage,
}


class PageFactory:
    def __init__(self, page: Page) -> None:
        self._page = page
        self._cache: dict[PageName, BasePage] = {}

    def get(self, name: PageName | str) -> BasePage:
        key = PageName(name) if not isinstance(name, PageName) else name
        if key not in self._cache:
            self._cache[key] = _REGISTRY[key](self._page)
        return self._cache[key]
