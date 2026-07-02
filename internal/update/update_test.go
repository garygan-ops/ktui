package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestSelectArchiveAsset(t *testing.T) {
	assets := []asset{
		{Name: "ktui_v0.1.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux"},
		{Name: "ktui_v0.1.0_windows_amd64.zip", BrowserDownloadURL: "https://example.com/windows"},
	}

	got, err := selectArchiveAsset(assets, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ktui_v0.1.0_linux_amd64.tar.gz" {
		t.Fatalf("asset = %q", got.Name)
	}

	got, err = selectArchiveAsset(assets, "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ktui_v0.1.0_windows_amd64.zip" {
		t.Fatalf("asset = %q", got.Name)
	}
}

func TestParseChecksum(t *testing.T) {
	data := []byte("abc123  ktui_v0.1.0_linux_amd64.tar.gz\n")
	got, ok := parseChecksum(data, "ktui_v0.1.0_linux_amd64.tar.gz")
	if !ok {
		t.Fatal("checksum not found")
	}
	if got != "abc123" {
		t.Fatalf("checksum = %q", got)
	}
}

func TestSameVersion(t *testing.T) {
	if !sameVersion("v0.1.0", "v0.1.0") {
		t.Fatal("same v-prefixed versions should match")
	}
	if !sameVersion("0.1.0", "v0.1.0") {
		t.Fatal("version should match tag without v prefix")
	}
	if sameVersion("dev", "v0.1.0") {
		t.Fatal("dev version should never be considered up to date")
	}
}

func TestCheckReportsAvailableUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/releases/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v0.2.0",
			Assets: []asset{
				{Name: "ktui_v0.2.0_" + runtime.GOOS + "_" + runtime.GOARCH + archiveExt(), BrowserDownloadURL: serverURL(r, "/download")},
			},
		})
	}))
	defer server.Close()

	result, err := Check(context.Background(), Options{
		APIBaseURL:     server.URL + "/api",
		CurrentVersion: "v0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.LatestVersion != "v0.2.0" || result.AssetName == "" {
		t.Fatalf("result = %+v, want available v0.2.0", result)
	}
}

func TestCheckIgnoresSameVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{TagName: "v0.1.0"})
	}))
	defer server.Close()

	result, err := Check(context.Background(), Options{
		APIBaseURL:     server.URL,
		CurrentVersion: "0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Available {
		t.Fatalf("result = %+v, want no update", result)
	}
}

func archiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func serverURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}
