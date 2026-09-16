import uuid

from .base import HTTPProvider
from ..protocols import ModelRequest, ModelResponse


class DifyProvider(HTTPProvider):
    provider_name = "dify"

    def __init__(self, api_url, api_key=None, app_id=None, workflow_id=None, **kwargs):
        if not api_key:
            raise ValueError("缺少 DIFY_API_KEY")
        if not app_id and not workflow_id:
            raise ValueError("缺少 DIFY_APP_ID 或 DIFY_WORKFLOW_ID")
        super().__init__(**kwargs)
        self.api_url = api_url.rstrip("/")
        self.api_key = api_key
        self.app_id = app_id or ""
        self.workflow_id = workflow_id or ""

    def generate(self, request: ModelRequest) -> ModelResponse:
        workflow = bool(self.workflow_id and not self.app_id)
        endpoint = "/v1/workflows/run" if workflow else "/v1/chat-messages"
        payload = {"inputs": request.context, "response_mode": "blocking", "user": request.actor_id}
        if workflow:
            payload["inputs"] = {**request.context, "messages": request.messages, "task": request.task}
            payload["workflow_id"] = self.workflow_id
        else:
            payload.update({"query": request.messages[-1].get("content", "") if request.messages else request.task, "conversation_id": ""})
            payload["inputs"] = {**request.context, "app_id": self.app_id}
        data, error, _ = self._request(self.api_url + endpoint, payload, {"Content-Type": "application/json", "Authorization": f"Bearer {self.api_key}", "X-Api-Key": self.api_key}, request)
        if error:
            return self._error(request, error)
        if not isinstance(data, dict):
            return self._error(request, "invalid_response")
        result = data.get("answer")
        if result is None and isinstance(data.get("data"), dict):
            result = data["data"].get("outputs")
        if result is None:
            return self._error(request, "invalid_response")
        return ModelResponse(data.get("message_id", str(uuid.uuid4())), "succeeded", self.provider_name, request.model, result=result)

