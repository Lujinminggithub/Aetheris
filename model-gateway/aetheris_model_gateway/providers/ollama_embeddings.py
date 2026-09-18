import json
import math
import socket
from urllib.error import HTTPError, URLError
from urllib.request import Request

from .base import HTTPProvider
from ..protocols import EmbeddingRequest, EmbeddingResponse


class OllamaEmbeddingProvider(HTTPProvider):
    def __init__(self, base_url="http://127.0.0.1:11434", default_model="embeddinggemma", num_threads=6, **kwargs):
        super().__init__(**kwargs)
        self.base_url = base_url.rstrip("/")
        self.url = self.base_url + "/api/embed"
        self.tags_url = self.base_url + "/api/tags"
        self.default_model = default_model
        self.num_threads = max(1, min(int(num_threads), 16))

    def embed(self, request: EmbeddingRequest) -> EmbeddingResponse:
        model = self.default_model if request.model in {"", "default"} else request.model
        payload = {"model": model, "input": request.inputs, "truncate": True, "options": {"num_thread": self.num_threads}}
        data, error, _ = self._request(self.url, payload, {"Content-Type": "application/json"}, request)
        if error:
            return EmbeddingResponse("error", model, error=error)
        vectors = data.get("embeddings") if isinstance(data, dict) else None
        if not self._valid_vectors(vectors, len(request.inputs)):
            return EmbeddingResponse("error", model, error="invalid_embeddings")
        return EmbeddingResponse("succeeded", str(data.get("model") or model), len(vectors[0]), vectors)

    def readiness(self) -> dict:
        try:
            request = Request(self.tags_url, headers={"Accept": "application/json"}, method="GET")
            with self.opener(request, timeout=self.timeout) as response:
                data = json.loads(response.read(self.max_response_bytes + 1).decode("utf-8"))
            names = {str(item.get("name", "")).split(":")[0] for item in data.get("models", []) if isinstance(item, dict)}
            if self.default_model.split(":")[0] not in names:
                return {"status": "unavailable", "provider": "ollama", "model": self.default_model, "error": "embedding_model_not_found"}
            return {"status": "ready", "provider": "ollama", "model": self.default_model}
        except (HTTPError, URLError, OSError, TimeoutError, socket.timeout, UnicodeDecodeError, json.JSONDecodeError):
            return {"status": "unavailable", "provider": "ollama", "model": self.default_model, "error": "embedding_provider_unavailable"}

    @staticmethod
    def _valid_vectors(vectors, expected: int) -> bool:
        if not isinstance(vectors, list) or len(vectors) != expected or not vectors:
            return False
        dimension = len(vectors[0]) if isinstance(vectors[0], list) else 0
        return dimension > 0 and all(isinstance(vector, list) and len(vector) == dimension and all(isinstance(value, (int, float)) and math.isfinite(value) for value in vector) for vector in vectors)
