from app.modules.business.chat.echo import EchoChatService


async def test_echo_streams_word_by_word():
    chat = EchoChatService()

    deltas = [
        delta
        async for delta in chat.stream(
            "xin  chào",
            session_id="s",
            principal_id="u",
            request_id="r",
            deadline_seconds=None,
        )
    ]

    assert deltas == ["echo: ", "xin ", "chào "]
