from dataclasses import dataclass, field
from typing import Any, Protocol


@dataclass
class ModelRequest:
    tenant_id: str = ""
    actor_id: str = ""
    task: str = ""
    model: str = "default"
    input_event_ids: list[str] = field(default_factory=list)
    context: dict[str, Any] = field(default_factory=dict)
    messages: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class ModelResponse:
    run_id: str
    status: str
    provider: str
    model: str
    result: Any = None
    error: str | None = None


class ModelProvider(Protocol):
    def generate(self, request: ModelRequest) -> ModelResponse:
        ...


@dataclass
class EmbeddingRequest:
    tenant_id: str = ""
    model: str = "default"
    inputs: list[str] = field(default_factory=list)


@dataclass
class EmbeddingResponse:
    status: str
    model: str
    dimensions: int = 0
    embeddings: list[list[float]] = field(default_factory=list)
    error: str | None = None


class EmbeddingProvider(Protocol):
    def embed(self, request: EmbeddingRequest) -> EmbeddingResponse:
        ...
