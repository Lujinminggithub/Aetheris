#!/usr/bin/env bash
set -euo pipefail

backup_dir="${AETHERIS_BACKUP_DIR:-/opt/aetheris/backups}"
database_url="${DATABASE_URL:?必须设置 DATABASE_URL}"
retention_days="${AETHERIS_BACKUP_RETENTION_DAYS:-14}"
mkdir -p "$backup_dir"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$backup_dir/aetheris-$timestamp.dump"
pg_dump --format=custom --file="$target" "$database_url"
find "$backup_dir" -type f -name 'aetheris-*.dump' -mtime "+$retention_days" -delete
chmod 0600 "$target"
echo "数据库备份完成: $target"

