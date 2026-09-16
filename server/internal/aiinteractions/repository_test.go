package aiinteractions

import "testing"

func TestProjectLabelDoesNotExposeLocalPath(t *testing.T) {
	tests := []struct {
		path, projectID, want string
	}{
		{`E:\code\jtagent`, "project-1", "jtagent"},
		{"/home/dev/service/", "project-2", "service"},
		{"", "project-3", "project-3"},
	}
	for _, test := range tests {
		if got := ProjectLabel(test.path, test.projectID); got != test.want {
			t.Fatalf("ProjectLabel(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestNormalizeMessageRoleUsesStableAdminCategories(t *testing.T) {
	tests := map[string]string{
		"user": "user", "assistant": "assistant", "ai_tool": "ai_tool", "system": "system", "tool": "tool",
		" USER ": "user", "function": "unknown", "": "unknown",
	}
	for input, want := range tests {
		if got := NormalizeMessageRole(input); got != want {
			t.Fatalf("NormalizeMessageRole(%q) = %q, want %q", input, got, want)
		}
	}
}
