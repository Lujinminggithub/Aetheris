package authorization

import "fmt"

type Principal struct {
	Kind        string
	ID          string
	TenantID    string
	SubjectID   string
	DeviceID    string
	AccessRole  string
	Permissions map[string]bool
}

type Scope struct {
	TenantID  string
	ProjectID string
}

func Require(principal Principal, permission string, scope Scope) error {
	if principal.TenantID == "" || principal.TenantID != scope.TenantID {
		return fmt.Errorf("tenant scope denied")
	}
	if !principal.Permissions[permission] {
		return fmt.Errorf("permission denied: %s", permission)
	}
	return nil
}
