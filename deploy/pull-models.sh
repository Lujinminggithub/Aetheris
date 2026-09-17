#!/usr/bin/env bash
set -euo pipefail

ollama_host="${OLLAMA_HOST:-127.0.0.1:11434}"
generation_model="${OLLAMA_GENERATION_MODEL:-qwen3:1.7b}"
embedding_model="${OLLAMA_EMBEDDING_MODEL:-embeddinggemma}"
ready_attempts="${OLLAMA_READY_ATTEMPTS:-90}"
ready_interval="${OLLAMA_READY_INTERVAL:-2}"

if [[ "$ollama_host" == http://* || "$ollama_host" == https://* ]]; then
  api_url="${ollama_host%/}"
else
  api_url="http://${ollama_host%/}"
fi

for ((attempt = 1; attempt <= ready_attempts; attempt++)); do
  if curl --fail --silent --show-error "$api_url/api/tags" >/dev/null 2>&1; then
    break
  fi
  if (( attempt == ready_attempts )); then
    echo "Ollama 在等待窗口内未就绪" >&2
    exit 1
  fi
  sleep "$ready_interval"
done

model_installed() {
  local model="$1"
  local latest="${model}:latest"
  ollama list | awk 'NR > 1 {print $1}' | grep -Fxq -e "$model" -e "$latest"
}

declare -A seen=()
for model in "$generation_model" "$embedding_model"; do
  [[ -n "$model" ]] || continue
  [[ -z "${seen[$model]:-}" ]] || continue
  seen[$model]=1
  if model_installed "$model"; then
    echo "模型已存在: $model"
    continue
  fi
  echo "开始下载模型: $model"
  ollama pull "$model"
  echo "模型下载完成: $model"
done

if [[ "${OLLAMA_PREWARM_GENERATION:-1}" == "1" ]]; then
  if [[ ! "$generation_model" =~ ^[A-Za-z0-9._:/-]+$ ]]; then
    echo "生成模型名称包含不支持的字符，无法预热" >&2
    exit 1
  fi
  echo "开始预热生成模型: $generation_model"
  curl --fail --silent --show-error \
    --header "Content-Type: application/json" \
    --data-binary "{\"model\":\"${generation_model}\",\"keep_alive\":\"30m\"}" \
    "$api_url/api/generate" >/dev/null
  echo "生成模型预热完成: $generation_model"
fi
