package processknowledge

import "testing"

func TestAtomizeKnowledgeCreatesScopedClaimsAndPreservesEvidence(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "knowledge", Topic: "DLP", Problem: "Windows DLP 如何实现内容阻断", Conclusion: "采集文件和剪贴板内容，图片通过 OCR 提取文本。\n- 命中策略后阻断并记录审计。", Applicability: "Windows 11", ValidationState: "verified"}, 1, []string{"evidence-1"})
	if len(claims) != 2 {
		t.Fatalf("claims=%+v", claims)
	}
	for _, claim := range claims {
		if claim.Domain != "dlp" || len(claim.Entities) == 0 || len(claim.EvidenceIDs) != 1 || claim.ValidationState != "verified" {
			t.Fatalf("claim=%+v", claim)
		}
	}
}

func TestAtomizeKnowledgeRejectsWorkflowAndCrossDomainContent(t *testing.T) {
	if claims := AtomizeKnowledge(KnowledgeDraft{ID: "agent", Topic: "研发过程知识", Problem: "继续规格实施", Conclusion: "Agent message from /root: FINAL_ANSWER src/nb_live.c:33 FEC 修复"}, 1, nil); len(claims) != 0 {
		t.Fatalf("workflow claim=%+v", claims)
	}
	if claims := AtomizeKnowledge(KnowledgeDraft{ID: "cross", Topic: "DLP", Problem: "Windows DLP 如何阻断", Conclusion: "FEC 恢复公网丢包并调整媒体队列，避免首包过期"}, 1, nil); len(claims) != 0 {
		t.Fatalf("cross-domain claim=%+v", claims)
	}
}

func TestAtomizeKnowledgeUsesProblemDomainInsteadOfPollutedTopic(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "mislabeled", Topic: "DLP", Problem: "UDP 下行丢包如何处理", Conclusion: "FEC 增加冗余分片以恢复公网丢包。", ValidationState: "unverified"}, 1, []string{"evidence-1"})
	if len(claims) != 1 || claims[0].Domain != "network_transport" {
		t.Fatalf("claims=%+v", claims)
	}
}

func TestAtomizeKnowledgeUsesExplicitClaimDomainWhenProblemSpansDomains(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "mixed", Problem: "Windows DLP 与 UDP 链路问题如何隔离证据", Conclusion: "FEC 增加冗余分片以恢复公网丢包。", ValidationState: "unverified"}, 1, []string{"evidence-1"})
	if len(claims) != 1 || claims[0].Domain != "network_transport" {
		t.Fatalf("claims=%+v", claims)
	}
}

func TestAtomizeKnowledgeRejectsAmbiguousClaimUnderMultiDomainProblem(t *testing.T) {
	claims := AtomizeKnowledge(KnowledgeDraft{ID: "ambiguous", Problem: "Windows DLP 与 UDP 链路问题如何隔离证据", Conclusion: "当前配置需要先核对实际运行状态。", ValidationState: "unverified"}, 1, []string{"evidence-1"})
	if len(claims) != 0 {
		t.Fatalf("ambiguous claims=%+v", claims)
	}
}

func TestAtomizeKnowledgeRejectsStructuralFragmentsAndClaimsWithoutEvidence(t *testing.T) {
	tests := []struct {
		name       string
		problem    string
		conclusion string
		evidence   []string
	}{
		{name: "arrow fragment", conclusion: "-> Exit HY2 UDP packet path", evidence: []string{"evidence-1"}},
		{name: "source path", conclusion: "vendor-sing-quic/hysteria2/traffic.go", evidence: []string{"evidence-1"}},
		{name: "content hash", conclusion: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", evidence: []string{"evidence-1"}},
		{name: "local source link", problem: "EDR 事件存储如何限制磁盘占用", conclusion: "核心修复位于 [edr_incident_store.ts](E:/project/safe/src/edr_incident_store.ts:23)。", evidence: []string{"evidence-1"}},
		{name: "size observation", conclusion: "大小：172,813,865 字节", evidence: []string{"evidence-1"}},
		{name: "check status", conclusion: "TypeScript 检查通过。", evidence: []string{"evidence-1"}},
		{name: "test status", conclusion: "68 个非视觉自动化测试全部通过。", evidence: []string{"evidence-1"}},
		{name: "execution narration", problem: "如何搭建 RAG 知识库", conclusion: "这属于一个新的本地知识库子系统，需要按架构型任务设计。", evidence: []string{"evidence-1"}},
		{name: "shell command", problem: "Windows DLP 如何配置调试参数", conclusion: `reg add "HKLM\SYSTEM\CurrentControlSet\Services\PersonalSafer\Parameters" /v DebugMask /t REG_DWORD /d 15 /f`, evidence: []string{"evidence-1"}},
		{name: "installer link", problem: "Windows DLP 如何构建安装包", conclusion: "新安装包：[PersonalSafer Setup.exe](</E:/project/safe/dist/PersonalSafer Setup.exe>)", evidence: []string{"evidence-1"}},
		{name: "language label", conclusion: "powershell", evidence: []string{"evidence-1"}},
		{name: "compound verification status", problem: "Windows DLP 如何验证", conclusion: "内核、native、TypeScript、六项自动化门禁及代理 HTTP/HTTPS 自检均通过；新驱动签名和包内哈希验证通过。", evidence: []string{"evidence-1"}},
		{name: "log fragment", problem: "Windows DLP OCR 如何验证", conclusion: "成功：`[PS][fileaudit] ocr completed chars=... warnings=OCR_COMPLETED", evidence: []string{"evidence-1"}},
		{name: "implementation artifact", problem: "Windows DLP 如何兼容旧配置", conclusion: "新增旧配置迁移与运行模式回归测试。", evidence: []string{"evidence-1"}},
		{name: "broken identifier fragment", problem: "Windows DLP OCR 如何识别中英文", conclusion: "eng`、`chi_sim` 分别识别后合并。", evidence: []string{"evidence-1"}},
		{name: "no evidence", conclusion: "FEC 增加冗余分片以恢复公网丢包。"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := tt.problem
			if problem == "" {
				problem = "UDP 下行丢包如何处理"
			}
			claims := AtomizeKnowledge(KnowledgeDraft{ID: tt.name, Problem: problem, Conclusion: tt.conclusion, ValidationState: "unverified"}, 1, tt.evidence)
			if len(claims) != 0 {
				t.Fatalf("claims=%+v", claims)
			}
		})
	}
}
