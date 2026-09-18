# Aetheris 服务端部署

本文主要描述服务端部署；涉及服务端下发策略时会说明 Windows Core 和本地 Lens 的接口边界。

## 本地开发

1. 启动 PostgreSQL：

   ```powershell
   docker compose -f deploy/docker-compose.yml up -d postgres
   ```

2. 设置环境变量：

   ```powershell
   $env:DATABASE_URL = "postgres://aetheris:local-development-only@127.0.0.1:54329/aetheris?sslmode=disable"
   $env:DEVICE_ENROLLMENT_SECRET = "local-enrollment-secret"
   $env:ADMIN_BOOTSTRAP_PASSWORD = "仅用于本地开发的长密码"
   ```

3. 执行 migration 并启动 Go Server：

   ```powershell
   go -C server run ./cmd/aetheris-migrate
   go -C server run ./cmd/aetheris-server
   ```

   生产配置使用 `CLIENT_DOWNLOAD_FILE` 指向允许下载的单个 Windows 客户端安装包。Go Server 通过 `/downloads/client` 返回该文件，不开放目录浏览。

4. 独立启动 Admin Web：

   ```powershell
   npm --prefix admin-web install
   npm --prefix admin-web run dev
   ```

5. 独立启动 Model Gateway：

   ```powershell
   $env:MODEL_GATEWAY_TOKEN = "local-model-token"
   $env:PYTHONPATH = "model-gateway"
   python -m aetheris_model_gateway
   ```

## Linux 生产部署

生产目录为 `/opt/aetheris`。部署前通过 SSH agent 或外部 secret 机制提供凭据；不得把服务器密码写入脚本、文档、环境文件模板或日志。

部署顺序：

1. 只读检查 `df -h /opt/aetheris`、`du -sh /opt/aetheris/*`、PostgreSQL 连通性和现有 systemd 状态。
2. 创建非 root 用户 `aetheris`，准备 `/opt/aetheris` 子目录。
3. 使用 `AETHERIS_RELEASE_DIR` 和 `AETHERIS_CONFIG_FILE` 执行 `deploy/install-server.sh`。
4. 以 `aetheris` 用户执行 `/opt/aetheris/bin/aetheris-migrate`。
5. 安装并启用 `deploy/systemd/*.service`。
6. 使用 `/opt/aetheris/backups` 的 `backup-postgres.sh` 配置定时备份。

空间预算为 PostgreSQL 25 GB、对象存储 40 GB、向量索引 15 GB、模型/Dify 数据 10 GB、日志/备份/升级 10 GB。磁盘使用率达到 80% 时告警，达到 90% 时停止非必要导入。

## 已完成的远程切换

目标虚拟机已完成一次并行迁移：PostgreSQL 14、Go Server、Admin Web 和 Model Gateway 均已部署。Go Server 当前接管 `0.0.0.0:8080`，Model Gateway 仅监听 `127.0.0.1:18081`，旧 Python Gateway 已停止。

旧 SQLite 数据库事件数为 0，因此没有执行事件数据导入；原数据库、Gateway token 配置和 Admin bootstrap 文件已保存在带时间戳的备份目录中。旧 Gateway 的回滚 unit 保留在 `/etc/systemd/system/aetheris-legacy-gateway.service`，默认不启用。

切换后检查命令：

```bash
systemctl is-active postgresql aetheris-server aetheris-model-gateway
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:18081/healthz
curl -fsS http://127.0.0.1:18081/readyz
ss -ltnp | grep -E '(:8080|:18081|:5432)'
```

正式服务必须启用 systemd 开机启动：

```bash
systemctl enable postgresql.service aetheris-server.service aetheris-model-gateway.service ollama.service qdrant.service
systemctl disable --now aetheris-gateway.service aetheris-legacy-gateway.service
```

其中 `aetheris-gateway.service` 是早期 Python Gateway，已被 Go Server 替代，不得与正式服务同时启用。`aetheris-legacy-gateway.service` 仅用于人工回滚，默认必须保持禁用。重启后应同时检查 `systemctl is-enabled`、`systemctl is-active`、监听端口和各组件健康接口，不能只依据进程存在判断启动成功。

