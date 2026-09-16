package activities

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func Classify(eventType, source string) string {
	switch {
	case strings.HasPrefix(eventType, "ai.") || strings.HasPrefix(source, "core.ai."):
		return "ai"
	case eventType == "application.activity" || strings.HasPrefix(source, "core.application."):
		return "application"
	case eventType == "terminal.command":
		return "terminal"
	case strings.HasPrefix(eventType, "ide.") || strings.Contains(source, "vscode") || strings.Contains(source, "visual_studio"):
		return "ide"
	case strings.HasPrefix(eventType, "browser.") || strings.HasPrefix(source, "core.browser"):
		return "browser"
	case strings.HasPrefix(eventType, "git.") || strings.HasPrefix(eventType, "svn."):
		return "version_control"
	default:
		return "other"
	}
}

func Supported(eventType, source string) bool {
	return Classify(eventType, source) != "other" || eventType == "process.observed"
}

func ProjectLabel(safeLabel, pathHint, storedName, projectID string) string {
	if value := strings.TrimSpace(safeLabel); value != "" {
		return value
	}
	value := strings.TrimRight(strings.TrimSpace(pathHint), `\/`)
	if value != "" {
		if leaf := filepath.Base(strings.ReplaceAll(value, `\`, "/")); leaf != "." && leaf != "/" && !strings.Contains(leaf, ":") {
			return leaf
		}
	}
	if value := strings.TrimSpace(storedName); value != "" && value != projectID {
		return value
	}
	return projectID
}

func Preview(eventType, actorOrigin, commandSummary, commandText string, payload map[string]any) string {
	if eventType == "ai.tool_call" || actorOrigin == "ai" && commandSummary != "" {
		return limit(commandSummary, 240)
	}
	if eventType == "terminal.command" {
		return limit(commandText, 240)
	}
	if eventType == "ai.message" {
		return limit(value(payload, "content"), 240)
	}
	switch Classify(eventType, "") {
	case "version_control":
		if subject := value(payload, "subject"); subject != "" {
			return limit(subject, 240)
		}
		return fmt.Sprintf("变更文件 %v，新增 %v，删除 %v", payload["files_changed"], payload["insertions"], payload["deletions"])
	case "ide":
		if summary := IDEBehaviorSummary(eventType, value(payload, "language_id")); summary != "" {
			return summary
		}
		return limit(filepath.Base(strings.ReplaceAll(value(payload, "active_file"), `\`, "/")), 240)
	case "browser":
		if content := value(payload, "visible_text"); content != "" {
			return limit(content, 240)
		}
		return limit(value(payload, "domain"), 240)
	case "application":
		application := value(payload, "application_name")
		context := value(payload, "window_context")
		if application == "" || context == "" {
			return ""
		}
		return limit(fmt.Sprintf("在 %s 中%s", application, context), 240)
	}
	raw, _ := json.Marshal(payload)
	return limit(string(raw), 240)
}

func IDEBehaviorSummary(eventType, languageID string) string {
	action := map[string]string{
		"ide.file_opened": "打开", "ide.file_edited": "编辑", "ide.file_saved": "保存", "ide.file_closed": "关闭",
	}[eventType]
	if action != "" {
		language := map[string]string{
			"typescript": "TypeScript", "typescriptreact": "TypeScript", "javascript": "JavaScript", "javascriptreact": "JavaScript",
			"python": "Python", "go": "Go", "java": "Java", "csharp": "C#", "cpp": "C++", "rust": "Rust",
		}[strings.ToLower(strings.TrimSpace(languageID))]
		if language == "" {
			return action + "代码文件"
		}
		return action + " " + language + " 文件"
	}
	switch eventType {
	case "ide.workspace_changed":
		return "切换工作区"
	case "ide.extension_changed":
		return "开发扩展环境变化"
	}
	return ""
}

func value(payload map[string]any, key string) string {
	result, _ := payload[key].(string)
	return result
}
func limit(value string, size int) string {
	value = strings.TrimSpace(value)
	characters := []rune(value)
	if len(characters) > size {
		return string(characters[:size])
	}
	return value
}
