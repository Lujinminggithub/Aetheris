import json
import secrets
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from .config import load
from .protocols import EmbeddingRequest, ModelRequest
from .providers.dify import DifyProvider
from .providers.ollama import OllamaProvider
from .providers.ollama_embeddings import OllamaEmbeddingProvider


def _provider_from_config(config):
    if config.provider == "dify":
        return DifyProvider(config.dify_api_url, config.dify_api_key, config.dify_app_id, config.dify_workflow_id, timeout=config.timeout, max_response_bytes=config.max_response_bytes)
    return OllamaProvider(config.ollama_url, default_model=config.ollama_model, num_threads=config.ollama_generation_threads, timeout=config.timeout, max_response_bytes=config.max_response_bytes)


def _embedding_provider_from_config(config):
    return OllamaEmbeddingProvider(config.ollama_url, default_model=config.ollama_embedding_model, num_threads=config.ollama_embedding_threads, timeout=config.timeout, max_response_bytes=config.max_response_bytes)


def create_server(host="127.0.0.1", port=8081, token=None, provider=None, embedding_provider=None, max_request_bytes=1_048_576):
    if token is None:
        config = load()
        token = config.token
        provider = provider or _provider_from_config(config)
        embedding_provider = embedding_provider or _embedding_provider_from_config(config)
        max_request_bytes = config.max_request_bytes
    if provider is None:
        raise ValueError("缺少模型 provider")

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/healthz":
                self._send(200, {"status": "ok"})
                return
            if self.path == "/readyz":
                try:
                    readiness = provider.readiness()
                except Exception:
                    readiness = {"status": "unavailable", "provider": "unknown", "error": "provider_check_failed"}
                if embedding_provider is not None and hasattr(embedding_provider, "readiness"):
                    embedding = embedding_provider.readiness()
                    readiness["embedding"] = embedding
                    if embedding.get("status") != "ready":
                        readiness["status"] = "unavailable"
                        readiness["error"] = embedding.get("error", "embedding_unavailable")
                self._send(200 if readiness.get("status") == "ready" else 503, readiness)
                return
            self._send(404, {"error": "not_found"})

        def do_POST(self):
            if self.path not in {"/internal/v1/generate", "/internal/v1/embed"}:
                self._send(404, {"error": "not_found"})
                return
            supplied = self.headers.get("Authorization", "")
            if not supplied.startswith("Bearer ") or not secrets.compare_digest(supplied[7:], token):
                self._send(401, {"error": "unauthorized"})
                return
            try:
                length = int(self.headers.get("Content-Length", "-1"))
            except ValueError:
                length = -1
            if length < 0 or length > max_request_bytes:
                self._send(413, {"error": "request_too_large"})
                return
            try:
                payload = json.loads(self.rfile.read(length))
                if not isinstance(payload, dict):
                    raise ValueError
                if self.path == "/internal/v1/embed":
                    inputs = payload.get("inputs")
                    if embedding_provider is None or not isinstance(inputs, list) or not 1 <= len(inputs) <= 64 or any(not isinstance(item, str) or not item.strip() or len(item) > 8192 for item in inputs):
                        self._send(400 if embedding_provider is not None else 503, {"error": "invalid_embedding_request" if embedding_provider is not None else "embedding_provider_disabled"})
                        return
                    response = embedding_provider.embed(EmbeddingRequest(tenant_id=str(payload.get("tenant_id", "")), model=str(payload.get("model", "default")), inputs=inputs))
                    output = {"status": response.status, "model": response.model, "dimensions": response.dimensions, "embeddings": response.embeddings, "error": response.error}
                    self._send(200 if response.status != "error" else 502, output)
                    return
                response = provider.generate(ModelRequest(**{key: payload[key] for key in ModelRequest.__dataclass_fields__ if key in payload}))
            except (ValueError, TypeError, json.JSONDecodeError):
                self._send(400, {"error": "invalid_request"})
                return
            output = {"run_id": response.run_id, "status": response.status, "provider": response.provider, "model": response.model, "result": response.result, "error": response.error}
            self._send(200 if response.status != "error" else 502, output)

        def _send(self, status, value):
            data = json.dumps(value, ensure_ascii=False).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *_):
            return

    return ThreadingHTTPServer((host, port), Handler)
