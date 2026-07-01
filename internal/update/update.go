package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultAPIBaseURL = "https://gitea.bytevibe.dev/api/v1/repos/gary/ktui"

type Options struct {
	APIBaseURL     string
	CurrentVersion string
	TargetVersion  string
	CheckOnly      bool
	Timeout        time.Duration
	Stdout         io.Writer
}

type release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Run(ctx context.Context, opts Options) error {
	if opts.APIBaseURL == "" {
		opts.APIBaseURL = DefaultAPIBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	client := &http.Client{Timeout: opts.Timeout}
	rel, err := fetchRelease(ctx, client, opts.APIBaseURL, opts.TargetVersion)
	if err != nil {
		return err
	}
	if rel.TagName == "" {
		return errors.New("release response does not include a tag name")
	}

	if sameVersion(opts.CurrentVersion, rel.TagName) {
		fmt.Fprintf(opts.Stdout, "ktui is already up to date (%s)\n", displayVersion(rel.TagName))
		return nil
	}

	archiveAsset, err := selectArchiveAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	checksumAsset, err := findAsset(rel.Assets, "checksums.txt")
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.Stdout, "latest:  %s\n", displayVersion(rel.TagName))
	fmt.Fprintf(opts.Stdout, "current: %s\n", opts.CurrentVersion)

	if opts.CheckOnly {
		fmt.Fprintf(opts.Stdout, "update available: %s\n", archiveAsset.Name)
		return nil
	}

	checksums, err := downloadBytes(ctx, client, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	expected, ok := parseChecksum(checksums, archiveAsset.Name)
	if !ok {
		return fmt.Errorf("checksums.txt does not contain %s", archiveAsset.Name)
	}

	archivePath, cleanup, err := downloadFile(ctx, client, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", archiveAsset.Name, err)
	}
	defer cleanup()

	if err := verifyChecksum(archivePath, expected); err != nil {
		return err
	}

	binary, mode, err := extractBinary(archivePath, archiveAsset.Name)
	if err != nil {
		return err
	}
	if err := installBinary(binary, mode, opts.Stdout); err != nil {
		return err
	}

	fmt.Fprintf(opts.Stdout, "updated ktui to %s\n", displayVersion(rel.TagName))
	return nil
}

func fetchRelease(ctx context.Context, client *http.Client, apiBaseURL, tag string) (release, error) {
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	endpoint := apiBaseURL + "/releases/latest"
	if tag != "" {
		endpoint = apiBaseURL + "/releases/tags/" + url.PathEscape(tag)
	}

	var rel release
	if err := getJSON(ctx, client, endpoint, &rel); err != nil {
		return release{}, err
	}
	return rel, nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	addAuth(req)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func downloadBytes(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	addAuth(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func downloadFile(ctx context.Context, client *http.Client, endpoint string) (string, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", nil, err
	}
	addAuth(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", nil, fmt.Errorf("GET %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	file, err := os.CreateTemp("", "ktui-update-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}

	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func addAuth(req *http.Request) {
	token := os.Getenv("KTUI_UPDATE_TOKEN")
	if token == "" {
		token = os.Getenv("GITEA_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
}

func selectArchiveAsset(assets []asset, goos, goarch string) (asset, error) {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	suffix := "_" + goos + "_" + goarch + ext
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "ktui_") && strings.HasSuffix(asset.Name, suffix) && asset.BrowserDownloadURL != "" {
			return asset, nil
		}
	}
	return asset{}, fmt.Errorf("release does not contain an asset for %s/%s", goos, goarch)
}

func findAsset(assets []asset, name string) (asset, error) {
	for _, asset := range assets {
		if asset.Name == name && asset.BrowserDownloadURL != "" {
			return asset, nil
		}
	}
	return asset{}, fmt.Errorf("release does not contain %s", name)
}

func parseChecksum(data []byte, filename string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if path.Base(name) == filename || filepath.Base(name) == filename {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func verifyChecksum(filename, expected string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filepath.Base(filename), got, expected)
	}
	return nil
}

func extractBinary(archivePath, archiveName string) ([]byte, os.FileMode, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractBinaryFromZip(archivePath)
	}
	return extractBinaryFromTarGZ(archivePath)
}

func extractBinaryFromTarGZ(filename string) ([]byte, os.FileMode, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, 0, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if path.Base(header.Name) == binaryName() {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, 0, err
			}
			return data, os.FileMode(header.Mode), nil
		}
	}
	return nil, 0, fmt.Errorf("archive does not contain %s", binaryName())
}

func extractBinaryFromZip(filename string) ([]byte, os.FileMode, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, 0, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if path.Base(file.Name) != binaryName() {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, 0, err
		}
		data, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			return nil, 0, err
		}
		return data, file.Mode(), nil
	}
	return nil, 0, fmt.Errorf("archive does not contain %s", binaryName())
}

func installBinary(data []byte, mode os.FileMode, stdout io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}

	mode = installMode(executable, mode)
	if runtime.GOOS == "windows" {
		next := executable + ".new"
		if err := os.WriteFile(next, data, mode); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "downloaded update to %s\n", next)
		fmt.Fprintf(stdout, "Windows cannot replace a running executable. Close ktui and replace %s manually.\n", executable)
		return nil
	}

	dir := filepath.Dir(executable)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(executable)+".new-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, mode); err != nil {
		return err
	}
	if err := os.Rename(tempName, executable); err != nil {
		return fmt.Errorf("replace %s: %w", executable, err)
	}
	return nil
}

func installMode(executable string, fallback os.FileMode) os.FileMode {
	if info, err := os.Stat(executable); err == nil {
		return info.Mode().Perm() | 0o111
	}
	mode := fallback.Perm()
	if mode == 0 {
		mode = 0o755
	}
	return mode | 0o111
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "ktui.exe"
	}
	return "ktui"
}

func sameVersion(current, tag string) bool {
	current = normalizeVersion(current)
	tag = normalizeVersion(tag)
	if current == "" || current == "dev" || current == "none" {
		return false
	}
	return current == tag
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "v")
	return value
}

func displayVersion(tag string) string {
	return strings.TrimSpace(tag)
}
