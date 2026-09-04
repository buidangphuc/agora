"""Core framework: base page/component + factories."""

from .base_component import BaseComponent
from .base_page import BasePage
from .page_factory import PageFactory
from .service_factory import ServiceFactory

__all__ = ["BasePage", "BaseComponent", "PageFactory", "ServiceFactory"]
