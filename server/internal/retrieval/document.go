package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
)

func BuildDocument(fact SourceFact, embeddingModel string) (Document, bool) {
	if fact.Excluded || fact.QualityState != "accepted" && fact.QualityState != "merged" {
		return Document{}, false
	}
	if fact.EventType == "ai.message" && fact.MessageRole != "user" {
		return Document{}, false
	}
	var body string
	switch fact.EventType {
	case "ai.tool_call":
		body = fmt.Sprintf("AI 工具：%s\n命令类型：%s\n安全摘要：%s", fact.AITool, fact.CommandType, fact.CommandSummary)
	case "ai.message":
		body = stringValue(fact.Payload["content"])
	case "terminal.command":
		if fact.ActorOrigin == "ai" {
			body = fact.CommandSummary
		} else {
			body = fact.CommandText
		}
	case "ide.file_opened", "ide.file_edited", "ide.file_saved", "ide.file_closed", "ide.workspace_changed", "ide.extension_changed":
		body = activities.IDEBehaviorSummary(fact.EventType, stringValue(fact.Payload["language_id"]))
	case "application.activity":
		body = activities.Preview(fact.EventType, fact.ActorOrigin, "", "", fact.Payload)
	default:
		body = activities.Preview(fact.EventType, fact.ActorOrigin, fact.CommandSummary, fact.CommandText, fact.Payload)
	}
	body = truncate(strings.TrimSpace(body), 4000)
	if body == "" {
		return Document{}, false
	}
	content := fmt.Sprintf("活动类型：%s\n事件类型：%s\n%s", fact.ActivityType, fact.EventType, body)
	contentDigest := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(contentDigest[:])
	idDigest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", fact.FactID, fact.RuleVersion, contentHash)))
	return Document{DocumentID: "doc-" + hex.EncodeToString(idDigest[:16]), TenantID: fact.TenantID, FactID: fact.FactID, SubjectID: fact.SubjectID, DeviceID: fact.DeviceID, ProjectID: fact.ProjectID, RuleVersion: fact.RuleVersion, ActivityType: fact.ActivityType, Content: content, ContentHash: contentHash, EmbeddingModel: embeddingModel, VectorKey: PointID(fact.TenantID, "doc-"+hex.EncodeToString(idDigest[:16])), OccurredAt: fact.OccurredAt}, true
}

func PointID(tenantID, documentID string) string {
	digest := sha256.Sum256([]byte(tenantID + "\x00" + documentID))
	hexValue := hex.EncodeToString(digest[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32])
}

func EmbeddingInput(document Document) string { return truncate(document.Content, 256) }

func CombineRelatedContext(anchor string, replies []string) string {
	parts := []string{strings.TrimSpace(anchor)}
	for _, reply := range replies {
		if value := strings.TrimSpace(reply); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 1 {
		return truncate(parts[0], 1200)
	}
	return truncate(parts[0]+"\n相关 AI 回复：\n"+strings.Join(parts[1:], "\n"), 1200)
}

func stringValue(value any) string { result, _ := value.(string); return result }
func truncate(value string, limit int) string {
	chars := []rune(value)
	if len(chars) > limit {
		return string(chars[:limit])
	}
	return value
}
