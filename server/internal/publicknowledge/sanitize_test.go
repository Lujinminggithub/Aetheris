package publicknowledge

import (
	"strings"
	"testing"
)

func TestSanitizePublicTextRemovesTenantSpecificData(t *testing.T) {
	raw := "项目 safe 位于 E:\\project\\safe，联系 alice@example.com，访问 http://10.0.0.8/admin，token=secret-value"
	safe, report := SanitizePublicText(raw, []string{"safe"})
	for _, forbidden := range []string{"E:\\project\\safe", "alice@example.com", "10.0.0.8", "secret-value", "项目 safe"} {
		if strings.Contains(safe, forbidden) {
			t.Fatalf("safe text leaks %q: %s", forbidden, safe)
		}
	}
	if report.Blocked || report.ReplacementCount < 4 {
		t.Fatalf("report=%+v", report)
	}
}

func TestSanitizePublicTextReplacesCodeBlocks(t *testing.T) {
	safe, report := SanitizePublicText("结论如下：\n```go\npackage private\n```", nil)
	if strings.Contains(safe, "package private") || !strings.Contains(safe, "[已移除代码]") || report.ReplacementCount != 1 {
		t.Fatalf("safe=%q report=%+v", safe, report)
	}
}
