package retrieval

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDocumentNeverIndexesAICommandText(t *testing.T) {
	fact := SourceFact{FactID: "fact-ai", TenantID: "tenant-1", RuleVersion: 2, EventType: "ai.tool_call", ActivityType: "ai", ActorOrigin: "ai", QualityState: "accepted", AITool: "codex", CommandType: "vcs", CommandSummary: "执行版本控制操作", CommandText: "git push secret", OccurredAt: time.Now()}

	document, ok := BuildDocument(fact, "bge-m3")

	if !ok || !strings.Contains(document.Content, "执行版本控制操作") || strings.Contains(document.Content, "git push secret") {
		t.Fatalf("document = %#v, ok = %v", document, ok)
	}
}

func TestVSCodeEditFactNeverContainsPathOrCharacterCountsInRetrievalText(t *testing.T) {
	fact := SourceFact{
		FactID: "fact-vscode-1", TenantID: "tenant-1", RuleVersion: 2,
		EventType: "ide.file_edited", ActivityType: "ide", ActorOrigin: "human",
		QualityState: "accepted", Payload: map[string]any{
			"relative_path": "src/app.py", "language_id": "typescript",
			"inserted_chars": 120, "deleted_chars": 30,
		},
	}
	document, ok := BuildDocument(fact, "embeddinggemma")
	if !ok || strings.Contains(document.Content, "src/app.py") || strings.Contains(document.Content, "120") {
		t.Fatalf("unsafe retrieval document: %#v", document)
	}
	if !strings.Contains(document.Content, "编辑 TypeScript 文件") {
		t.Fatal(document.Content)
	}
}

func TestApplicationOCRDocumentNeverContainsVisibleText(t *testing.T) {
	fact := SourceFact{FactID: "fact-app-1", TenantID: "tenant-1", RuleVersion: 3, EventType: "application.activity", ActivityType: "application", ActorOrigin: "human", QualityState: "accepted", Payload: map[string]any{"application_name": "designer.exe", "window_context": "查看配置页面", "visible_text": "客户密码和内部完整内容"}}

	document, ok := BuildDocument(fact, "embeddinggemma")

	if !ok || !strings.Contains(document.Content, "在 designer.exe 中查看配置页面") || strings.Contains(document.Content, "客户密码") {
		t.Fatalf("document = %#v", document)
	}
}

func TestBuildDocumentRejectsExcludedAndQuarantinedFacts(t *testing.T) {
	base := SourceFact{FactID: "fact-1", TenantID: "tenant-1", RuleVersion: 2, EventType: "ai.message", ActivityType: "ai", QualityState: "accepted", Payload: map[string]any{"content": "安全内容"}}
	excluded := base
	excluded.Excluded = true
	quarantined := base
	quarantined.QualityState = "quarantined"

	if _, ok := BuildDocument(excluded, "bge-m3"); ok {
		t.Fatal("excluded fact was indexed")
	}
	if _, ok := BuildDocument(quarantined, "bge-m3"); ok {
		t.Fatal("quarantined fact was indexed")
	}
}

func TestBuildDocumentUsesUserMessagesAsConversationAnchors(t *testing.T) {
	base := SourceFact{FactID: "fact-message", TenantID: "tenant-1", RuleVersion: 2, EventType: "ai.message", ActivityType: "ai", QualityState: "accepted", Payload: map[string]any{"content": "内容"}}
	user := base
	user.MessageRole = "user"
	assistant := base
	assistant.FactID = "fact-assistant"
	assistant.MessageRole = "assistant"
	if _, ok := BuildDocument(user, "bge-m3"); !ok {
		t.Fatal("user anchor was not indexed")
	}
	if _, ok := BuildDocument(assistant, "bge-m3"); ok {
		t.Fatal("assistant reply was independently indexed")
	}
}

func TestPointIDIsDeterministicUUID(t *testing.T) {
	first := PointID("tenant-1", "doc-1")
	if first != PointID("tenant-1", "doc-1") || first == PointID("tenant-2", "doc-1") || len(first) != 36 {
		t.Fatalf("point id = %q", first)
	}
}

func TestEmbeddingInputIsBoundedWithoutTruncatingStoredDocument(t *testing.T) {
	document := Document{Content: strings.Repeat("中", 2000)}
	input := EmbeddingInput(document)
	if len([]rune(input)) != 256 || len([]rune(document.Content)) != 2000 {
		t.Fatalf("input=%d content=%d", len([]rune(input)), len([]rune(document.Content)))
	}
}

func TestCombineRelatedContextKeepsAnchorAndRepliesBounded(t *testing.T) {
	combined := CombineRelatedContext("用户要求修复登录", []string{strings.Repeat("AI 回复一", 500), "AI 回复二"})
	if !strings.Contains(combined, "用户要求修复登录") || !strings.Contains(combined, "AI 回复一") || len([]rune(combined)) > 1200 {
		t.Fatalf("combined = %q", combined)
	}
}
