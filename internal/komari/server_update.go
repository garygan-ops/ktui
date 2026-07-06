package komari

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultServerReleaseAPIURL = "https://api.github.com/repos/komari-monitor/komari/releases?per_page=100"

type ServerUpdateOptions struct {
	CurrentVersion string
	APIURL         string
	Timeout        time.Duration
	HTTPClient     *http.Client
}

type ServerUpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	LatestName     string
	ReleaseURL     string
	PublishedAt    string
	ReleaseCount   int
	Available      bool
}

type serverRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Published  string `json:"published_at"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

var semverPrefixRE = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

func CheckServerUpdate(ctx context.Context, opts ServerUpdateOptions) (ServerUpdateResult, error) {
	if opts.APIURL == "" {
		opts.APIURL = DefaultServerReleaseAPIURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}

	current := strings.TrimSpace(opts.CurrentVersion)
	result := ServerUpdateResult{CurrentVersion: current}
	if current == "" {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	releases, err := fetchServerReleases(ctx, client, opts.APIURL)
	if err != nil {
		return ServerUpdateResult{}, err
	}
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		version := valueOrString(rel.TagName, rel.Name)
		if !serverVersionIsNewer(version, current) {
			continue
		}
		if !result.Available {
			result.LatestVersion = version
			result.LatestName = rel.Name
			result.ReleaseURL = rel.HTMLURL
			result.PublishedAt = rel.Published
			result.Available = true
		}
		result.ReleaseCount++
	}
	return result, nil
}

func fetchServerReleases(ctx context.Context, client *http.Client, endpoint string) ([]serverRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var releases []serverRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func serverVersionIsNewer(latest string, current string) bool {
	next, ok := parseServerSemver(latest)
	if !ok {
		return false
	}
	base, ok := parseServerSemver(current)
	if !ok {
		return false
	}
	for i := 0; i < len(next); i++ {
		if next[i] > base[i] {
			return true
		}
		if next[i] < base[i] {
			return false
		}
	}
	return false
}

func parseServerSemver(value string) ([3]int, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	value = strings.TrimPrefix(value, "V")
	match := semverPrefixRE.FindStringSubmatch(value)
	if match == nil {
		return [3]int{}, false
	}
	var version [3]int
	for i := range version {
		parsed, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, false
		}
		version[i] = parsed
	}
	return version, true
}

func valueOrString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
