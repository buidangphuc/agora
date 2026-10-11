import re
from collections.abc import Mapping
from typing import Literal, cast

TraceContentMode = Literal["off", "redacted", "full"]

# Vietnamese mobile: 0 or +84/84 prefix, network digit 3/5/7/8/9, then 8 more digits,
# each optionally separated by space/dot/dash (0912345678, +84 912 345 678,
# 091-234-5678). Digit-bounded so longer numbers are not partially matched.
_VN_PHONE = re.compile(r"(?<!\d)(?:\+?84|0)[\s.-]?[35789](?:[\s.-]?\d){8}(?!\d)")
# CMND (9 digits) / CCCD (12 digits) are masked ONLY right after an ID keyword:
# bare 9/12-digit numbers are indistinguishable from VND prices (850000000) and order
# numbers. Keyword list is deliberately specific ("id" alone would hit order ids).
_VN_NATIONAL_ID = re.compile(
    r"(?i)\b((?:cmnd|cccd|cmt|c\u0103n\s+c\u01b0\u1edbc(?:\s+c\u00f4ng\s+d\u00e2n)?"
    r"|id\s*card|identity\s*card|national\s*id|citizen\s*id)"
    r"(?:\s*(?:s\u1ed1|so|no\.?|number))?\s*[:#]?\s*)(?<!\d)(?:\d{12}|\d{9})(?!\d)"
)


class RedactionPolicy:
    def __init__(
        self, *, mode: TraceContentMode = "redacted", mask_national_id: bool = False
    ) -> None:
        self.mode = mode
        # Keyword-anchored CMND/CCCD masking (a 9/12-digit number is masked only
        # right after an ID keyword) is for the LLM-input / log path only; document
        # indexing (RAG) leaves it off so catalog numbers are untouched.
        self.mask_national_id = mask_national_id

    @classmethod
    def from_trace_content(
        cls, trace_content: str, *, mask_national_id: bool = False
    ) -> "RedactionPolicy":
        mode: TraceContentMode = "redacted"
        if trace_content in {"off", "redacted", "full"}:
            mode = cast(TraceContentMode, trace_content)
        return cls(mode=mode, mask_national_id=mask_national_id)

    def redact_text(self, value: str) -> str:
        if self.mode == "full":
            return value
        if self.mode == "off":
            return "[redacted]"

        redacted = re.sub(r"[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}", "[email]", value)
        redacted = re.sub(r"\bsk-[A-Za-z0-9_-]+\b", "[secret]", redacted)
        redacted = re.sub(r"\bBearer\s+\S+\b", "[secret]", redacted)
        redacted = _VN_PHONE.sub("[phone]", redacted)
        if self.mask_national_id:
            redacted = _VN_NATIONAL_ID.sub(r"\1[id]", redacted)
        return redacted

    @classmethod
    def for_llm_input(cls) -> "RedactionPolicy":
        """Policy for text handed to the model: ALWAYS masked.

        Independent of ``LLM_TRACE_CONTENT``: ``off`` would blank the whole message
        and make chat useless, and ``full`` only governs what logs and traces may
        hold, never what leaves for the provider. Masks email, ``sk-`` keys, bearer
        tokens, Vietnamese mobile numbers and keyword-anchored CMND/CCCD numbers.
        """
        return cls(mode="redacted", mask_national_id=True)

    def redact_mapping(self, payload: Mapping[str, object]) -> dict[str, object]:
        if self.mode == "full":
            return dict(payload)
        if self.mode == "off":
            return dict.fromkeys(payload, "[redacted]")

        redacted: dict[str, object] = {}
        for key, value in payload.items():
            if self._is_secret_key(key):
                redacted[key] = "[secret]"
            elif isinstance(value, str):
                redacted[key] = self.redact_text(value)
            elif isinstance(value, Mapping):
                redacted[key] = self.redact_mapping(value)
            elif isinstance(value, list):
                redacted[key] = [
                    self.redact_mapping(item) if isinstance(item, Mapping) else item
                    for item in value
                ]
            else:
                redacted[key] = value
        return redacted

    def _is_secret_key(self, key: str) -> bool:
        normalized = key.lower()
        return any(
            marker in normalized
            for marker in ("api_key", "token", "password", "secret", "pepper")
        )
