package projects

import (
	"fmt"
	"regexp"
	"strings"
)

var fingerprintPattern = regexp.MustCompile(`^hmac-sha256:v[1-9][0-9]*:[a-f0-9]{64}$`)

func ValidateRegistration(value Registration) error {
	value.LocalProjectID = strings.TrimSpace(value.LocalProjectID)
	value.DisplayName = strings.TrimSpace(value.DisplayName)
	if !strings.HasPrefix(value.LocalProjectID, "project-") || len(value.LocalProjectID) > 128 {
		return fmt.Errorf("本地项目标识无效")
	}
	if value.DisplayName == "" || len([]rune(value.DisplayName)) > 255 || strings.ContainsAny(value.DisplayName, `/\\`) {
		return fmt.Errorf("项目显示名称无效")
	}
	if value.VCS != "git" && value.VCS != "svn" && value.VCS != "none" {
		return fmt.Errorf("版本控制类型无效")
	}
	if value.RemoteFingerprint != "" && !fingerprintPattern.MatchString(value.RemoteFingerprint) {
		return fmt.Errorf("远程仓库指纹无效")
	}
	if !fingerprintPattern.MatchString(value.RootFingerprint) {
		return fmt.Errorf("项目根目录指纹无效")
	}
	if value.WorkspaceKind != "primary" && value.WorkspaceKind != "clone" && value.WorkspaceKind != "worktree" && value.WorkspaceKind != "non_vcs" {
		return fmt.Errorf("工作区类型无效")
	}
	if value.KeyVersion < 1 || value.MetadataRevision < 0 {
		return fmt.Errorf("项目元数据版本无效")
	}
	return nil
}

func ResolveRegistration(registration Registration, exact *Location, candidates []LogicalProject) Resolution {
	if exact != nil && exact.LogicalProjectID != "" {
		return Resolution{LogicalProjectID: exact.LogicalProjectID, Method: "exact_binding"}
	}
	if registration.RemoteFingerprint == "" {
		return Resolution{Method: "new_project", Create: true}
	}
	matches := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.ID != "" && candidate.RemoteFingerprint == registration.RemoteFingerprint {
			matches[candidate.ID] = struct{}{}
		}
	}
	if len(matches) == 1 {
		for id := range matches {
			return Resolution{LogicalProjectID: id, Method: "remote_fingerprint"}
		}
	}
	if len(matches) > 1 {
		return Resolution{Method: "ambiguous_remote", NeedsReview: true}
	}
	return Resolution{Method: "new_project", Create: true}
}
