package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/demo"
)

func getPage[T any](t *testing.T, handler http.Handler, path string) T {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s = %d: %s", path, response.Code, response.Body.String())
	}
	var page T
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestActivityCursorPagesAndCurrentAuthorization(t *testing.T) {
	repository := demo.New(time.Now().UTC())
	handler := (&Server{Fleet: repository, Auth: viewerAuth{user: repository.Viewer(), allowed: true}}).Handler()
	path := "/api/v1/activity?limit=2"
	seen := map[string]bool{}
	var firstCursor string
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page := getPage[contract.ActivityPage](t, handler, path)
		if len(page.Events) > 2 {
			t.Fatal("activity exceeded the requested page limit")
		}
		for _, event := range page.Events {
			if seen[event.Id] {
				t.Fatal("activity repeated an event across pages")
			}
			seen[event.Id] = true
		}
		if page.NextCursor == nil {
			break
		}
		if firstCursor == "" {
			firstCursor = *page.NextCursor
		}
		path = "/api/v1/activity?limit=2&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	if len(seen) < 3 || firstCursor == "" {
		t.Fatal("fixture did not exercise multiple pages")
	}
	// A cursor issued before access was revoked carries no authority.
	revoked := (&Server{Fleet: repository, Auth: viewerAuth{user: config.User{Subject: "reader"}, allowed: true}}).Handler()
	page := getPage[contract.ActivityPage](t, revoked, "/api/v1/activity?cursor="+url.QueryEscape(firstCursor))
	if len(page.Events) != 0 || page.NextCursor != nil {
		t.Fatal("the cursor exposed activity after access was revoked")
	}
}

func TestHistoryCursorBindsDeploymentAndWindow(t *testing.T) {
	now := time.Now().UTC()
	repository := demo.New(now)
	handler := (&Server{Fleet: repository, Auth: viewerAuth{user: repository.Viewer(), allowed: true}}).Handler()
	since := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	path := "/api/v1/deployments/identity/history?limit=1&since=" + url.QueryEscape(since)
	first := getPage[contract.History](t, handler, path)
	if len(first.Observations) != 1 || first.Baseline == nil || first.NextCursor == nil ||
		!first.Baseline.RecordedAt.Before(now.Add(-48*time.Hour)) {
		t.Fatal("windowed history did not retain its pre-window baseline and cursor")
	}
	second := getPage[contract.History](t, handler, path+"&cursor="+url.QueryEscape(*first.NextCursor))
	if second.Baseline != nil || len(second.Observations) != 1 ||
		!second.Observations[0].RecordedAt.Before(first.Observations[0].RecordedAt) {
		t.Fatal("the second page repeated its baseline or observation")
	}
	for _, invalid := range []string{
		strings.Replace(path, "/identity/", "/payments/", 1),
		strings.Replace(path, url.QueryEscape(since), url.QueryEscape(now.Format(time.RFC3339Nano)), 1),
		"/api/v1/activity?limit=1",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, invalid+"&cursor="+url.QueryEscape(*first.NextCursor), nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("cursor used on another list returned %d", response.Code)
		}
	}
}

func TestPageQueriesRejectInvalidInputAndAuthorizeHistoryFirst(t *testing.T) {
	repository := demo.New(time.Now().UTC())
	handler := (&Server{Fleet: repository, Auth: viewerAuth{user: repository.Viewer(), allowed: true}}).Handler()
	for _, query := range []string{
		"limit=0", "limit=501", "limit=-1", "limit=one", "limit=1&limit=2", "cursor=", "cursor=!", "cursor=" + strings.Repeat("a", 513),
	} {
		for _, path := range []string{"/api/v1/activity", "/api/v1/deployments/identity/history"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
			if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("invalid page query returned %d", response.Code)
			}
		}
	}
	for _, query := range []string{
		"since=", "since=yesterday", "since=0000-01-01T00:00:00Z",
		"since=2026-10-01T00:00:00Z&since=2026-10-02T00:00:00Z",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/identity/history?"+query, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatal("invalid history window was accepted")
		}
	}
	hidden := (&Server{Fleet: repository, Auth: viewerAuth{allowed: true}}).Handler()
	response := httptest.NewRecorder()
	hidden.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/identity/history?limit=invalid", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal("an unauthorized history request was validated before authorization")
	}
	anonymous := (&Server{Fleet: repository, Auth: viewerAuth{}}).Handler()
	response = httptest.NewRecorder()
	anonymous.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatal("anonymous activity request was accepted")
	}
}
