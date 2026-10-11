from unittest.mock import patch


def test_a(monkeypatch):
    with patch("stripe.Charge.create"):  # external, fine
        pass
    with patch("app.llm.openai_call"):  # asdlc: allow-mock external LLM API
        pass
    # asdlc: allow-mock external gateway
    monkeypatch.setattr("app.payments.gateway", None)
