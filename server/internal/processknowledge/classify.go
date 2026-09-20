package processknowledge

import "strings"

func ClassifyTurn(turn SourceTurn) StatementKind {
	content := strings.TrimSpace(strings.ToLower(turn.Content))
	switch turn.EventType {
	case "ai.search_query":
		return SearchQuery
	case "ai.search_result":
		return SearchResult
	case "ai.reasoning_summary":
		return ReasoningSummary
	case "ai.tool_call":
		return ToolCall
	case "ai.tool_result":
		if containsAny(content, "test", "测试") {
			return TestResult
		}
		if containsAny(content, "build", "构建", "编译") {
			return BuildResult
		}
		if containsAny(content, "运行验证", "runtime validation", "健康检查") {
			return RuntimeValidation
		}
		return ToolResult
	}
	switch strings.ToLower(turn.Role) {
	case "user":
		if containsAny(content, "确认", "同意", "采用这个", "按这个", "按此") && len([]rune(content)) < 200 {
			return HumanConfirmation
		}
		if strings.HasPrefix(content, "不对") || containsAny(content, "不同意该方案", "拒绝该方案", "否定该方案", "取消这个方案") {
			return HumanRejection
		}
		if containsAny(content, "必须", "不能", "不要", "不应该", "不应", "边界", "限制") {
			return HumanConstraint
		}
		if containsAny(content, "另外", "还有", "继续", "补充", "那么") {
			return HumanFollowup
		}
		return HumanQuestion
	case "assistant":
		if len([]rune(content)) >= 400 || containsAny(content, "# ", "## ", "完整结论", "最终方案", "总结如下", "分析报告") {
			return AIFinalAnswer
		}
		return AIExploration
	case "tool":
		if containsAny(content, "test", "测试") {
			return TestResult
		}
		if containsAny(content, "build", "构建", "编译") {
			return BuildResult
		}
		if containsAny(content, "commit", "patch", "修改文件") {
			return CodeChange
		}
		return ToolResult
	case "system":
		return SystemContext
	default:
		return SystemContext
	}
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
