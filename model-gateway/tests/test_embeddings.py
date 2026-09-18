import json
import math
import unittest

from aetheris_model_gateway.protocols import EmbeddingRequest


class _Response:
    def __init__(self, payload, status=200): self.payload, self.status = payload, status
    def __enter__(self): return self
    def __exit__(self, *_): return False
    def getcode(self): return self.status
    def read(self, _): return json.dumps(self.payload).encode()


class EmbeddingProviderTests(unittest.TestCase):
    def test_ollama_embedding_provider_returns_uniform_vectors(self):
        from aetheris_model_gateway.providers.ollama_embeddings import OllamaEmbeddingProvider

        provider = OllamaEmbeddingProvider("http://ollama:11434", default_model="bge-m3", opener=lambda request, timeout: _Response({"model": "bge-m3", "embeddings": [[0.1, 0.2], [0.3, 0.4]]}))
        result = provider.embed(EmbeddingRequest(tenant_id="tenant-1", inputs=["一", "二"]))

        self.assertEqual(result.model, "bge-m3")
        self.assertEqual(result.dimensions, 2)
        self.assertEqual(result.embeddings, [[0.1, 0.2], [0.3, 0.4]])

    def test_ollama_embedding_provider_limits_background_threads(self):
        from aetheris_model_gateway.providers.ollama_embeddings import OllamaEmbeddingProvider

        captured = {}
        def opener(request, timeout):
            captured.update(json.loads(request.data))
            return _Response({"model": "embeddinggemma", "embeddings": [[0.1, 0.2]]})

        result = OllamaEmbeddingProvider("http://ollama:11434", num_threads=6, opener=opener).embed(EmbeddingRequest(inputs=["一"]))

        self.assertEqual(result.status, "succeeded")
        self.assertEqual(captured["options"]["num_thread"], 6)

    def test_ollama_embedding_provider_rejects_malformed_vectors(self):
        from aetheris_model_gateway.providers.ollama_embeddings import OllamaEmbeddingProvider

        provider = OllamaEmbeddingProvider("http://ollama:11434", opener=lambda request, timeout: _Response({"embeddings": [[0.1], [math.nan]]}), retries=0)
        result = provider.embed(EmbeddingRequest(inputs=["一", "二"]))

        self.assertEqual(result.status, "error")
        self.assertEqual(result.error, "invalid_embeddings")