## Ollama 默认模型

生产环境使用独立 `ollama.service`，只监听 `127.0.0.1:11434`，模型文件位于 `/opt/aetheris/models/ollama`。当前默认在线模型为 `qwen3:1.7b`；`qwen3:4b-instruct` 可保留为人工选择的高质量模型，但不作为纯 CPU 虚拟机的在线默认值。服务配置为：

```text
MODEL_PROVIDER=ollama
OLLAMA_URL=http://127.0.0.1:11434
OLLAMA_MODEL=qwen3:1.7b
OLLAMA_EMBEDDING_THREADS=6
MODEL_GATEWAY_TIMEOUT=600
```

部署包必须包含 `deploy/pull-models.sh`。`install-server.sh` 会将其安装到
`/opt/aetheris/bin/pull-models.sh`，`ollama.service` 在每次启动后执行该脚本。脚本先等待
Ollama API 就绪，再检查本机模型清单；已经存在的模型不会重复下载，缺少的模型自动执行：

```bash
OLLAMA_HOST=127.0.0.1:11434 \
OLLAMA_GENERATION_MODEL=qwen3:1.7b \
OLLAMA_EMBEDDING_MODEL=embeddinggemma \
/opt/aetheris/bin/pull-models.sh
```

模型准备完成后，脚本会使用 `keep_alive=30m` 预热生成模型，避免首次智能查询承担模型冷启动开销。
在纯 CPU 虚拟机上，Go Server 到 Model Gateway 的调用超时为 600 秒；Model Gateway 到 Ollama
的超时建议为 540 秒，必须小于外层超时。智能查询仍通过异步任务返回阶段进度；查询持有前台工作锁
期间，活动索引和过程知识索引不会启动新的 embedding 批次，防止后台回填挤占生成资源。
`RETRIEVAL_INDEX_BATCH` 默认使用 8，避免单个大批次长时间占用 CPU 或在响应阶段扩大重试范围。
embedding 请求单独使用 `OLLAMA_EMBEDDING_THREADS=6`，限制后台索引的 CPU 并行度；回答模型不受该参数限制。
`PROCESS_KNOWLEDGE_INTERVAL` 默认 60 秒，避免无新增事实时频繁执行全量反关联检查。

首次安装可以通过以下命令观察下载进度：

```bash
journalctl -u ollama.service -f
```

如需更换模型，使用 systemd drop-in 覆盖 `OLLAMA_GENERATION_MODEL` 或
`OLLAMA_EMBEDDING_MODEL`，然后重启 `ollama.service`。脚本不读取或打印应用凭据。

Ollama 设置 `OLLAMA_CONTEXT_LENGTH=16384`，用于容纳结构化效能指标。Model Gateway 对单次总结限制生成 256 token，并且只发送聚合指标、覆盖率、趋势、项目/角色分布和指标口径，不发送原始事件 payload。

## 个人效能聚合

个人效能由 Go Server 后台 worker 每 5 分钟重算今天和昨天。生产配置：

```text
EFFECTIVENESS_TIMEZONE=Asia/Shanghai
EFFECTIVENESS_RECOMPUTE_INTERVAL=300
```

人工重算通过 Admin Web 或 `POST /api/v1/admin/effectiveness/recompute` 发起，单次不能超过 31 天。查询最大范围为 90 天。聚合失败不会阻塞事件 ingest。

## 事件清洗事实层

生产环境使用 `clean_event_facts` 保存可版本重算的规范事实，`events` 始终作为不可变原始证据。当前清洗规则版本为 2，个人效能定义版本为 4。

- PowerShell 仅把奇数个尾随反引号视为续行；偶数个反引号不抢占下一条参数残片。
- 孤立参数残片在同设备、同项目、相邻采集顺序和 5 秒窗口内与主命令合并，记录 `merge_method=inferred_parameter_join` 和全部 `source_event_ids`。
- AI 结构化 shell 调用保存命令类型、安全摘要和设备内 HMAC，不保存命令原文。
- 动态 automation 无法可靠还原时进入待确认并排除效能；AI tool 与 PSReadLine 匹配时只计算 AI 权威事实一次。
- Admin Web 的“数据质量”页面展示规则版本、合并原因、置信度和证据链，可点击源事件查看脱敏详情。

