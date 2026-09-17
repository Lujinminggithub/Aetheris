package processknowledge

import (
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

func EligibleTrainingCandidate(unit KnowledgeUnit) bool {
	return unit.DecisionState == "accepted" && unit.ValidationState == "verified" && unit.LifecycleState == "active"
}

func EDRAcceptanceGaps(answer string, citations []retrieval.Citation) []string {
	value := strings.ToLower(answer)
	required := map[string][]string{
		"总体架构":  {"架构", "组成", "协作"},
		"内核采集":  {"内核", "采集"},
		"用户态代理": {"用户态", "代理"},
		"检测关联":  {"检测", "关联"},
		"响应执行":  {"响应", "阻断", "隔离"},
		"管理闭环":  {"管理", "闭环"},
	}
	gaps := []string{}
	for topic, terms := range required {
		matched := false
		for _, term := range terms {
			if strings.Contains(value, term) {
				matched = true
				break
			}
		}
		if !matched {
			gaps = append(gaps, topic)
		}
	}
	hasKnowledge := false
	for _, citation := range citations {
		if citation.KnowledgeID != "" {
			hasKnowledge = true
			break
		}
	}
	if !hasKnowledge {
		gaps = append(gaps, "过程知识引用")
	}
	if strings.Contains(value, "当前项目已经实现") || strings.Contains(value, "safe 已实现") {
		gaps = append(gaps, "禁止能力清单")
	}
	return gaps
}
