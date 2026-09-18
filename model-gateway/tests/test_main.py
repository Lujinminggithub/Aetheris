import unittest
from unittest.mock import patch

from aetheris_model_gateway.config import GatewayConfig


class MainAssemblyTests(unittest.TestCase):
    def test_build_server_wires_embedding_provider(self):
        from aetheris_model_gateway.__main__ import build_server

        config = GatewayConfig(token="token", port=0, ollama_embedding_model="bge-m3", ollama_embedding_threads=4)
        with patch("aetheris_model_gateway.__main__.create_server") as create:
            build_server(config)

        arguments = create.call_args.kwargs
        self.assertIsNotNone(arguments["embedding_provider"])
        self.assertEqual(arguments["embedding_provider"].default_model, "bge-m3")
        self.assertEqual(arguments["embedding_provider"].num_threads, 4)


if __name__ == "__main__":
    unittest.main()
