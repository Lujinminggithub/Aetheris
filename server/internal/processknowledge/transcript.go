package processknowledge

import (
	"regexp"
	"strings"
	"time"
)

var transcriptMarker = regexp.MustCompile(`(?m)^\s*\[(\d+)\]\s+(user|assistant|system|tool(?:\s+[^:]*)?):\s*`)
var environmentBlock = regexp.MustCompile(`(?s)<environment_context>.*?</environment_context>`)

func ExpandSourceTurn(source SourceTurn) []SourceTurn {
	content := strings.TrimSpace(source.Content)
	if strings.Contains(content, ">>> TRANSCRIPT") {
		matches := transcriptMarker.FindAllStringSubmatchIndex(content, -1)
		if len(matches) > 0 {
			result := make([]SourceTurn, 0, len(matches))
			for index, match := range matches {
				start := match[1]
				end := len(content)
				if index+1 < len(matches) {
					end = matches[index+1][0]
				}
				value := strings.TrimSpace(strings.TrimSuffix(content[start:end], ">>> TRANSCRIPT END"))
				if value == "" {
					continue
				}
				item := source
				roleValue := strings.ToLower(content[match[4]:match[5]])
				switch {
				case strings.HasPrefix(roleValue, "tool"):
					item.Role = "tool"
				case roleValue == "assistant":
					item.Role = "assistant"
				case roleValue == "system":
					item.Role = "system"
				default:
					item.Role = "user"
				}
				item.Content = cleanProcessContent(value)
				item.SourceKey = "transcript-" + content[match[2]:match[3]]
				item.OccurredAt = source.OccurredAt.Add(time.Duration(index) * time.Nanosecond)
				if item.Content != "" {
					result = append(result, item)
				}
			}
			return result
		}
	}
	content = cleanProcessContent(content)
	if content == "" {
		return nil
	}
	source.Content = content
	return []SourceTurn{source}
}

func cleanProcessContent(value string) string {
	value = environmentBlock.ReplaceAllString(value, "")
	if index := strings.Index(value, "## My request:"); index >= 0 {
		value = value[index+len("## My request:"):]
	} else if index := strings.Index(value, "## My request"); index >= 0 {
		value = value[index+len("## My request"):]
	}
	return strings.TrimSpace(value)
}
