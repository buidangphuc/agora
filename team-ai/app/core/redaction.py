import re
from collections.abc import Mapping
from typing import Literal, cast

TraceContentMode = Literal["off", "redacted", "full"]

_EMAIL = re.compile(r"[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}")
_SECRET_PATTERNS = (
    re.compile(r"\bsk-[A-Za-z0-9_-]+\b"),
    re.compile(r"\bBearer\s+\S+\b"),
    re.compile(r"\bAKIA[0-9A-Z]{16}\b"),  # AWS access key id
)
# 13-19 digits, optionally grouped by single spaces/dashes; Luhn decides.
_CARD_CANDIDATE = re.compile(r"(?<!\d)\d(?:[ -]?\d){12,18}(?!\d)")
# Vietnamese numbers (0xxxxxxxxx / +84xxxxxxxxx) and E.164-style international ones.
_PHONE = re.compile(
    r"(?<![\w+])(?:\+84|0)(?:[ .-]?\d){9}(?!\d)"
    r"|(?<![\w+])\+\d{1,3}(?:[ .-]?\d){7,12}(?!\d)"
)
# CMND (9 digits) / CCCD (12 digits) are masked ONLY right after an ID keyword:
# bare 9/12-digit numbers are indistinguishable from VND prices (850000000) and order
# numbers. Keyword list is deliberately specific ("id" alone would hit order ids).
_VN_NATIONAL_ID = re.compile(
    r"(?i)\b((?:cmnd|cccd|cmt|c\u0103n\s+c\u01b0\u1edbc(?:\s+c\u00f4ng\s+d\u00e2n)?"
    r"|id\s*card|identity\s*card|national\s*id|citizen\s*id)"
    r"(?:\s*(?:s\u1ed1|so|no\.?|number))?\s*[:#]?\s*)(?<!\d)(?:\d{12}|\d{9})(?!\d)"
)
_SECRET_KEY_MARKERS = (
    "api_key",
    "apikey",
    "authorization",
    "cookie",
    "token",
    "password",
    "secret",
    "pepper",
)


def _luhn_valid(digits: str) -> bool:
    total = 0
    for index, char in enumerate(reversed(digits)):
        digit = int(char)
        if index % 2 == 1:
            digit *= 2
            if digit > 9:
                digit -= 9
        total += digit
    return total % 10 == 0


def _mask_card(match: re.Match[str]) -> str:
    digits = re.sub(r"\D", "", match.group(0))
    if 13 <= len(digits) <= 19 and _luhn_valid(digits):
        return "[card]"
    return match.group(0)


class RedactionPolicy:
    def __init__(
        self, *, mode: TraceContentMode = "redacted", mask_national_id: bool = False
    ) -> None:
        self.mode = mode
        # Keyword-anchored CMND/CCCD masking is for text sent to a model; trace and
        # log redaction leave it off.
        self.mask_national_id = mask_national_id

    @classmethod
    def from_trace_content(cls, trace_content: str) -> "RedactionPolicy":
        if trace_content in {"off", "redacted", "full"}:
            return cls(mode=cast(TraceContentMode, trace_content))
        return cls(mode="redacted")

    @classmethod
    def for_llm_input(cls) -> "RedactionPolicy":
        """Policy for text handed to the model: ALWAYS masked.

        Independent of ``LLM_TRACE_CONTENT``: ``off`` would blank the whole message
        and make chat useless, and ``full`` only governs what logs and traces may
        hold, never what leaves for the provider.
        """
        return cls(mode="redacted", mask_national_id=True)

    def redact_text(self, value: str) -> str:
        if self.mode == "full":
            return value
        if self.mode == "off":
            return "[redacted]"

        redacted = _EMAIL.sub("[email]", value)
        for pattern in _SECRET_PATTERNS:
            redacted = pattern.sub("[secret]", redacted)
        redacted = _CARD_CANDIDATE.sub(_mask_card, redacted)
        redacted = _PHONE.sub("[phone]", redacted)
        if self.mask_national_id:
            redacted = _VN_NATIONAL_ID.sub(r"\1[id]", redacted)
        return redacted

    def redact_mapping(self, payload: Mapping[str, object]) -> dict[str, object]:
        if self.mode == "full":
            return dict(payload)
        if self.mode == "off":
            return dict.fromkeys(payload, "[redacted]")

        redacted: dict[str, object] = {}
        for key, value in payload.items():
            if self._is_secret_key(key):
                redacted[key] = "[secret]"
            else:
                redacted[key] = self._redact_value(value)
        return redacted

    def _redact_value(self, value: object) -> object:
        if isinstance(value, str):
            return self.redact_text(value)
        if isinstance(value, Mapping):
            return self.redact_mapping(value)
        if isinstance(value, list | tuple):
            return [self._redact_value(item) for item in value]
        return value

    def _is_secret_key(self, key: str) -> bool:
        normalized = key.lower().replace("-", "_")
        return any(marker in normalized for marker in _SECRET_KEY_MARKERS)
