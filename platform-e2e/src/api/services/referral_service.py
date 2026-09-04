"""Referral service: create a referral code, read my referral, list rewards.

Gateway Connect/JSON RPCs for platform.referral.v1.ReferralService. Self-contained
endpoint paths so this wrapper needs no edit to the shared gateway_endpoints barrel.
"""

from __future__ import annotations

from .base_service import BaseService

_SVC = "/platform.referral.v1.ReferralService"


class ReferralService(BaseService):
    def create_referral_code(self) -> dict:
        return self.post(f"{_SVC}/CreateReferralCode", {})

    def get_my_referral(self) -> dict:
        return self.post(f"{_SVC}/GetMyReferral", {})

    def list_referral_rewards(self, page_size: int = 20) -> dict:
        return self.post(f"{_SVC}/ListReferralRewards", {"page": {"pageSize": page_size}})
