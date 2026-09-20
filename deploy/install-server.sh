#!/usr/bin/env bash
set -euo pipefail

release_dir="${AETHERIS_RELEASE_DIR:?必须设置 AETHERIS_RELEASE_DIR}"
config_file="${AETHERIS_CONFIG_FILE:?必须设置 AETHERIS_CONFIG_FILE}"
install_root="/opt/aetheris"
prepared_config=""
cleanup() {
  if [[ -n "$prepared_config" && -f "$prepared_config" ]]; then
    rm -f -- "$prepared_config"
  fi
}
trap cleanup EXIT

if [[ ! -d "$release_dir" ]]; then
  echo "发布目录不存在: $release_dir" >&2
  exit 1
fi
if [[ ! -f "$config_file" ]]; then
  echo "配置文件不存在: $config_file" >&2
  exit 1
fi

install -d -m 0750 "$install_root/bin" "$install_root/config" "$install_root/web/admin" "$install_root/backups" "$install_root/logs" "$install_root/scripts"
install -m 0750 "$release_dir/aetheris-server" "$install_root/bin/aetheris-server"
install -m 0750 "$release_dir/aetheris-migrate" "$install_root/bin/aetheris-migrate"
model_script="$release_dir/deploy/pull-models.sh"
if [[ ! -f "$model_script" ]]; then
  model_script="$release_dir/pull-models.sh"
fi
if [[ ! -f "$model_script" ]]; then
  echo "模型下载脚本不存在: deploy/pull-models.sh" >&2
  exit 1
fi
install -m 0750 "$model_script" "$install_root/bin/pull-models.sh"
for helper in backfill-public-knowledge.sh backfill-public-knowledge.ps1; do
  helper_path="$release_dir/scripts/$helper"
  if [[ -f "$helper_path" ]]; then
    install -m 0750 "$helper_path" "$install_root/scripts/$helper"
  fi
done
if [[ -f "$release_dir/aetheris-admin" ]]; then
  install -m 0750 "$release_dir/aetheris-admin" "$install_root/bin/aetheris-admin"
fi
if ! grep -q '^AETHERIS_PROJECT_IDENTITY_KEY=' "$config_file"; then
  if ! command -v openssl >/dev/null 2>&1; then
    echo "配置缺少 AETHERIS_PROJECT_IDENTITY_KEY，且系统没有 openssl" >&2
    exit 1
  fi
  prepared_config="$(mktemp)"
  umask 077
  cat "$config_file" > "$prepared_config"
  printf 'AETHERIS_PROJECT_IDENTITY_KEY=%s\n' "$(openssl rand -base64 32 | tr -d '\n')" >> "$prepared_config"
  printf 'AETHERIS_PROJECT_IDENTITY_KEY_VERSION=1\n' >> "$prepared_config"
  install -m 0600 "$prepared_config" "$install_root/config/server.env"
else
  install -m 0600 "$config_file" "$install_root/config/server.env"
fi
if [[ -d "$release_dir/admin-web/dist" ]]; then
  cp -R "$release_dir/admin-web/dist/." "$install_root/web/admin/"
fi

echo "部署文件已安装到 $install_root；ollama.service 启动后将自动准备默认生成与向量模型。"
