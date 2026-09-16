# Aetheris Model Gateway

这是独立的 Python 模型适配服务。Go Server 负责鉴权和事件筛选，Model Gateway 不连接 PostgreSQL，只处理最小必要的模型请求。

默认 provider 为 Ollama；Dify 通过环境变量启用。启动前设置 `MODEL_GATEWAY_TOKEN`，服务只监听本机地址，除非部署层明确配置内网监听。
逻辑模型名 `default` 由 `OLLAMA_MODEL` 映射到实际模型，默认为 `qwen3:4b-instruct`。`/healthz` 只检查网关进程，`/readyz` 同时检查 Ollama 连通性和默认模型是否已安装。

```powershell
$env:MODEL_GATEWAY_TOKEN = "local-model-token"
$env:PYTHONPATH = "model-gateway"
python -m aetheris_model_gateway
```

内部 endpoint：`POST /internal/v1/generate`，使用 `Authorization: Bearer <MODEL_GATEWAY_TOKEN>`。错误只返回 provider、状态和错误码，不返回凭据或原始 prompt。
