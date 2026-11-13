from dataclasses import dataclass

from app.modules.business.ai_assistant.adapters.listing import GrpcListingReader
from app.modules.business.ai_assistant.adapters.search import GrpcListingSearch


@dataclass(frozen=True, slots=True)
class AssistantClients:
    search: GrpcListingSearch
    listing: GrpcListingReader
