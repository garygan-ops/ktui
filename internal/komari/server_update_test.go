package komari

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerVersionIsNewer(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "v1.2.4", current: "1.2.3", want: true},
		{latest: "1.3.0", current: "v1.2.9", want: true},
		{latest: "1.2.3", current: "1.2.3", want: false},
		{latest: "1.2.2", current: "1.2.3", want: false},
		{latest: "nightly", current: "1.2.3", want: false},
		{latest: "1.2.4-beta", current: "1.2.3", want: true},
	}
	for _, tt := range tests {
		if got := serverVersionIsNewer(tt.latest, tt.current); got != tt.want {
			t.Fatalf("serverVersionIsNewer(%q, %q) = %t, want %t", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestCheckServerUpdateFiltersStableReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		releases := []serverRelease{
			{TagName: "v1.3.0-beta", Name: "beta", Prerelease: true, HTMLURL: "https://example.com/beta"},
			{TagName: "v1.2.5", Name: "v1.2.5", HTMLURL: "https://example.com/v1.2.5", Published: "2026-07-06T00:00:00Z"},
			{TagName: "v1.2.4", Name: "v1.2.4", HTMLURL: "https://example.com/v1.2.4"},
			{TagName: "v1.2.3", Name: "v1.2.3", HTMLURL: "https://example.com/v1.2.3"},
			{TagName: "v1.2.6", Name: "draft", Draft: true, HTMLURL: "https://example.com/draft"},
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	result, err := CheckServerUpdate(context.Background(), ServerUpdateOptions{
		CurrentVersion: "v1.2.3",
		APIURL:         server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.LatestVersion != "v1.2.5" || result.ReleaseCount != 2 {
		t.Fatalf("result = %+v, want latest v1.2.5 with two stable releases", result)
	}
	if result.ReleaseURL != "https://example.com/v1.2.5" || result.PublishedAt == "" {
		t.Fatalf("result = %+v, want release metadata", result)
	}
}

func TestCheckServerUpdateReturnsUnavailableForCurrentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]serverRelease{
			{TagName: "v1.2.3", Name: "v1.2.3"},
			{TagName: "v1.2.2", Name: "v1.2.2"},
		})
	}))
	defer server.Close()

	result, err := CheckServerUpdate(context.Background(), ServerUpdateOptions{
		CurrentVersion: "1.2.3",
		APIURL:         server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Available || result.ReleaseCount != 0 {
		t.Fatalf("result = %+v, want no update", result)
	}
}

func TestCheckServerUpdateReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := CheckServerUpdate(context.Background(), ServerUpdateOptions{
		CurrentVersion: "1.2.3",
		APIURL:         server.URL,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}
