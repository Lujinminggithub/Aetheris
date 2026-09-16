package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/aetheris-dev/aetheris/server/internal/episodes"
)

type fakeEpisodeStore struct{ items []episodes.Episode }

func (store *fakeEpisodeStore) List(_ context.Context, _ string, filter episodes.Filter) ([]episodes.Episode, error) {
	if filter.DeviceID != "device-1" && len(store.items) > 0 && store.items[0].DeviceID == "device-1" {
		return nil, context.Canceled
	}
	return store.items, nil
}
func (store *fakeEpisodeStore) Get(_ context.Context, _, id string) (episodes.Episode, error) {
	for _, item := range store.items {
		if item.EpisodeID == id {
			return item, nil
		}
	}
	return episodes.Episode{}, context.Canceled
}

func TestAdminEpisodesListsOnlyWithEventReadPermission(t *testing.T) {
	store := &fakeEpisodeStore{items: []episodes.Episode{{EpisodeID: "episode-1", ProjectID: "logical-1", Objective: "修复登录"}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/work-episodes", nil)

	forbidden := httptest.NewRecorder()
	serveAdminEpisodes(forbidden, request, authorization.Principal{TenantID: "tenant-1", Permissions: map[string]bool{}}, Dependencies{Episodes: store})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status=%d", forbidden.Code)
	}

	allowed := httptest.NewRecorder()
	serveAdminEpisodes(allowed, request, authorization.Principal{TenantID: "tenant-1", Permissions: map[string]bool{"events:read": true}}, Dependencies{Episodes: store})
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), "修复登录") {
		t.Fatalf("status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestAdminEpisodeDetailReturnsEvidence(t *testing.T) {
	store := &fakeEpisodeStore{items: []episodes.Episode{{EpisodeID: "episode-1", ProjectID: "logical-1", Objective: "修复登录", Evidence: []episodes.Evidence{{EventID: "event-1", Section: "objective", Reason: "用户目标"}}}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/work-episodes/episode-1", nil)
	response := httptest.NewRecorder()
	serveAdminEpisodes(response, request, authorization.Principal{TenantID: "tenant-1", Permissions: map[string]bool{"events:read": true}}, Dependencies{Episodes: store})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event-1") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminEpisodeListPassesDeviceFilter(t *testing.T) {
	store := &fakeEpisodeStore{items: []episodes.Episode{{EpisodeID: "episode-1", DeviceID: "device-1", Objective: "修复登录"}}}
	response := httptest.NewRecorder()
	serveAdminEpisodes(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/work-episodes?device_id=device-1", nil), authorization.Principal{TenantID: "tenant-1", Permissions: map[string]bool{"events:read": true}}, Dependencies{Episodes: store})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
