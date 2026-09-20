# 平台共享知识发布与回滚

## 1. 配置

`/opt/aetheris/config/server.env` 增加：

```ini
PROCESS_KNOWLEDGE_COLLECTION=aetheris_process_knowledge_v1
PUBLIC_KNOWLEDGE_COLLECTION=aetheris_public_knowledge_v1
PUBLIC_KNOWLEDGE_INTERVAL=300
PUBLIC_KNOWLEDGE_BATCH=25
PUBLIC_KNOWLEDGE_QUERY_MODE=shadow
```

`shadow` 会执行私有和公共检索，但只向用户返回当前租户私有知识。后台任务批量固定为 25，间隔 300 秒，用户查询通过共享 WorkloadGate 获得优先权。

## 2. 部署与迁移

```bash
sudo systemctl stop aetheris-server
sudo /opt/aetheris/bin/aetheris-migrate
sudo systemctl start aetheris-server
curl -fsS http://127.0.0.1:8080/healthz
```

确认 migration 21 已执行，服务日志不存在 migration、Qdrant collection 或 embedding 错误。

## 3. 生成公共候选

Linux：

```bash
export AETHERIS_SERVER_URL=http://127.0.0.1:8080
export AETHERIS_ADMIN_USERNAME=admin
export AETHERIS_ADMIN_PASSWORD='管理员密码'
/opt/aetheris/scripts/backfill-public-knowledge.sh build_candidates
```

Windows：

```powershell
.\scripts\backfill-public-knowledge.ps1 -ServerUrl http://192.168.78.138:8080 -Username admin -Password '管理员密码' -Mode build_candidates
```

候选生成不会自动公开知识。任务完成后在 Admin Web 的“知识治理”中依次执行来源确认、证据验证和平台认证。

## 4. 影子验证

保持 `PUBLIC_KNOWLEDGE_QUERY_MODE=shadow`，完成以下检查：

1. 公共 Qdrant collection 中只存在 `public_knowledge_id`、revision、主题、知识类型和验证状态。
2. 公共 payload 不包含 tenant、project、device、session 或 event 字段。
3. 智能查询结果与私有知识基线一致。
4. 日志中没有公共检索超时、脱敏隔离失败或跨租户来源字段。

## 5. 正式启用

```bash
sudo sed -i 's/^PUBLIC_KNOWLEDGE_QUERY_MODE=.*/PUBLIC_KNOWLEDGE_QUERY_MODE=active/' /opt/aetheris/config/server.env
sudo systemctl restart aetheris-server
```

在不同租户分别验证：当前租户私有已验证结果优先、公共知识显示匿名来源统计、公共引用不能展开其他租户证据。

## 6. 回滚

公共检索异常时立即关闭，不删除 PostgreSQL 或 Qdrant 数据：

```bash
sudo sed -i 's/^PUBLIC_KNOWLEDGE_QUERY_MODE=.*/PUBLIC_KNOWLEDGE_QUERY_MODE=off/' /opt/aetheris/config/server.env
sudo systemctl restart aetheris-server
```

`off` 只使用当前租户私有过程知识。修复后先切回 `shadow`，重新执行 `reindex` 任务并完成验收，再切换 `active`。

单条知识错误时应在“知识治理”中暂停或撤回。数据库会先令该知识对新查询不可见，后台再删除公共向量。
