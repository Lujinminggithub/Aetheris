# Aetheris 服务端运维

## 健康检查

- Go Server：`GET /healthz`。
- Model Gateway：使用内部服务端口检查进程存活，不对公网暴露。
- Model provider 就绪：`GET http://127.0.0.1:18081/readyz`，要求 Ollama 可达且默认模型已安装。
- Ollama：`GET http://127.0.0.1:11434/api/tags`，并使用 `systemctl is-active ollama`检查进程。
- PostgreSQL：使用 `pg_isready` 和应用连接池 ping。

## 日志与审计

应用日志只记录 request ID、事件 ID、数量、状态、耗时和错误码。不得记录 token、密码、事件 payload、原始 prompt 或模型 response。涉及设备注册、角色分配、事件 tombstone、模型调用的写操作必须进入 `audit_logs`。

## 备份与恢复

使用 `deploy/backup-postgres.sh` 生成 PostgreSQL custom-format 备份，默认保留 14 天。恢复前先停止写入并确认目标数据库，恢复后执行 migration 版本检查和 `/healthz` 验证。

## 角色维护

Admin 访问角色只决定谁能查看/管理数据；`研发`、`测试`、`产品`等工作角色只用于终端主体的事件归类。角色变更不修改历史事件中的角色快照。

## 重置管理员密码

管理员密码与 Linux root 密码相互独立。使用专用命令重置，密码只通过进程环境变量传入，至少 12 个字符；命令执行后会撤销该用户的全部旧会话并写入审计日志。

```bash
set -a
. /opt/aetheris/config/server.env
set +a
ADMIN_RESET_USERNAME=admin ADMIN_RESET_PASSWORD='新的强密码' \
  /opt/aetheris/bin/aetheris-admin reset-password
```

不要把真实密码写入 shell 脚本、文档或命令历史。

## 过程知识运维

过程知识使用独立 PostgreSQL 表和 Qdrant collection `aetheris_process_knowledge_v1`。首次部署先执行
`020_process_knowledge.sql`，再按项目执行 dry-run 和 apply 回填。运行模式按
`shadow -> canary -> active` 切换，任一阶段可以回滚到上一知识版本，原始事件不受影响。

常用命令：

```bash
/opt/aetheris/bin/aetheris-admin process-knowledge-backfill --mode dry_run --project <logical-project-id> --version 1
/opt/aetheris/bin/aetheris-admin process-knowledge-backfill --mode apply --project <logical-project-id> --version 1
```

监控 `process_knowledge_jobs` 的 scanned/candidate/verified/conflict/failed 数量，以及
`process_knowledge_chunks` 的 pending/failed 状态。模型或 Qdrant 故障不得阻塞事件接收。
