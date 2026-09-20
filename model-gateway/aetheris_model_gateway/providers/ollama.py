import json
import socket
from copy import deepcopy
from urllib.error import HTTPError, URLError
from urllib.request import Request

from .base import HTTPProvider
from ..protocols import ModelRequest, ModelResponse


ANSWER_FORMAT = {
    "type": "object",
    "properties": {
        "answer": {"type": "string"},
        "answer_mode": {"type": "string", "enum": ["direct", "numeric", "reason", "procedure", "analysis"]},
        "confidence": {"type": "string", "enum": ["high", "medium", "low"]},
        "details": {"type": "string"},
        "citation_numbers": {"type": "array", "items": {"type": "integer"}},
    },
    "required": ["answer", "answer_mode", "confidence", "details", "citation_numbers"],
    "additionalProperties": False,
}

PLAN_FORMAT = {
    "type": "object",
    "properties": {
        "question_intent": {"type": "string"},
        "required_topics": {"type": "array", "items": {"type": "string"}},
        "claims": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "claim": {"type": "string"},
                    "source_kind": {"type": "string"},
                    "knowledge_unit_ids": {"type": "array", "items": {"type": "string"}},
                    "applicability": {"type": "string"},
                },
                "required": ["claim", "source_kind", "knowledge_unit_ids", "applicability"],
                "additionalProperties": False,
            },
        },
        "coverage_gaps": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["question_intent", "required_topics", "claims", "coverage_gaps"],
    "additionalProperties": False,
}


class OllamaProvider(HTTPProvider):
    provider_name = "ollama"

    def __init__(self, base_url="http://127.0.0.1:11434", default_model="qwen3:1.7b", num_threads=6, **kwargs):
        kwargs.setdefault("retries", 0)
        super().__init__(**kwargs)
        self.base_url = base_url.rstrip("/")
        self.url = self.base_url + "/api/chat"
        self.tags_url = self.base_url + "/api/tags"
        self.default_model = default_model
        self.num_threads = max(1, min(int(num_threads), 16))

    def generate(self, request: ModelRequest) -> ModelResponse:
        model = self.default_model if request.model in {"", "default"} else request.model
        messages = list(request.messages)
        if request.context:
            context = json.dumps(request.context, ensure_ascii=False, sort_keys=True)
            messages.append({"role": "user", "content": "请根据以下结构化数据生成简洁、可追溯的中文总结：\n" + context})
        answer_mode = str(request.context.get("answer_mode", "direct")) if isinstance(request.context, dict) else "direct"
        if request.task == "plan_rag_answer":
            num_predict = 128
        elif request.task == "rag_answer" and answer_mode == "analysis":
            num_predict = 192
        elif request.task == "rag_answer" and answer_mode in {"reason", "procedure", "numeric"}:
            num_predict = 160
        else:
            num_predict = 96
        payload = {
            "model": model,
            "messages": messages,
            "stream": False,
            "think": False,
            "keep_alive": "30m",
            "options": {"num_ctx": 4096, "num_predict": num_predict, "num_thread": self.num_threads, "temperature": 0.2},
        }
        if request.task == "plan_rag_answer":
            payload["format"] = PLAN_FORMAT
        elif request.task == "rag_answer":
            answer_format = deepcopy(ANSWER_FORMAT)
            evidence = request.context.get("evidence", []) if isinstance(request.context, dict) else []
            has_verified = any(
                isinstance(item, dict) and item.get("validation_state") in {"verified", "platform_certified"}
                for item in evidence
            )
            if not has_verified:
                answer_format["properties"]["confidence"]["enum"] = ["medium", "low"]
            answer_format["properties"]["answer"]["minLength"] = 10
            if answer_mode == "analysis":
                answer_format["properties"]["details"]["minLength"] = 100
                answer_format["properties"]["details"]["maxLength"] = 220
            else:
                answer_format["properties"]["details"]["maxLength"] = 0
            payload["format"] = answer_format
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
