"""Identity auth service: register + login through the gateway.

Login/Register are public. Both return `{ "result": { "token": "<jwt>" } }`; that
token is what the frontend stores in the `session` cookie, so it doubles as the
value we inject for fast browser auth.
"""

from __future__ import annotations

from src.constants import gateway_endpoints as ep

from .base_service import BaseService


class AuthService(BaseService):
    def register(self, username: str, password: str, role: str = "buyer") -> str:
        """Register a user; returns the JWT. Ignores AlreadyExists by re-login."""
        try:
            data = self.post(
                ep.AUTH_REGISTER, {"username": username, "password": password, "role": role}
            )
            return _token(data)
        except Exception:  # noqa: BLE001 - account may already exist; fall back to login
            return self.login(username, password)

    def login(self, username: str, password: str) -> str:
        data = self.post(ep.AUTH_LOGIN, {"username": username, "password": password})
        token = _token(data)
        if not token:
            raise RuntimeError(f"Login for {username!r} returned no token")
        self.set_token(token)
        return token


def _token(data: dict) -> str:
    return (data.get("result") or {}).get("token", "")
