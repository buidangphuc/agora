from __future__ import annotations

from fastapi import Request

from app.modules.business.ai_assistant.service import AIAssistantService


def get_ai_service(request: Request) -> AIAssistantService:
    resources = getattr(request.app.state, "resources", None)
    if resources is not None:
        service = getattr(resources, "ai_service", None)
        if service is not None:
            return service
        # If resources exist but ai_service not yet cached, create and cache it
        rag_service = getattr(resources, "rag_service", None)
        service = AIAssistantService(rag_service=rag_service)
        resources.ai_service = service
        return service
    return AIAssistantService()
