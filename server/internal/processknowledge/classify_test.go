package processknowledge

import "testing"

func TestClassifyTurnSeparatesIntentConclusionAndValidation(t *testing.T) {
	cases := []struct {
		turn SourceTurn
		want StatementKind
	}{
		{SourceTurn{Role: "user", Content: "Windows 如何实现一个 EDR？"}, HumanQuestion},
		{SourceTurn{Role: "user", Content: "确认采用这个方案"}, HumanConfirmation},
		{SourceTurn{Role: "user", Content: "确认按取消动态验证码的方案实施"}, HumanConfirmation},
		{SourceTurn{Role: "user", Content: "不对，不要使用该方案"}, HumanRejection},
		{SourceTurn{Role: "user", Content: "另外，服务端通信不要依赖 bootstrap 和 Broker"}, HumanConstraint},
		{SourceTurn{Role: "user", Content: "必须保证高 IRQL 路径不执行重操作"}, HumanConstraint},
		{SourceTurn{Role: "assistant", Content: "我先检查代码"}, AIExploration},
		{SourceTurn{Role: "assistant", Content: "# 完整结论\n" + longText(500)}, AIFinalAnswer},
		{SourceTurn{EventType: "ai.search_query", Role: "tool", Content: "Windows EDR callbacks"}, SearchQuery},
		{SourceTurn{EventType: "ai.search_result", Role: "tool", Content: "Microsoft Learn 官方文档"}, SearchResult},
		{SourceTurn{EventType: "ai.reasoning_summary", Role: "assistant", Content: "比较 ETW 和内核回调"}, ReasoningSummary},
		{SourceTurn{EventType: "ai.tool_result", Role: "tool", Content: "读取官方文档完成"}, ToolResult},
		{SourceTurn{EventType: "ai.tool_result", Role: "tool", Content: "go test ./... PASS"}, TestResult},
		{SourceTurn{Role: "tool", Content: "go test ./... PASS"}, TestResult},
		{SourceTurn{EventType: "ai.tool_call", Role: "tool", Content: "运行测试"}, ToolCall},
	}
	for _, item := range cases {
		if got := ClassifyTurn(item.turn); got != item.want {
			t.Errorf("content=%q got=%s want=%s", item.turn.Content, got, item.want)
		}
	}
}
