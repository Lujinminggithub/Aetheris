import json
import socket
from urllib.error import HTTPError, URLError
from urllib.request import Request

from .base import HTTPProvider
from ..protocols import ModelRequest, ModelResponse


class OllamaProvider(HTTPProvider):
    provider_name = "ollama"

    def __init__(self, base_url="http://127.0.0.1:11434", default_model="qwen3:4b-instruct", **kwargs):
        super().__init__(**kwargs)
        self.base_url = base_url.rstrip("/")
        self.url = self.base_url + "/api/chat"
        self.tags_url = self.base_url + "/api/tags"
        self.default_model = default_model

    def generate(self, request: ModelRequest) -> ModelResponse:
        model = self.default_model if request.model in {"", "default"} else request.model
        messages = list(request.messages)
        if request.context:
            context = json.dumps(request.context, ensure_ascii=False, sort_keys=True)
            messages.append({"role": "user", "content": "请根据以下结构化数据生成简洁、可追溯的中文总结：\n" + context})
        answer_mode = str(request.context.get("answer_mode", "direct")) if isinstance(request.context, dict) else "direct"
        if request.task == "plan_rag_answer":
            num_predict = 1024
        elif request.task == "rag_answer" and answer_mode == "analysis":
            num_predict = 2048
        elif request.task == "rag_answer" and answer_mode in {"reason", "procedure", "numeric"}:
            num_predict = 768
        else:
            num_predict = 256
        payload = {"model": model, "messages": messages, "stream": False, "options": {"num_predict": num_predict, "temperature": 0.2}}
        data, error, _ = self._request(self.url, payload, {"Content-Type": "application/json"}, request)
        if error:
            response = self._error(request, error)
            response.model = model
            return response
        message = data.get("message") if isinstance(data, dict) else None
        if not isinstance(message, dict) or "content" not in message:
            response = self._error(request, "invalid_response")
            response.model = model
            return response
        return ModelResponse(data.get("id", ""), "succeeded", self.provider_name, model, result=message["content"])

    def readiness(self) -> dict:
        try:
            request = Request(self.tags_url, headers={"Accept": "application/json"}, method="GET")
            with self.opener(request, timeout=self.timeout) as response:
                raw = response.read(self.max_response_bytes + 1)
                if len(raw) > self.max_response_bytes:
                    return self._readiness_error("response_too_large")
                data = json.loads(raw.decode("utf-8"))
        except HTTPError as exc:
            return self._readiness_error(f"http_{exc.code}")
        except (TimeoutError, socket.timeout, URLError, OSError):
            return self._readiness_error("connection_error")
        except (UnicodeDecodeError, json.JSONDecodeError, TypeError, ValueError):
            return self._readiness_error("invalid_json")
        names = {item.get("name") for item in data.get("models", []) if isinstance(item, dict)} if isinstance(data, dict) else set()
        if self.default_model not in names:
            return self._readiness_error("model_not_found")
        return {"status": "ready", "provider": self.provider_name, "model": self.default_model}

    def _readiness_error(self, error: str) -> dict:
        return {"status": "unavailable", "provider": self.provider_name, "model": self.default_model, "error": error}
