"""Base component object (mirrors bds `BaseComponent`).

A component scopes to a root that may be the whole page, a modal Locator, or an
iframe FrameLocator — all three expose `get_by_*` / `locator`, so components work
uniformly whether they wrap a section, a popup, or an embedded frame.
"""

from __future__ import annotations

from playwright.sync_api import FrameLocator, Locator, Page


class BaseComponent:
    def __init__(self, root: Page | Locator | FrameLocator) -> None:
        self.root = root
