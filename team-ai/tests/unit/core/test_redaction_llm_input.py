from app.core.redaction import RedactionPolicy


def test_keyword_anchored_id_is_masked_and_the_keyword_kept():
    redacted = RedactionPolicy.for_llm_input().redact_text(
        "CCCD: 012345678901 giá 850000000"
    )

    assert redacted == "CCCD: [id] giá 850000000"


def test_nine_digit_cmnd_after_a_keyword_is_masked():
    policy = RedactionPolicy.for_llm_input()

    assert policy.redact_text("số CMND 123456789") == "số CMND [id]"
    assert policy.redact_text("căn cước công dân số 012345678901") == (
        "căn cước công dân số [id]"
    )


def test_a_number_without_a_keyword_is_untouched():
    policy = RedactionPolicy.for_llm_input()

    assert policy.redact_text("giá 850000000 đơn 123456789") == (
        "giá 850000000 đơn 123456789"
    )


def test_for_llm_input_still_masks_email_and_phone():
    redacted = RedactionPolicy.for_llm_input().redact_text("a@b.co 0912345678")

    assert redacted == "[email] [phone]"


def test_plain_policy_does_not_mask_ids():
    assert RedactionPolicy(mode="redacted").redact_text("CCCD: 012345678901") == (
        "CCCD: 012345678901"
    )
