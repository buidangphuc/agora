"""Frontend route paths (team-frontend Next.js App Router, :3000).

Paths are relative to BASE_URL. Dynamic segments are format templates.
"""

from __future__ import annotations

HOME = "/"
LOGIN = "/login"
REGISTER = "/register"
SEARCH = "/search"  # ?q=&category=&minPrice=&maxPrice=&sort=
LISTING_DETAIL = "/listing/{listing_id}"
SHOP = "/shop/{shop_id}"
CART = "/cart"
CHECKOUT = "/checkout"
CHECKOUT_PAY = "/checkout/pay/{order_id}"
FAVORITES = "/favorites"
VOUCHERS = "/vouchers"
NOTIFICATIONS = "/notifications"
CHAT = "/chat"
ASSISTANT = "/assistant"
ACCOUNT_ORDERS = "/account/orders"
ACCOUNT_ADDRESSES = "/account/addresses"

# Seller area (gated: requires principal + `listing.write` scope)
SELLER = "/seller"
SELLER_NEW = "/seller/new"
SELLER_EDIT = "/seller/{listing_id}/edit"
SELLER_ORDERS = "/seller/orders"
SELLER_ANALYTICS = "/seller/analytics"

# Admin
ADMIN_COCKPIT = "/admin/cockpit"
