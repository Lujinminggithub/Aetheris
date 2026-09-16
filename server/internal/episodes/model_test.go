package episodes

import (
	"testing"
	"time"
)

func TestBuildEpisodeLinksObjectiveActionsAndValidationToEvidence(t *testing.T) {
	at := time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC)
	result := Build([]Fact{
		{EventID: "event-user", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "logical-1", SessionID: "session-1", EventType: "ai.message", Role: "user", Content: "修复登录超时问题", OccurredAt: at},
		{EventID: "event-tool", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "logical-1", SessionID: "session-1", EventType: "ai.tool_call", Role: "tool", Summary: "修改认证超时时间", OccurredAt: at.Add(time.Minute)},
		{EventID: "event-test", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "logical-1", SessionID: "session-1", EventType: "ide.test", Role: "system", Summary: "测试通过", OccurredAt: at.Add(2 * time.Minute)},
	})
	if len(result) != 1 {
		t.Fatalf("episodes=%d", len(result))
	}
	episode := result[0]
	if episode.SubjectID != "subject-1" {
		t.Fatalf("subject id was not propagated: %+v", episode)
	}
	if episode.DeviceID != "device-1" {
		t.Fatalf("device id was not propagated: %+v", episode)
	}
	if episode.Objective != "修复登录超时问题" || len(episode.Actions) != 1 || len(episode.Validations) != 1 || episode.Outcome != "验证通过" {
		t.Fatalf("unexpected episode=%+v", episode)
	}
	for _, evidence := range episode.Evidence {
		if evidence.EventID == "" {
			t.Fatal("empty evidence event id")
		}
	}
}

func TestBuildDoesNotTreatCommandInvocationAsSuccessfulValidation(t *testing.T) {
	result := Build([]Fact{{
		EventID: "event-tool", ProjectID: "logical-1", SessionID: "session-1", EventType: "ai.tool_call", Role: "tool", Summary: "执行测试命令", OccurredAt: time.Now().UTC(),
	}})
	if len(result) != 1 || result[0].Outcome != "" || len(result[0].Validations) != 0 {
		t.Fatalf("command invocation implied success: %+v", result)
	}
}

func TestBuildSplitsEpisodesAfterThirtyMinuteInactivity(t *testing.T) {
	at := time.Date(2026, 9, 9, 9, 0, 0, 0, time.UTC)
	result := Build([]Fact{
		{EventID: "event-1", ProjectID: "logical-1", SessionID: "session-1", EventType: "ai.message", Role: "user", Content: "第一项工作", OccurredAt: at},
		{EventID: "event-2", ProjectID: "logical-1", SessionID: "session-1", EventType: "ai.message", Role: "user", Content: "第二项工作", OccurredAt: at.Add(31 * time.Minute)},
	})
	if len(result) != 2 {
		t.Fatalf("episodes=%d", len(result))
	}
}

func TestVSCodeFileBehaviorBecomesEpisodeAction(t *testing.T) {
	at := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	result := Build([]Fact{{EventID: "save-1", SubjectID: "s1", DeviceID: "d1", ProjectID: "p1", SessionID: "vscode-1", EventType: "ide.file_saved", Role: "human", Summary: "保存代码文件", OccurredAt: at}})
	if len(result) != 1 || len(result[0].Actions) != 1 || result[0].Actions[0].Summary != "保存代码文件" {
		t.Fatalf("episodes = %#v", result)
	}
}

func TestApplicationOCRBehaviorBecomesShortEpisodeAction(t *testing.T) {
	at := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	result := Build([]Fact{{EventID: "app-1", SubjectID: "s1", DeviceID: "d1", ProjectID: "p1", SessionID: "app-session", EventType: "application.activity", Role: "human", Summary: "在 designer.exe 中查看配置页面", OccurredAt: at}})
	if len(result) != 1 || len(result[0].Actions) != 1 || result[0].Actions[0].Summary != "在 designer.exe 中查看配置页面" {
		t.Fatalf("episodes = %#v", result)
	}
}
