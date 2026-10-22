from __future__ import annotations

from src.api.services import BaseService
from src.api.services.base_service import GatewayError
from src.core import ServiceFactory
from src.utils import get_test_data_manager

_VOUCHER = "/platform.promotion.v1.VoucherService/"

KNOWN_VOUCHERS: dict[str, dict[str, str]] = {
    "SAVE10": {
        "scope": "VOUCHER_SCOPE_PLATFORM",
        "discountType": "DISCOUNT_TYPE_PERCENT",
        "discountValue": "10",
        "minSpend": "0",
        "maxDiscount": "0",
        "quota": "1000000",
        "startsAt": "2026-01-01T00:00:00Z",
        "endsAt": "2030-12-31T23:59:59Z",
    },
}

_seeded: set[str] = set()


def ensure_known_voucher(code: str) -> None:
    spec = KNOWN_VOUCHERS.get(code)
    if spec is None or code in _seeded:
        return
    admin = get_test_data_manager().get_user_by_role("admin")
    sf = ServiceFactory()
    try:
        token = sf.auth.login(admin.username, admin.password)
    finally:
        sf.close()
    api = BaseService(token=token)
    try:
        try:
            found = api.post(_VOUCHER + "GetVoucher", {"code": code})
        except GatewayError as err:
            if err.status != 404:
                raise
            found = {}
        if (found.get("voucher") or {}).get("code") != code:
            created = api.post(_VOUCHER + "CreateVoucher", {"code": code, **spec})
            assert (created.get("voucher") or {}).get(
                "code"
            ) == code, f"CreateVoucher {code} failed: {created}"
    finally:
        api.close()
    _seeded.add(code)
