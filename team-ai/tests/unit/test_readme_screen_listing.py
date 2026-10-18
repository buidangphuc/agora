"""Task 5 docs checks: team-ai/README.md documents the listing screen."""

from pathlib import Path

README = (Path(__file__).resolve().parents[2] / "README.md").read_text()


def test_task5_readme_lists_screenlisting_rpc():
    assert "ScreenListing" in README


def test_task5_readme_documents_screen_backend_switch():
    assert "SCREEN_BACKEND" in README


def test_task5_readme_states_indexer_only_published():
    assert "PUBLISHED" in README
