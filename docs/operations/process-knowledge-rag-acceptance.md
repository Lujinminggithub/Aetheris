# 过程知识增强智能查询验收

## 自动化门禁

```powershell
python -m unittest discover -s tests -v
go -C server test ./...
python -m unittest discover -s model-gateway/tests -v
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
```

重点覆盖：

- 同设备、同项目、相同时间但不同 `session_id` 的消息不会串联。
- 用户问题不能独立生成技术结论。
- AI 最终回答、人工确认和测试结果分别形成结论、决策和验证状态。
- 长对话后半段的 EDR 内容能够形成独立知识块。
- supersession 事件继承被替代事件的逻辑项目。
- 过程知识查询必须选择逻辑项目或显式选择全部项目。
- 关键词与向量召回经过 RRF、知识单元去重、会话配额和验证状态排序。
- analysis 回答使用动态生成预算，并先生成回答计划。

## 历史回填

先执行 dry-run：

```bash
set -a
. /opt/aetheris/config/server.env
set +a
/opt/aetheris/bin/aetheris-admin process-knowledge-backfill \
  --mode dry_run --project logical-project-safe --version 1
```

确认会话、候选、冲突和未归属数量后执行 apply。回填完成后通过管理 API 将版本切换为
`shadow`，对比新旧检索；抽样通过后再进入 `canary` 和 `active`。

## 固定 EDR 问题

问题：

> 帮我分析，Windows 操作系统如何实现一个 EDR

必须满足：

1. 覆盖总体架构、内核采集、用户态代理、检测关联、响应执行和管理闭环。
2. 使用 `safe` 过程知识，但不输出项目能力清单。
3. 用户提示词不得作为事实证据。
4. 已验证本地结果优先，并说明适用条件。
5. 本地结论引用 `knowledge_id` 和原始事件 ID。
6. 选择 `safe` 时不得混入未归属 Claude Code 或其他项目。

## 远期评测数据

```bash
/opt/aetheris/bin/aetheris-admin process-knowledge-export-eval \
  --output /opt/aetheris/backups/process-knowledge-eval-v1.jsonl
```

导出只包含 `accepted + verified + active` 的知识单元。该文件用于离线评测和后续人工审核，
不代表已经启用在线训练或自动微调。
