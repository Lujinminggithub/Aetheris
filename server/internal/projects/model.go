package projects

import "time"

type Registration struct {
	LocalProjectID    string `json:"local_project_id"`
	DisplayName       string `json:"display_name"`
	VCS               string `json:"vcs"`
	RemoteFingerprint string `json:"remote_fingerprint,omitempty"`
	RootFingerprint   string `json:"root_fingerprint"`
	WorkspaceKind     string `json:"workspace_kind"`
	WorktreeName      string `json:"worktree_name,omitempty"`
	Active            bool   `json:"active"`
	KeyVersion        int    `json:"key_version"`
	MetadataRevision  int64  `json:"metadata_revision"`
}

type RegistrationBatch struct {
	Projects []Registration `json:"projects"`
}

type LogicalProject struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id,omitempty"`
	DisplayName       string    `json:"display_name"`
	VCS               string    `json:"vcs"`
	RemoteFingerprint string    `json:"-"`
	Status            string    `json:"status"`
	MetadataRevision  int64     `json:"metadata_revision"`
	CreatedAt         time.Time `json:"created_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}

type Location struct {
	ID               string    `json:"id"`
	LogicalProjectID string    `json:"logical_project_id"`
	DeviceID         string    `json:"device_id"`
	LocalProjectID   string    `json:"local_project_id"`
	DisplayName      string    `json:"display_name"`
	RootFingerprint  string    `json:"-"`
	WorkspaceKind    string    `json:"workspace_kind"`
	WorktreeName     string    `json:"worktree_name,omitempty"`
	Active           bool      `json:"active"`
	KeyVersion       int       `json:"key_version"`
	MetadataRevision int64     `json:"metadata_revision"`
	FirstSeenAt      time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt       time.Time `json:"last_seen_at,omitempty"`
}

type Resolution struct {
	LogicalProjectID string `json:"logical_project_id,omitempty"`
	Method           string `json:"resolution"`
	Create           bool   `json:"-"`
	NeedsReview      bool   `json:"needs_review"`
}

type RegistrationResult struct {
	LocalProjectID   string `json:"local_project_id"`
	LogicalProjectID string `json:"logical_project_id,omitempty"`
	DisplayName      string `json:"display_name"`
	Resolution       string `json:"resolution"`
	NeedsReview      bool   `json:"needs_review"`
}

type BatchResult struct {
	RegistryRevision int64                `json:"registry_revision"`
	Projects         []RegistrationResult `json:"projects"`
}

type ProjectView struct {
	ID               string     `json:"id"`
	DisplayName      string     `json:"display_name"`
	VCS              string     `json:"vcs"`
	Status           string     `json:"status"`
	MetadataRevision int64      `json:"metadata_revision"`
	LocationCount    int        `json:"location_count"`
	Locations        []Location `json:"locations"`
}
