package processknowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

func ChunkKnowledge(unit KnowledgeDraft, minimum, maximum int) []ChunkDraft {
	if minimum < 1 {
		minimum = 600
	}
	if maximum < minimum {
		maximum = 1000
	}
	content := strings.TrimSpace(fmt.Sprintf("主题：%s\n问题：%s\n约束：%s\n结论：%s\n依据：%s\n适用条件：%s\n边界：%s", unit.Topic, unit.Problem, unit.Constraints, unit.Conclusion, unit.Rationale, unit.Applicability, unit.Caveats))
	if content == "" {
		return nil
	}
	runes := []rune(content)
	chunks := []ChunkDraft{}
	for start := 0; start < len(runes); {
		end := start + maximum
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = safeBoundary(runes, start+minimum, end)
		}
		if end <= start {
			end = minInt(start+maximum, len(runes))
		}
		text := strings.TrimSpace(string(runes[start:end]))
		if text != "" {
			digest := sha256.Sum256([]byte(text))
			id := stableID("chunk", unit.ID, fmt.Sprint(len(chunks)), hex.EncodeToString(digest[:]))
			chunks = append(chunks, ChunkDraft{ID: id, KnowledgeID: unit.ID, Topic: unit.Topic, Content: text, SearchText: unit.Topic + "\n" + text, ContentHash: hex.EncodeToString(digest[:]), Index: len(chunks)})
		}
		if end == len(runes) {
			break
		}
		start = end
	}
	return chunks
}

func safeBoundary(value []rune, minimum, maximum int) int {
	if maximum >= len(value) {
		return len(value)
	}
	for index := maximum; index >= minimum; index-- {
		if isBoundary(value[index-1]) {
			return index
		}
	}
	if maximum > 0 && maximum < len(value) && isIdentifierRune(value[maximum-1]) && isIdentifierRune(value[maximum]) {
		for index := maximum - 1; index >= minimum; index-- {
			if !isIdentifierRune(value[index-1]) {
				return index
			}
		}
	}
	return maximum
}

func isBoundary(value rune) bool {
	return unicode.IsSpace(value) || strings.ContainsRune("。；，、！？.!?;,:)】]}", value)
}
func isIdentifierRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
