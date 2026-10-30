"""Route a path to the Next app that serves it: seller pages live on SELLER_BASE_URL."""

from __future__ import annotations

from src.core.base_page import wait_streamed

__all__ = ["app_url", "is_seller_path", "wait_streamed"]

SELLER_ROOTS = ("/seller", "/sell")


def is_seller_path(path: str) -> bool:
    for root in SELLER_ROOTS:
        if path == root or path.startswith((f"{root}/", f"{root}?", f"{root}#")):
            return True
    return False


def app_url(settings, path: str) -> str:
    base = settings.seller_base_url if is_seller_path(path) else settings.base_url
    return f"{base.rstrip('/')}{path}"
