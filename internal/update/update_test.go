package update

import "testing"

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
