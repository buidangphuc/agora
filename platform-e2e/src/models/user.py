"""Domain model: a marketplace user/account under test."""

from __future__ import annotations

import base64
import json
from dataclasses import dataclass


@dataclass
class User:
    username: str
    password: str
    role: str  # "buyer" | "seller" | "admin"
    token: str | None = None  # JWT captured after API login

    @property
    def is_seller(self) -> bool:
        return self.role == "seller"

    @property
    def user_id(self) -> str:
        """The JWT `sub` (the id services key a user's data by); empty before login."""
        if not self.token:
            return ""
        try:
            payload = self.token.split(".")[1]
            payload += "=" * (-len(payload) % 4)
            return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")
        except (IndexError, ValueError):
            return ""
