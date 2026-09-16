import json
import threading
import unittest
from http.client import HTTPConnection

from aetheris_model_gateway.http import create_server
from aetheris_model_gateway.protocols import ModelResponse


class _Provider:
    def generate(self, request):
        return ModelResponse(run_id="run-1", status="succeeded", provider="test", model=request.model, result={"text": "ok"})

    def readiness(self):
        return {"status": "ready", "provider": "test", "model": "test-model"}


class _UnavailableProvider(_Provider):
    def readiness(self):
        return {"status": "unavailable", "provider": "ollama", "model": "qwen3:4b-instruct", "error": "connection_error"}


class _EmbeddingProvider:
    def embed(self, request):
        from aetheris_model_gateway.protocols import EmbeddingResponse
        return EmbeddingResponse(status="succeeded", model=request.model or "bge-m3", dimensions=2, embeddings=[[0.1, 0.2] for _ in request.inputs])


class HttpTests(unittest.TestCase):
    def setUp(self):
        self.server = create_server("127.0.0.1", 0, token="internal-token", provider=_Provider(), embedding_provider=_EmbeddingProvider())
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def post(self, path, body, token="internal-token"):
        connection = HTTPConnection(*self.server.server_address, timeout=2)
        data = json.dumps(body).encode()
        connection.request("POST", path, data, {"Content-Type": "application/json", "Authorization": "Bearer " + token})
        response = connection.getresponse()
        payload = json.loads(response.read())
        connection.close()
        return response.status, payload

    def get(self, path):
        connection = HTTPConnection(*self.server.server_address, timeout=2)
        connection.request("GET", path)
        response = connection.getresponse()
        payload = json.loads(response.read())
        connection.close()
        return response.status, payload

    def test_generate_requires_internal_token(self):
        status, payload = self.post("/internal/v1/generate", {"model": "m"}, token="wrong")
        self.assertEqual(status, 401)
        self.assertEqual(payload["error"], "unauthorized")

    def test_generate_returns_normalized_response(self):
        status, payload = self.post("/internal/v1/generate", {"tenant_id": "t", "actor_id": "a", "task": "x", "model": "m", "messages": []})
        self.assertEqual(status, 200)
        self.assertEqual(payload["run_id"], "run-1")
        self.assertEqual(payload["status"], "succeeded")

    def test_generate_rejects_invalid_json(self):
        connection = HTTPConnection(*self.server.server_address, timeout=2)
        connection.request("POST", "/internal/v1/generate", b"bad", {"Content-Type": "application/json", "Authorization": "Bearer internal-token"})
        response = connection.getresponse()
        self.assertEqual(response.status, 400)
        connection.close()

    def test_embed_requires_internal_token_and_enforces_batch_limit(self):
        status, _ = self.post("/internal/v1/embed", {"tenant_id": "t", "inputs": ["text"]}, token="wrong")
        self.assertEqual(status, 401)
        status, payload = self.post("/internal/v1/embed", {"tenant_id": "t", "inputs": ["text"] * 65})
        self.assertEqual(status, 400)
        self.assertEqual(payload["error"], "invalid_embedding_request")

    def test_embed_returns_dimensions_and_vectors(self):
        status, payload = self.post("/internal/v1/embed", {"tenant_id": "t", "model": "bge-m3", "inputs": ["一", "二"]})
        self.assertEqual(status, 200)
        self.assertEqual(payload["dimensions"], 2)
        self.assertEqual(len(payload["embeddings"]), 2)

    def test_readyz_reports_provider_failure(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)
        self.server = create_server("127.0.0.1", 0, token="internal-token", provider=_UnavailableProvider())
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

        status, payload = self.get("/readyz")

        self.assertEqual(status, 503)
        self.assertEqual(payload["error"], "connection_error")


if __name__ == "__main__":
    unittest.main()
