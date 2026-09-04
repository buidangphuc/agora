"""Human-readable page names used in Gherkin, resolved by the PageFactory.

Keeps Gherkin readable (`the "home" page`) while the Enum gives call sites
type-safety the bds string-map lacked.
"""

from __future__ import annotations

from enum import Enum


class PageName(str, Enum):
    HOME = "home"
    LOGIN = "login"
    REGISTER = "register"
    SEARCH = "search"
    LISTING_DETAIL = "listing detail"
    SHOP_PROFILE = "shop profile"
    CART = "cart"
    CHECKOUT = "checkout"
    ACCOUNT_ORDERS = "account orders"
    ORDER_DETAIL = "order detail"
    FAVORITES = "favorites"
    VOUCHERS = "vouchers"
    NOTIFICATIONS = "notifications"
    CHAT = "chat"
    ADDRESSES = "addresses"
    ASSISTANT = "assistant"
    SELLER_LISTINGS = "seller listings"
    SELLER_NEW_LISTING = "seller new listing"
    SELLER_EDIT_LISTING = "seller edit listing"
    SELLER_ORDERS = "seller orders"
    SELLER_ANALYTICS = "seller analytics"
    ADMIN_COCKPIT = "admin cockpit"