清洗重算接口单次支持 1 到 90 天；效能重算接口单次最多 31 天。生产回填应先完成清洗并对账，再分段提交效能重算。

## 统一活动记录与智能查询

Admin Web 不再把 AI 交互作为独立一级数据类型。“活动记录”统一展示 AI 协作、AI 工具执行、终端、IDE、浏览器、版本控制和其他规范事实；旧 AI 交互 API 仅保留兼容。

无法映射到授权项目的 AI cwd 在 Core 本地归入 `Codex`、`Claude Code`、`Cursor` 或 `GitHub Copilot` 逻辑项目。fallback 事件不上传 cwd，使用 `supersedes_event_id` 关联旧归因事件，清洗层只采用新证据。

“智能查询”入口位于 Admin Web 主导航，生产组件为：

```text
Admin Web -> Go Server :8080
Go Server -> Model Gateway 127.0.0.1:18081
Model Gateway -> Ollama 127.0.0.1:11434
Go Server -> Qdrant 127.0.0.1:6333
```

默认在线回答模型为 `qwen3:1.7b`，embedding 模型为 `embeddinggemma`，向量维度为 768。`qwen3:4b-instruct` 仅作为可选高质量模型保留。生产同批基准中 EmbeddingGemma 热态为 24.3 秒、BGE-M3 热态为 76.1 秒。Qdrant collection 为 `aetheris_activities_v1`，数据目录为 `/opt/aetheris/vector/qdrant`。Qdrant HTTP/gRPC 只监听 loopback。

RAG 使用清洗事实中的用户消息、AI 工具、终端、Git、IDE和浏览器作为向量锚点；AI 回复保留为邻域事实。待确认、动态 automation、命令残片和效能排除事实不建立向量。正文保存在 PostgreSQL，Qdrant 只保存向量与过滤维度。

查询 API：

```text
GET  /api/v1/admin/rag/status
POST /api/v1/admin/rag/queries
GET  /api/v1/admin/rag/queries/{query_id}
```

查询异步返回 202，状态依次为 `queued`、`embedding`、`retrieving`、`generating`、`completed|failed`。索引和查询共享查询优先门；稳态索引批量为 16，查询等待当前批次后独占模型算力。Ollama 配置 `OLLAMA_MAX_LOADED_MODELS=2`，Model Gateway 和 Go 内部模型超时为 300 秒。

运维检查：

```bash
systemctl is-active postgresql aetheris-server aetheris-model-gateway ollama qdrant
curl -fsS http://127.0.0.1:6333/readyz
curl -fsS http://127.0.0.1:18081/readyz
curl -fsS http://127.0.0.1:8080/healthz
```

Qdrant 使用官方 x86_64 musl 构建；GNU 构建要求目标机不具备的 glibc 2.38，不能用于当前 Ubuntu。生产固定版本 1.19.1，musl 包 SHA-256 为 `70a40529e2ebe0a2787d574d3a2e28437cfe94f26f24fa419f6ac57b4ae817c9`。

## 浏览器采集策略

浏览器活动默认关闭。租户管理员在 Admin Web 的“浏览器采集”页面配置允许域名并启用后，Windows Core 在下一次 heartbeat 后通过 Device Token 拉取策略。服务端接口为：

```text
GET /api/v1/admin/browser-policy
PUT /api/v1/admin/browser-policy
GET /api/v1/device/browser-policy
```

服务端只接受规范域名，自动去除 HTTP(S) 协议、端口、路径、大小写和末尾点，拒绝通配符、用户信息、非法字符及非 HTTP(S) 协议。Core 策略同步失败时继续使用最后一次有效白名单；本地 Lens 的 `browser_policy` 状态显示启用状态、revision、域名数量和最近错误，不显示域名明细。

个人效能的 `browser_events` 统计清洗后的 `browser.page_view`，并与 AI、终端、IDE、版本控制和其他活动分开展示。活跃分钟仍按主体全局不重复的 5 分钟窗口计算，不能把各活动类型直接相加解释为工时。
