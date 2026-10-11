from app.core.redaction import RedactionPolicy


def test_redaction_policy_masks_email_api_key_and_bearer_tokens():
    policy = RedactionPolicy(mode="redacted")

    redacted = policy.redact_text(
        "Email admin@example.com with key sk-test123 and Bearer token-value."
    )

    assert "admin@example.com" not in redacted
    assert "sk-test123" not in redacted
    assert "Bearer token-value" not in redacted
    assert "[email]" in redacted
    assert "[secret]" in redacted


def test_redaction_policy_can_hide_or_preserve_content():
    assert RedactionPolicy(mode="off").redact_text("secret text") == "[redacted]"
    assert RedactionPolicy(mode="full").redact_text("secret text") == "secret text"


def test_redaction_policy_redacts_nested_mapping():
    policy = RedactionPolicy(mode="redacted")

    payload = policy.redact_mapping(
        {
            "prompt": "Contact admin@example.com",
            "nested": {"api_key": "sk-test123"},  # pragma: allowlist secret
        }
    )

    assert payload == {
        "prompt": "Contact [email]",
        "nested": {"api_key": "[secret]"},
    }


def test_redaction_masks_vietnamese_mobile_formats():
    policy = RedactionPolicy(mode="redacted")

    for phone in (
        "0912345678",
        "+84 912 345 678",
        "+84912345678",
        "84912345678",
        "091-234-5678",
        "0912.345.678",
        "0912 345 678",
        "0356789012",
    ):
        redacted = policy.redact_text(f"goi {phone} nhe")
        assert redacted == "goi [phone] nhe", phone


def _ids() -> RedactionPolicy:
    return RedactionPolicy(mode="redacted", mask_national_id=True)


def test_national_id_masked_only_after_an_id_keyword():
    policy = _ids()

    assert policy.redact_text("CCCD 079123456789 cua toi") == "CCCD [id] cua toi"
    assert policy.redact_text("CMND: 123456789.") == "CMND: [id]."
    assert policy.redact_text("cmnd:123456789") == "cmnd:[id]"
    assert policy.redact_text("Số CMT 123456789") == "Số CMT [id]"
    assert policy.redact_text("căn cước công dân số 079123456789") == (
        "căn cước công dân số [id]"
    )
    assert policy.redact_text("ID card: 079123456789") == "ID card: [id]"


def test_bare_numbers_are_never_masked_as_national_ids():
    policy = _ids()
    text = (
        "Giá 850000000 VND, 2500000000 đ, 1 tỷ 200, đơn 123456789 và "
        "mã 001203004567, order id 123456789"
    )

    assert policy.redact_text(text) == text


def test_national_id_masking_is_off_by_default_even_with_keyword():
    assert RedactionPolicy(mode="redacted").redact_text("CCCD 079123456789") == (
        "CCCD 079123456789"
    )


def test_redaction_matches_phone_before_national_id():
    policy = _ids()

    assert policy.redact_text("0912345678") == "[phone]"
    assert policy.redact_text("so 0912345678 va cccd 001203004567") == (
        "so [phone] va cccd [id]"
    )


def test_redaction_leaves_order_numbers_and_prices_untouched():
    policy = _ids()
    text = (
        "Don DH-20240915-001 gia 1.500.000d, 250000 VND, ma #12345678, "
        "ship 30k, 12 san pham, SKU A1234567890B"
    )

    assert policy.redact_text(text) == text


def test_redaction_off_and_full_modes_unchanged_for_pii():
    raw = "call 0912345678 cccd 001203004567"

    assert RedactionPolicy(mode="full").redact_text(raw) == raw
    assert RedactionPolicy(mode="off").redact_text(raw) == "[redacted]"


def test_llm_input_policy_always_masks_whatever_the_trace_content_mode_is():
    policy = RedactionPolicy.for_llm_input()

    assert policy.mode == "redacted"
    assert policy.redact_text("call 0912345678, cccd 079123456789") == (
        "call [phone], cccd [id]"
    )
    # Unlike the log policy, it never blanks ("off") or passes through ("full").
    assert RedactionPolicy(mode="off").redact_text("hi") == "[redacted]"
    assert RedactionPolicy(mode="full").redact_text("0912345678") == "0912345678"


def test_redact_mapping_masks_phone_in_values():
    policy = RedactionPolicy(mode="redacted")

    assert policy.redact_mapping({"note": "tel 0912345678"}) == {"note": "tel [phone]"}
