package activities

import "testing"

func TestClassifyUsesUnifiedActivityTypes(t *testing.T) {
	tests := []struct{ eventType, source, want string }{
		{"ai.message", "core.ai.codex", "ai"},
		{"ai.tool_call", "core.ai.claude_code", "ai"},
		{"terminal.command", "core.terminal", "terminal"},
		{"ide.activity", "core.vscode", "ide"},
		{"browser.page_view", "core.browser", "browser"},
		{"git.commit", "core.git", "version_control"},
		{"svn.activity", "core.svn", "version_control"},
		{"process.observed", "core.process", "other"},
		{"application.activity", "core.application.ocr", "application"},
	}
	for _, test := range tests {
		if got := Classify(test.eventType, test.source); got != test.want {
			t.Fatalf("Classify(%q, %q) = %q, want %q", test.eventType, test.source, got, test.want)
		}
	}
}

func TestApplicationPreviewUsesWindowContextNotOCRBody(t *testing.T) {
	payload := map[string]any{"application_name": "designer.exe", "window_context": "查看配置页面", "visible_text": "内部完整 OCR 文本"}
	if got := Preview("application.activity", "human", "", "", payload); got != "在 designer.exe 中查看配置页面" {
		t.Fatalf("preview = %q", got)
	}
}

func TestProjectLabelPrefersSafeLabelAndNeverReturnsPath(t *testing.T) {
	tests := []struct{ safe, hint, stored, id, want string }{
		{"Codex", `C:\\private\\workspace`, "project-id", "project-1", "Codex"},
		{"", `E:\\code\\service-api`, "project-id", "project-2", "service-api"},
		{"", "", "Claude Code", "project-3", "Claude Code"},
	}
	for _, test := range tests {
		if got := ProjectLabel(test.safe, test.hint, test.stored, test.id); got != test.want {
			t.Fatalf("ProjectLabel(...) = %q, want %q", got, test.want)
		}
	}
}

func TestPreviewUsesSafeAICommandSummary(t *testing.T) {
	payload := map[string]any{"command": "secret command", "command_summary": "执行测试", "content": "ignored"}
	if got := Preview("ai.tool_call", "ai", "执行测试", "secret command", payload); got != "执行测试" {
		t.Fatalf("preview = %q", got)
	}
}
