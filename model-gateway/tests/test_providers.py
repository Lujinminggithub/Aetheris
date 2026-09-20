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
        self.assertEqual(sent["options"]["num_predict"], 96)
        self.assertFalse(sent["think"])
        self.assertEqual(sent["keep_alive"], "30m")
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

    def test_ollama_uses_dynamic_budget_for_planning_and_analysis(self):
        body = json.dumps({"message": {"content": "{}"}}).encode()
        cases = [
            ("plan_rag_answer", {}, 128),
            ("rag_answer", {"answer_mode": "analysis"}, 192),
            ("rag_answer", {"answer_mode": "reason"}, 160),
            ("rag_answer", {"answer_mode": "direct"}, 96),
        ]
        for task, context, expected in cases:
            with self.subTest(task=task, context=context):
                model_request = ModelRequest(task=task, model="default", context=context)
                with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
                    OllamaProvider("http://ollama:11434").generate(model_request)
                sent = json.loads(open_url.call_args.args[0].data)
                self.assertEqual(sent["options"]["num_predict"], expected)
                self.assertEqual(sent["options"]["num_ctx"], 4096)
                self.assertEqual(sent["options"]["num_thread"], 6)
                schema = sent["format"]
                self.assertEqual(schema["type"], "object")
                self.assertFalse(schema["additionalProperties"])
                if task == "rag_answer":
                    self.assertEqual(
                        set(schema["required"]),
                        {"answer", "answer_mode", "confidence", "details", "citation_numbers"},
                    )
                    self.assertEqual(schema["properties"]["confidence"]["enum"], ["medium", "low"])
                    if context.get("answer_mode") == "analysis":
                        self.assertEqual(schema["properties"]["details"]["minLength"], 100)
                    else:
                        self.assertEqual(schema["properties"]["details"]["maxLength"], 0)

    def test_ollama_allows_high_confidence_only_with_verified_evidence(self):
        body = json.dumps({"message": {"content": "{}"}}).encode()
        model_request = ModelRequest(
            task="rag_answer",
            model="default",
            context={"answer_mode": "analysis", "evidence": [{"validation_state": "verified"}]},
        )
        with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
            OllamaProvider("http://ollama:11434").generate(model_request)
        sent = json.loads(open_url.call_args.args[0].data)
        self.assertEqual(sent["format"]["properties"]["confidence"]["enum"], ["high", "medium", "low"])

    def test_ollama_allows_high_confidence_with_platform_certified_evidence(self):
        body = json.dumps({"message": {"content": "{}"}}).encode()
        model_request = ModelRequest(task="rag_answer", model="default", context={"answer_mode": "analysis", "evidence": [{"validation_state": "platform_certified"}]})
        with patch("urllib.request.urlopen", return_value=_Response(body)) as open_url:
            OllamaProvider("http://ollama:11434").generate(model_request)
        sent = json.loads(open_url.call_args.args[0].data)
        self.assertEqual(sent["format"]["properties"]["confidence"]["enum"], ["high", "medium", "low"])

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
