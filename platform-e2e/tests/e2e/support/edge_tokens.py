"""Sign bearer tokens with the LOCAL dev identity key (edge-route-policy scenarios).

The key is the one the root `docker-compose.services.yaml` hands to team-identity for
local runs (JWT_PRIVATE_KEY / JWT_KID). It is read from that file at run time, never
copied into the tests, and only ever works against the local stack. Signing shells out
to `openssl` so no JWT library is needed in the e2e venv.
"""

from __future__ import annotations

import base64
import json
import re
import subprocess
import tempfile
import time
from pathlib import Path

_COMPOSE = Path(__file__).resolve().parents[3].parent / "docker-compose.services.yaml"


def _b64(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def _dev_key() -> tuple[str, str]:
    text = _COMPOSE.read_text()
    key = re.search(r"JWT_PRIVATE_KEY='(-----BEGIN [^']+)'", text)
    kid = re.search(r"JWT_KID=([\w.-]+)", text)
    if not key or not kid:
        raise RuntimeError(f"no local JWT_PRIVATE_KEY / JWT_KID in {_COMPOSE}")
    return key.group(1).replace("\\n", "\n"), kid.group(1)


def sign_dev_token(claims: dict) -> str:
    """RS256 JWT over `claims` exactly as given (no exp/sub is added when omitted)."""
    pem, kid = _dev_key()
    header = {"alg": "RS256", "typ": "JWT", "kid": kid}
    signing_input = f"{_b64(json.dumps(header).encode())}.{_b64(json.dumps(claims).encode())}"
    with tempfile.NamedTemporaryFile("w", suffix=".pem") as keyfile:
        keyfile.write(pem)
        keyfile.flush()
        sig = subprocess.run(
            ["openssl", "dgst", "-sha256", "-sign", keyfile.name],
            input=signing_input.encode(),
            capture_output=True,
            check=True,
        ).stdout
    return f"{signing_input}.{_b64(sig)}"


def user_claims(sub: str, *, with_exp: bool = True) -> dict:
    """Claims of an ordinary signed-in user, mirroring what team-identity issues."""
    claims: dict = {
        "sub": sub,
        "typ": "user",
        "name": "e2e-edge",
        "scopes": [],
        "iat": int(time.time()),
    }
    if with_exp:
        claims["exp"] = int(time.time()) + 600
    return claims
