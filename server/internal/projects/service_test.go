package projects

import "testing"

func validRegistration() Registration {
	return Registration{
		LocalProjectID:    "project-1234567890abcdef",
		DisplayName:       "jtagent",
		VCS:               "git",
		RemoteFingerprint: "hmac-sha256:v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RootFingerprint:   "hmac-sha256:v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		WorkspaceKind:     "worktree",
		WorktreeName:      "feature",
		Active:            true,
		KeyVersion:        1,
		MetadataRevision:  3,
	}
}

func TestResolveUsesExactDeviceBindingBeforeRemoteFingerprint(t *testing.T) {
	registration := validRegistration()
	resolution := ResolveRegistration(registration, &Location{LogicalProjectID: "logical-exact"}, []LogicalProject{
		{ID: "logical-remote", RemoteFingerprint: registration.RemoteFingerprint},
	})
	if resolution.LogicalProjectID != "logical-exact" || resolution.Method != "exact_binding" || resolution.NeedsReview {
		t.Fatalf("unexpected resolution: %+v", resolution)
	}
}

func TestResolveGroupsSingleMatchingRemoteFingerprint(t *testing.T) {
	registration := validRegistration()
	resolution := ResolveRegistration(registration, nil, []LogicalProject{
		{ID: "logical-repo", DisplayName: "仓库", RemoteFingerprint: registration.RemoteFingerprint},
	})
	if resolution.LogicalProjectID != "logical-repo" || resolution.Method != "remote_fingerprint" || resolution.Create {
		t.Fatalf("unexpected resolution: %+v", resolution)
	}
}

func TestResolveNeverMergesByDisplayNameAlone(t *testing.T) {
	registration := validRegistration()
	registration.RemoteFingerprint = ""
	resolution := ResolveRegistration(registration, nil, []LogicalProject{{ID: "logical-other", DisplayName: "jtagent"}})
	if !resolution.Create || resolution.LogicalProjectID != "" || resolution.Method != "new_project" {
		t.Fatalf("same display name was incorrectly merged: %+v", resolution)
	}
}

func TestResolveMarksMultipleRemoteCandidatesForReview(t *testing.T) {
	registration := validRegistration()
	resolution := ResolveRegistration(registration, nil, []LogicalProject{
		{ID: "logical-one", RemoteFingerprint: registration.RemoteFingerprint},
		{ID: "logical-two", RemoteFingerprint: registration.RemoteFingerprint},
	})
	if !resolution.NeedsReview || resolution.Method != "ambiguous_remote" || resolution.Create {
		t.Fatalf("ambiguous candidates were not quarantined: %+v", resolution)
	}
}

func TestValidateRegistrationRejectsUnsafeOrIncompleteValues(t *testing.T) {
	for name, mutate := range map[string]func(*Registration){
		"missing local id":      func(value *Registration) { value.LocalProjectID = "" },
		"absolute display path": func(value *Registration) { value.DisplayName = `C:\\code\\repo` },
		"raw remote":            func(value *Registration) { value.RemoteFingerprint = "https://example.com/repo" },
		"unknown workspace":     func(value *Registration) { value.WorkspaceKind = "temporary" },
		"invalid key version":   func(value *Registration) { value.KeyVersion = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			value := validRegistration()
			mutate(&value)
			if err := ValidateRegistration(value); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
