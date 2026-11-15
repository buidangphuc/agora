"""Legal versions the default stack treats as current (change identity-legal-consent).

Defaults match LEGAL_TERMS_VERSIONS / PRIVACY_POLICY_VERSIONS of the default stack; override with
E2E_LEGAL_TERMS_VERSION / E2E_PRIVACY_POLICY_VERSION when the stack is configured differently.
"""

from __future__ import annotations

import os

CURRENT_TERMS = os.getenv("E2E_LEGAL_TERMS_VERSION", "2026-05")
CURRENT_PRIVACY = os.getenv("E2E_PRIVACY_POLICY_VERSION", "2026-05")
NEW_TERMS = os.getenv("E2E_LEGAL_TERMS_NEW_VERSION", "2026-11")


def versions() -> dict[str, str]:
    """The two accepted-version fields every Register body carries."""
    return {"termsVersion": CURRENT_TERMS, "privacyVersion": CURRENT_PRIVACY}
