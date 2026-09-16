# 简洁智能查询回答 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Nexus 根据问题粒度输出简洁业务答案，默认隐藏实现细节，并保留可按需展开的授权证据。

**Architecture:** QueryService 先用确定性规则分类回答模式，再让模型网关返回严格结构化 JSON。服务端验证模式、长度、引用和内部细节，失败时重试一次，仍失败则返回确定性“数据不足”答案；Admin Web 默认只显示主答案和置信度，将引用折叠展示。

**Tech Stack:** Go 1.23、PostgreSQL JSONB、Ollama/model-gateway、React 19、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-09-15-process-ocr-fallback-and-concise-rag-design.md`

## Global Constraints

- `direct` 问题默认一至两句，主答案最多 80 个中文字符。
- `numeric` 先给数值和单位；`reason` 最多三个原因；`procedure` 最多五步；只有 `analysis` 可展开长说明。
- 默认主答案不暴露模型、Embedding、Qdrant、向量、数据库表、内部 API、Worker、索引和日志连接状态。
- 证据引用只能来自本次租户、主体、设备、项目和日期授权后的检索结果。
- 非法模型输出最多重试一次，之后返回“当前数据不足以确认。”，不得透传原文。
- 前端默认折叠依据；用户明确询问详细说明时自动使用 `analysis`，首期不新增模式选择器。
- 所有产品文案使用中文，内部固定错误码不得直接显示给用户。
- 工作区没有 `.git` 时不创建提交，改为记录实施检查点。

---

### Task 1: 确定性问题意图分类

**状态：已完成（检查点 1）**

**Files:**
- Create: `server/internal/retrieval/intent.go`
- Create: `server/internal/retrieval/intent_test.go`

**Interfaces:**
- Produces: `ClassifyAnswerMode(question string) AnswerMode` with `direct|numeric|reason|procedure|analysis`.
- Consumes: trimmed Chinese or English question up to the existing 1000-rune limit.

- [ ] **Step 1: Write failing table-driven tests**

```go
func TestClassifyAnswerMode(t *testing.T) {
    cases := map[string]AnswerMode{
        "线路有没有流量限制": DirectMode,
        "一共有多少次失败": NumericMode,
        "为什么线路会限速": ReasonMode,
        "怎么配置线路限速": ProcedureMode,
        "详细分析限速实现和证据": AnalysisMode,
    }
    for question, want := range cases {
        if got := ClassifyAnswerMode(question); got != want { t.Fatalf("%q: %s", question, got) }
    }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/retrieval -run ClassifyAnswerMode -v`  
Expected: FAIL because `AnswerMode` and classifier do not exist.

- [ ] **Step 3: Implement ordered classification**

Normalize whitespace and case. Match explicit detailed-analysis terms first, then procedure, reason, numeric and direct terms. Default to `direct`. Do not invoke a model for classification. Export `Valid()` so request/result validation shares the same mode set.

- [ ] **Step 4: Verify GREEN and ambiguous cases**

Run: `go -C server test ./internal/retrieval -run ClassifyAnswerMode -v`  
Expected: PASS, including “有没有” as direct even when retrieved evidence contains technical terms.

- [ ] **Step 5: Record checkpoint**

Record classifier cases and focused test output in the implementation log.

### Task 2: 结构化生成契约和严格验证器

**状态：已完成（检查点 2）**

**Files:**
- Create: `server/internal/retrieval/answer.go`
- Create: `server/internal/retrieval/answer_test.go`
- Modify: `server/internal/retrieval/generator.go`
- Modify: `server/internal/retrieval/query_service.go`
- Modify: `server/internal/retrieval/query_test.go`

**Interfaces:**
- Produces: `type GeneratedAnswer struct { Answer string; Mode AnswerMode; Confidence string; Details string; CitationNumbers []int }`.
- Produces: `ValidateGeneratedAnswer(answer GeneratedAnswer, allowed []Citation) error`.
- Changes: `AnswerGenerator.Generate(ctx, question, mode, citations, correction string) (GeneratedAnswer, error)`.

- [ ] **Step 1: Write failing contract tests**

```go
func TestDirectAnswerRejectsLongOrInternalTechnicalDetail(t *testing.T) {
    allowed := []Citation{{Number: 1}}
    bad := GeneratedAnswer{Answer: "线路存在限制，Qdrant 向量索引尚未读取服务器日志。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}}
    if ValidateGeneratedAnswer(bad, allowed) == nil { t.Fatal("internal detail accepted") }
}

func TestDirectAnswerAcceptsConciseBusinessConclusion(t *testing.T) {
    answer := GeneratedAnswer{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}}
    if err := ValidateGeneratedAnswer(answer, []Citation{{Number: 1}}); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/retrieval -run 'GeneratedAnswer|DirectAnswer' -v`  
Expected: FAIL because the structured type and validator do not exist.

- [ ] **Step 3: Implement prompt and parser**

Send `answer_mode`, per-mode length rules and allowed citation numbers in the system message. Require exactly the JSON fields from the spec. Parse a JSON object only; reject Markdown fences, free text, unknown modes, invalid confidence, empty answer, details in non-analysis modes and citations outside the allowed set.

- [ ] **Step 4: Implement relevance and detail validation**

For `direct`, enforce at most 80 runes and two Chinese sentence terminators. Reject platform-internal terms with a narrow case-insensitive set: `qdrant`, `embedding`, `向量索引`, `数据库表`, `内部 api`, `worker`, `模型网关`, `rag`, `未连接服务器`, `未读取日志`, `索引状态`. Do not ban domain terms such as “限速器” globally; those are permitted only in `analysis` or when the question contains “实现/机制/算法”.

- [ ] **Step 5: Verify GREEN**

Run: `go -C server test ./internal/retrieval -run 'GeneratedAnswer|DirectAnswer|Generator' -v`  
Expected: PASS for valid JSON, invalid citations, fenced output, long direct answers and internal-detail leakage.

### Task 3: 单次纠错重试和确定性降级

**状态：已完成（检查点 2）**

**Files:**
- Modify: `server/internal/retrieval/query_service.go`
- Modify: `server/internal/retrieval/query_test.go`

**Interfaces:**
- Consumes: Task 1 classifier and Task 2 generator/validator.
- Produces: `QueryService.generateAnswer(ctx, job, citations) GeneratedAnswer` with one retry maximum.

- [ ] **Step 1: Write failing retry and fallback tests**

```go
func TestQueryRetriesInvalidAnswerOnceThenUsesSafeFallback(t *testing.T) {
    generator := &sequenceGenerator{answers: []GeneratedAnswer{invalidTechnicalAnswer(), invalidTechnicalAnswer()}}
    service.RunJob(ctx, job)
    if generator.calls != 2 { t.Fatalf("calls=%d", generator.calls) }
    if repository.completed.Answer != "当前数据不足以确认。" { t.Fatal(repository.completed.Answer) }
}

func TestDirectQuestionStoresOnlyConciseMainAnswer(t *testing.T) {
    service.RunJob(ctx, directJob("线路有没有流量限制"))
    if repository.completed.Answer != "线路存在流量限制。" { t.Fatal(repository.completed) }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/retrieval -run 'RetriesInvalid|ConciseMain' -v`  
Expected: FAIL because QueryService still accepts one free-text response.

- [ ] **Step 3: Implement one retry and safe fallback**

Classify mode before generation. Validate the first result; on failure call the generator once more with a correction string containing only the fixed validation reason code, never the rejected model text. If the second response fails, return `{answer:"当前数据不足以确认。", answer_mode:<classified>, confidence:"low", details:"", citation_numbers:[]}` and still complete the query successfully.

- [ ] **Step 4: Ensure citations match selected numbers**

Complete the query with only citations whose numbers appear in `CitationNumbers`, preserving their original order. An answer with no selected citations is accepted only for the deterministic insufficient-data fallback.

- [ ] **Step 5: Verify full query service**

Run: `go -C server test ./internal/retrieval -v`  
Expected: PASS; generator call count never exceeds two.

### Task 4: 持久化结构化答案并兼容旧查询

**状态：已完成（检查点 3）**

**Files:**
- Create: `server/migrations/019_structured_rag_answers.sql`
- Modify: `server/internal/retrieval/query_service.go`
- Modify: `server/internal/retrieval/repository.go`
- Modify: `server/internal/retrieval/repository_test.go`
- Modify: `server/internal/httpapi/rag_handlers_test.go`

**Interfaces:**
- Adds query fields: `answer_mode`, `confidence`, `details`, `citation_numbers`.
- Adds optional `StructuredQueryRepository.CompleteStructuredQuery(ctx, id string, answer GeneratedAnswer, citations []Citation) error`; legacy `QueryRepository.CompleteQuery(ctx, id, answer string, citations)` remains as a compatibility fallback.
- Preserves: JSON `answer` string for old Admin Web clients.

- [ ] **Step 1: Write failing repository round-trip tests**

```go
func TestStructuredAnswerRoundTripsWithLegacyAnswer(t *testing.T) {
    saved := GeneratedAnswer{Answer:"线路存在流量限制。", Mode:DirectMode, Confidence:"high", CitationNumbers:[]int{1}}
    repository.CompleteQuery(ctx, "q1", saved, citations)
    got := repository.GetQuery(ctx, tenant, actor, "q1")
    if got.Answer != saved.Answer || got.AnswerMode != DirectMode || got.Confidence != "high" { t.Fatal(got) }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go -C server test ./internal/retrieval ./internal/httpapi -run StructuredAnswer -v`  
Expected: FAIL because migration and repository fields are missing.

- [ ] **Step 3: Add backward-compatible migration and repository mapping**

Add non-null defaults: `answer_mode='direct'`, `answer_confidence='low'`, `answer_details=''`, `citation_numbers='[]'::jsonb`. Backfill completed legacy rows as direct/low with citation numbers derived from stored citations. Validate decoded numbers again in `GetQuery` and return an empty list on malformed legacy data.

- [ ] **Step 4: Verify API wire contract**

Ensure GET query response contains `answer`, `answer_mode`, `confidence`, `details`, `citation_numbers`, and authorized `citations`. POST remains unchanged so existing callers do not select a mode manually.

- [ ] **Step 5: Verify database and handler tests**

Run: `go -C server test ./internal/retrieval ./internal/httpapi -v`  
Expected: PASS.

### Task 5: Admin Web 主答案优先和依据折叠

**状态：已完成（检查点 4）**

**Files:**
- Modify: `admin-web/src/api/types.ts`
- Modify: `admin-web/src/pages/RAGQueryPage.tsx`
- Modify: `admin-web/src/pages/RAGQueryPage.test.tsx`
- Modify: `admin-web/src/styles.css`

**Interfaces:**
- Consumes: structured query job fields from Task 4.
- Produces: concise answer panel, confidence label and collapsed evidence disclosure.

- [ ] **Step 1: Write failing product-behavior tests**

```tsx
it('默认只显示简洁答案并折叠技术证据', async () => {
  mockCompleted({ answer: '线路存在流量限制。', answer_mode: 'direct', confidence: 'high', citations: [citation] })
  render(<RAGQueryPage />)
  expect(await screen.findByText('线路存在流量限制。')).toBeInTheDocument()
  expect(screen.getByText('高置信度')).toBeInTheDocument()
  expect(screen.queryByText(citation.excerpt)).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '查看依据（1）' }))
  expect(screen.getByText(citation.excerpt)).toBeInTheDocument()
  expect(screen.queryByText(/embeddinggemma|向量状态/)).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run the test and verify RED**

Run: `npm --prefix admin-web test -- --run src/pages/RAGQueryPage.test.tsx`  
Expected: FAIL because evidence is currently always visible and model details are shown.

- [ ] **Step 3: Implement compact answer presentation**

Remove embedding model and vector implementation labels from the page. Show the answer as normal body text, a Chinese confidence badge, and a native button-controlled evidence region with `aria-expanded`. Keep evidence cards and detail drawer unchanged inside the expanded region. Show `details` only when `answer_mode=analysis` and it is non-empty.

- [ ] **Step 4: Verify responsive layout and accessibility**

Ensure answer and longest Chinese status text wrap without overlap at 360px, 768px and 1440px widths. Keep cards at 8px radius or less. The disclosure must be keyboard operable and preserve focus after closing the evidence drawer.

- [ ] **Step 5: Run frontend verification**

Run:

```powershell
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
```

Expected: all tests and production build pass.

### Task 6: 示例回归、发布与回滚验证

**状态：已完成（检查点 5）**

**Files:**
- Create: `docs/operations/concise-rag-acceptance.md`
- Modify: `docs/operations/implementation-log.md`
- Produce: updated Linux server release and Admin Web static bundle.

**Interfaces:**
- Consumes: Tasks 1-5.
- Produces: deployed structured-answer Nexus behavior with rollback artifacts.

- [ ] **Step 1: Add exact acceptance fixtures**

Run a deterministic generator fixture for:

```text
问题：线路有没有流量限制
主答案：线路存在流量限制。
模式：direct
details：空
```

Also verify “为什么线路会限速” uses reason mode and “详细说明限速实现和证据” uses analysis mode.

- [ ] **Step 2: Run full verification**

Run:

```powershell
go -C server test ./...
npm --prefix admin-web test -- --run
npm --prefix admin-web run build
python -m unittest discover -s tests
```

Expected: zero failures.

- [ ] **Step 3: Build and stage Linux release**

Build `aetheris-server` and `aetheris-migrate` for Linux amd64 with `CGO_ENABLED=0`, copy migrations and `admin-web/dist`, archive the staged release, and compute SHA-256. Do not modify production during staging.

- [ ] **Step 4: Deploy with rollback checkpoint**

Read `/opt/aetheris`, service state and disk usage. Back up server, migrate binary, migrations and Admin static directory. Deploy atomically, restart the service, and verify migration 019, `/healthz`, `/admin/` and existing client download.

- [ ] **Step 5: Verify real model behavior**

Submit the three acceptance questions through the authenticated Admin Web/API. Confirm the direct answer contains no internal technical terms, evidence is collapsed in UI, citations expand correctly, and invalid-output fallback produces “当前数据不足以确认。” Record response mode, confidence and citation count without storing sensitive evidence text in the implementation log.
