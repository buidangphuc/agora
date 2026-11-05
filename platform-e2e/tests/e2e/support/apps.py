"""Route a path to the Next app that serves it: seller pages live on SELLER_BASE_URL, admin
pages on ADMIN_BASE_URL, everything else on BASE_URL."""

from __future__ import annotations

__all__ = ["app_base", "app_url", "is_admin_path", "is_seller_path"]

SELLER_ROOTS = ("/seller", "/sell")
ADMIN_ROOTS = ("/admin",)


def _under(path: str, roots: tuple[str, ...]) -> bool:
    for root in roots:
        if path == root or path.startswith((f"{root}/", f"{root}?", f"{root}#")):
            return True
    return False


def is_seller_path(path: str) -> bool:
    return _under(path, SELLER_ROOTS)


def is_admin_path(path: str) -> bool:
    return _under(path, ADMIN_ROOTS)


def app_base(settings, path: str) -> str:
    if is_seller_path(path):
        return settings.seller_base_url.rstrip("/")
    if is_admin_path(path):
        return settings.admin_base_url.rstrip("/")
    return settings.base_url.rstrip("/")


def app_url(settings, path: str) -> str:
    return f"{app_base(settings, path)}{path}"
