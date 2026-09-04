"""Domain model: a marketplace user/account under test."""

from __future__ import annotations

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
