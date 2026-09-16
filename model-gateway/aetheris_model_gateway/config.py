import os
from dataclasses import dataclass


@dataclass(frozen=True)
class GatewayConfig:
    token: str
    host: str = "127.0.0.1"
    port: int = 8081
    provider: str = "ollama"
    ollama_url: str = "http://127.0.0.1:11434"
    ollama_model: str = "qwen3:4b-instruct"
    ollama_embedding_model: str = "embeddinggemma"
    dify_api_url: str = ""
    dify_api_key: str = ""
    dify_app_id: str = ""
    dify_workflow_id: str = ""
    timeout: float = 120.0
    max_request_bytes: int = 1_048_576
    max_response_bytes: int = 4_194_304


def load() -> GatewayConfig:
    token = os.environ.get("MODEL_GATEWAY_TOKEN", "")
    if not token:
        raise ValueError("缺少 MODEL_GATEWAY_TOKEN")
    return GatewayConfig(
        token=token,
        host=os.environ.get("MODEL_GATEWAY_HOST", "127.0.0.1"),
        port=int(os.environ.get("MODEL_GATEWAY_PORT", "8081")),
        provider=os.environ.get("MODEL_PROVIDER", "ollama"),
        ollama_url=os.environ.get("OLLAMA_URL", "http://127.0.0.1:11434"),
        ollama_model=os.environ.get("OLLAMA_MODEL", "qwen3:4b-instruct"),
        ollama_embedding_model=os.environ.get("OLLAMA_EMBEDDING_MODEL", "embeddinggemma"),
        dify_api_url=os.environ.get("DIFY_API_URL", ""),
        dify_api_key=os.environ.get("DIFY_API_KEY", ""),
        dify_app_id=os.environ.get("DIFY_APP_ID", ""),
        dify_workflow_id=os.environ.get("DIFY_WORKFLOW_ID", ""),
        timeout=float(os.environ.get("MODEL_GATEWAY_TIMEOUT", "120")),
        max_request_bytes=int(os.environ.get("MODEL_GATEWAY_MAX_REQUEST_BYTES", "1048576")),
        max_response_bytes=int(os.environ.get("MODEL_GATEWAY_MAX_RESPONSE_BYTES", "4194304")),
    )
