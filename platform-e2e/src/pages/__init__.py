"""Barrel for page objects. New pages must be registered here and in PageFactory."""

from .addresses_page import AddressesPage
from .assistant_page import AssistantPage
from .cart_page import CartPage
from .chat_page import ChatPage
from .checkout_page import CheckoutPage
from .cockpit_page import CockpitPage
from .favorites_page import FavoritesPage
from .home_page import HomePage
from .listing_detail_page import ListingDetailPage
from .login_page import LoginPage
from .notifications_page import NotificationsPage
from .order_detail_page import OrderDetailPage
from .orders_list_page import OrdersListPage
from .payment_result_page import PaymentResultPage
from .register_page import RegisterPage
from .search_page import SearchPage
from .seller_analytics_page import SellerAnalyticsPage
from .seller_edit_listing_page import SellerEditListingPage
from .seller_listings_page import SellerListingsPage
from .seller_new_listing_page import SellerNewListingPage
from .seller_order_detail_page import SellerOrderDetailPage
from .seller_orders_page import SellerOrdersPage
from .seller_shop_page import SellerShopPage
from .seller_wallet_page import SellerWalletPage
from .shop_profile_page import ShopProfilePage
from .vouchers_page import VouchersPage

__all__ = [
    "HomePage",
    "LoginPage",
    "RegisterPage",
    "FavoritesPage",
    "NotificationsPage",
    "ChatPage",
    "AddressesPage",
    "SellerEditListingPage",
    "SearchPage",
    "ListingDetailPage",
    "ShopProfilePage",
    "CartPage",
    "CheckoutPage",
    "PaymentResultPage",
    "OrderDetailPage",
    "OrdersListPage",
    "VouchersPage",
    "AssistantPage",
    "SellerListingsPage",
    "SellerNewListingPage",
    "SellerOrdersPage",
    "SellerAnalyticsPage",
    "SellerWalletPage",
    "SellerShopPage",
    "SellerOrderDetailPage",
    "CockpitPage",
]
