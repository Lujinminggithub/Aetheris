import os
import unittest
from unittest.mock import patch

from aetheris_model_gateway.config import load


class ConfigTests(unittest.TestCase):
    def test_default_timeout_allows_cpu_model_inference(self):
        with patch.dict(os.environ, {"MODEL_GATEWAY_TOKEN": "test-token"}, clear=True):
            config = load()

        self.assertEqual(config.timeout, 540.0)
        self.assertEqual(config.ollama_embedding_model, "embeddinggemma")
        self.assertEqual(config.ollama_embedding_threads, 6)


if __name__ == "__main__":
    unittest.main()
