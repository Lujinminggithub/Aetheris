package effectiveness

import "strings"

const MetricDefinitionVersion = 6

type ActivityClass string

const (
	ActivityDelivery ActivityClass = "delivery"
	ActivityCoding   ActivityClass = "coding"
	ActivityTerminal ActivityClass = "terminal"
	ActivityAI       ActivityClass = "ai_collaboration"
	ActivityBrowser  ActivityClass = "browser"
	ActivityOther    ActivityClass = "other"
)

func Classify(eventType, source string) ActivityClass {
	switch eventType {
	case "git.commit", "git.diff":
		return ActivityDelivery
	case "ide.file_saved":
		return ActivityDelivery
	case "ai.session", "ai.message", "ai.tool_call":
		return ActivityAI
	case "terminal.command":
		return ActivityTerminal
	case "application.activity":
		return ActivityCoding
	case "browser.page_view":
		return ActivityBrowser
	case "process.observed", "ide.activity", "vscode.activity", "visualstudio.activity":
		return ActivityCoding
	}
	if strings.HasPrefix(eventType, "ide.") {
		return ActivityCoding
	}
	if strings.HasPrefix(source, "core.ai.") {
		return ActivityAI
	}
	return ActivityOther
}

func ShouldReadRawEvent(cleaningRuleVersion int, eventType, source string) bool {
	if cleaningRuleVersion < 1 {
		return true
	}
	if cleaningRuleVersion == 1 {
		return eventType != "terminal.command" && eventType != "ai.message" && eventType != "ai.tool_call"
	}
	if strings.HasPrefix(eventType, "ide.") {
		return false
	}
	if cleaningRuleVersion >= 3 && eventType == "application.activity" {
		return false
	}
	switch eventType {
	case "terminal.command", "ai.message", "ai.tool_call", "ide.activity", "browser.page_view", "git.commit", "git.diff", "svn.activity", "process.observed":
		return false
	}
	return !strings.HasPrefix(source, "core.visual_studio.")
}
