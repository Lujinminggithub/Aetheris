import json
import socket
import uuid
from typing import Callable
from urllib.error import HTTPError, URLError
from urllib.request import Request
import urllib.request

from ..protocols import ModelRequest, ModelResponse


class HTTPProvider:
    provider_name = ""

    def __init__(self, timeout=30.0, retries=2, max_response_bytes=4_194_304, opener: Callable | None = None):
        self.timeout = timeout
        self.retries = max(0, retries)
        self.max_response_bytes = max_response_bytes
        self.opener = opener or urllib.request.urlopen

    def _request(self, url, payload, headers, request):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        last_error = None
        for attempt in range(self.retries + 1):
            try:
                response_request = Request(url, data=body, headers=headers, method="POST")
                with self.opener(response_request, timeout=self.timeout) as response:
                    status = response.getcode()
                    data = response.read(self.max_response_bytes + 1)
                    if len(data) > self.max_response_bytes:
                        return None, "response_too_large", False
                    if status < 200 or status >= 300:
                        if status >= 500 and attempt < self.retries:
                            continue
                        return None, f"http_{status}", False
                    try:
                        return json.loads(data.decode("utf-8")), None, False
                    except (UnicodeDecodeError, json.JSONDecodeError):
                        return None, "invalid_json", False
            except HTTPError as exc:
                last_error = f"http_{exc.code}"
                retryable = exc.code >= 500
            except (TimeoutError, socket.timeout, URLError, OSError) as exc:
                last_error = "connection_error"
                retryable = True
            if not retryable or attempt >= self.retries:
                return None, last_error or "request_failed", False
        return None, last_error or "request_failed", False

    def _error(self, request: ModelRequest, code: str) -> ModelResponse:
        return ModelResponse(str(uuid.uuid4()), "error", self.provider_name, request.model, error=code)
