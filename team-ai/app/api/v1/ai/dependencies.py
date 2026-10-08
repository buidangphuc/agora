from __future__ import annotations

from fastapi import Request

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.service import AIAssistantService
from app.modules.business.tag_classifier.service import TagClassifierService


def get_ai_service(request: Request) -> AIAssistantService:
    resources = getattr(request.app.state, "resources", None)
    if resources is not None:
        service = getattr(resources, "ai_service", None)
        if service is not None:
            return service
        # If resources exist but ai_service not yet cached, create and cache it
        rag_service = getattr(resources, "rag_service", None)
        settings = getattr(request.app.state, "settings", None)
        policy = (
            RedactionPolicy.from_trace_content(
                settings.LLM_TRACE_CONTENT, mask_national_id=True
            )
            if settings is not None
            else None
        )
        service = AIAssistantService(rag_service=rag_service, redaction_policy=policy)
        resources.ai_service = service
        return service
    return AIAssistantService()


def get_tag_classifier_service(request: Request) -> TagClassifierService:
    resources = getattr(request.app.state, "resources", None)
    if resources is not None:
        service = getattr(resources, "tag_classifier_service", None)
        if service is not None:
            return service
        service = TagClassifierService()
        resources.tag_classifier_service = service
        return service
    return TagClassifierService()
