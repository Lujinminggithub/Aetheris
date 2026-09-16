import json
import os
import unittest
from unittest.mock import patch
from urllib.error import HTTPError

from aetheris_model_gateway.protocols import ModelRequest
from aetheris_model_gateway.providers.dify import DifyProvider
from aetheris_model_gateway.providers.ollama import OllamaProvider


class _Response:
    def __init__(self, body, status=200):
        self.body = body
        self.status = status

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False

    def read(self, *_):
        return self.body

    def getcode(self):
        return self.status


def request():
    return ModelRequest(
        tenant_id="tenant-1", actor_id="user-1", task="summarize",
        model="llama3", input_event_ids=["event-1"],
        context={"work_role": "研发"}, messages=[{"role": "user", "content": "你好"}],
    )


class ProviderTests(unittest.TestCase):
    def test_ollama_success_maps_chat_response(self):
        body = json.dumps({"message": {"content": "完成"}}).encode()
        with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
            response = OllamaProvider("http://ollama:11434").generate(request())
        self.assertEqual(response.result, "完成")
        self.assertEqual(response.provider, "ollama")
        self.assertEqual(open_url.call_args.kwargs["timeout"], 30.0)

    def test_ollama_maps_default_model_and_sends_structured_context(self):
        body = json.dumps({"message": {"content": "完成"}}).encode()
        model_request = ModelRequest(
            tenant_id="tenant-1", actor_id="user-1", task="summarize_personal_effectiveness",
            model="default", context={"effectiveness": {"totals": {"active_days": 3}}},
            messages=[{"role": "system", "content": "只描述可解释指标"}],
        )
        with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
            response = OllamaProvider("http://ollama:11434", default_model="qwen3:4b-instruct").generate(model_request)

        sent = json.loads(open_url.call_args.args[0].data)
        self.assertEqual(sent["model"], "qwen3:4b-instruct")
        self.assertEqual(sent["messages"][-1]["role"], "user")
        self.assertIn('"active_days": 3', sent["messages"][-1]["content"])
        self.assertEqual(sent["options"]["num_predict"], 256)
        self.assertEqual(response.model, "qwen3:4b-instruct")

    def test_ollama_readiness_requires_configured_model(self):
        body = json.dumps({"models": [{"name": "qwen3:4b-instruct"}]}).encode()
        with patch("urllib.request.urlopen", return_value=_Response(body)):
            ready = OllamaProvider("http://ollama:11434", default_model="qwen3:4b-instruct").readiness()
        with patch("urllib.request.urlopen", return_value=_Response(json.dumps({"models": []}).encode())):
            missing = OllamaProvider("http://ollama:11434", default_model="qwen3:4b-instruct").readiness()

        self.assertEqual(ready["status"], "ready")
        self.assertEqual(missing["status"], "unavailable")
        self.assertEqual(missing["error"], "model_not_found")

    def test_provider_retries_connection_error_once_then_succeeds(self):
        body = json.dumps({"message": {"content": "完成"}}).encode()
        with patch("urllib.request.urlopen", side_effect=[TimeoutError(), _Response(body)]) as open_url:
            response = OllamaProvider("http://ollama:11434", retries=1).generate(request())
        self.assertEqual(response.result, "完成")
        self.assertEqual(open_url.call_count, 2)

    def test_dify_requires_api_key(self):
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError):
                DifyProvider("https://dify.example", app_id="app")

    def test_dify_success_uses_key_and_returns_answer(self):
        body = json.dumps({"answer": "摘要"}).encode()
        with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
            response = DifyProvider("https://dify.example", api_key="secret", app_id="app").generate(request())
        self.assertEqual(response.result, "摘要")
        sent = open_url.call_args.args[0]
        self.assertIn("secret", sent.headers.values())

    def test_non_2xx_is_reported(self):
        error = HTTPError("http://ollama", 503, "unavailable", {}, None)
        with patch("urllib.request.urlopen", side_effect=error):
            response = OllamaProvider("http://ollama", retries=0).generate(request())
        self.assertEqual(response.status, "error")
        self.assertEqual(response.error, "http_503")

    def test_invalid_json_is_reported(self):
        with patch("urllib.request.urlopen", return_value=_Response(b"not-json")):
            response = OllamaProvider("http://ollama", retries=0).generate(request())
        self.assertEqual(response.status, "error")
        self.assertEqual(response.error, "invalid_json")

    def test_response_limit_is_reported(self):
        with patch("urllib.request.urlopen", return_value=_Response(b"123456")):
            response = OllamaProvider("http://ollama", max_response_bytes=5).generate(request())
        self.assertEqual(response.status, "error")
        self.assertEqual(response.error, "response_too_large")


if __name__ == "__main__":
    unittest.main()
