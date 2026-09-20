#!/usr/bin/env bash
set -euo pipefail

server_url="${AETHERIS_SERVER_URL:-http://127.0.0.1:8080}"
admin_user="${AETHERIS_ADMIN_USERNAME:-admin}"
admin_password="${AETHERIS_ADMIN_PASSWORD:?必须设置 AETHERIS_ADMIN_PASSWORD}"
mode="${1:-build_candidates}"

case "$mode" in
  build_candidates|reindex|rebuild) ;;
  *) echo "模式必须是 build_candidates、reindex 或 rebuild" >&2; exit 2 ;;
esac

cookie_file="$(mktemp)"
cleanup() { rm -f -- "$cookie_file"; }
trap cleanup EXIT

login_body="$(ADMIN_USER="$admin_user" ADMIN_PASSWORD="$admin_password" python3 -c 'import json,os; print(json.dumps({"username":os.environ["ADMIN_USER"],"password":os.environ["ADMIN_PASSWORD"]}))')"
curl -fsS -c "$cookie_file" -H 'Content-Type: application/json' -d "$login_body" "$server_url/api/v1/auth/login" >/dev/null
csrf_token="$(awk '$6=="aetheris_csrf" {print $7}' "$cookie_file" | tail -n 1)"
if [[ -z "$csrf_token" ]]; then
  echo "登录成功但未获得 aetheris_csrf Cookie" >&2
  exit 1
fi

job_body="$(MODE="$mode" python3 -c 'import json,os; print(json.dumps({"mode":os.environ["MODE"]}))')"
job_json="$(curl -fsS -b "$cookie_file" -H 'Content-Type: application/json' -H "X-CSRF-Token: $csrf_token" -d "$job_body" "$server_url/api/v1/admin/public-knowledge/jobs")"
job_id="$(JOB_JSON="$job_json" python3 -c 'import json,os; print(json.loads(os.environ["JOB_JSON"])["id"])')"
echo "公共知识任务已创建: $job_id"

while true; do
  job_json="$(curl -fsS -b "$cookie_file" "$server_url/api/v1/admin/public-knowledge/jobs/$job_id")"
  JOB_JSON="$job_json" python3 -c 'import json,os; j=json.loads(os.environ["JOB_JSON"]); print("state={state} scanned={scanned_count} candidates={candidate_count} conflicts={conflict_count} failed={failed_count}".format(**j))'
  state="$(JOB_JSON="$job_json" python3 -c 'import json,os; print(json.loads(os.environ["JOB_JSON"])["state"])')"
  case "$state" in
    completed) exit 0 ;;
    failed) exit 1 ;;
  esac
  sleep 3
done
