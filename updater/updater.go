package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultAPIURL = "https://api.github.com/repos/arabkin/winctl/releases/latest"

// UpdateInfo holds information about an available update.
type UpdateInfo struct {
	Available   bool   `json:"available"`
	Version     string `json:"version,omitempty"`
	ReleaseName string `json:"release_name,omitempty"`
	Body        string `json:"body,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// ghAsset represents a GitHub release asset.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

// ghRelease represents a GitHub release API response.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Name    string    `json:"name"`
	Body    string    `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

// Updater checks for and downloads updates from GitHub releases.
type Updater struct {
	currentVersion string
	apiURL         string

	mu     sync.RWMutex
	cached UpdateInfo
}

// New creates an Updater. If apiURL is empty, the default GitHub API URL is used.
func New(currentVersion, apiURL string) *Updater {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         apiURL,
	}
}

// Check queries the GitHub releases API and returns update information.
func (u *Updater) Check() (UpdateInfo, error) {
	resp, err := http.Get(u.apiURL)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("fetching release info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return UpdateInfo{Available: false}, nil
		}
		return UpdateInfo{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel); err != nil {
		return UpdateInfo{}, fmt.Errorf("decoding release JSON: %w", err)
	}

	remoteVersion := strings.TrimPrefix(rel.TagName, "v")

	if !isNewer(remoteVersion, u.currentVersion) {
		info := UpdateInfo{Available: false}
		u.mu.Lock()
		u.cached = info
		u.mu.Unlock()
		return info, nil
	}

	// Find .exe asset
	var exeAsset *ghAsset
	for i := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(rel.Assets[i].Name), ".exe") {
			exeAsset = &rel.Assets[i]
			break
		}
	}

	if exeAsset == nil {
		return UpdateInfo{}, fmt.Errorf("no .exe asset found in release %s", rel.TagName)
	}

	sha := ""
	if after, found := strings.CutPrefix(exeAsset.Digest, "sha256:"); found {
		sha = after
	}

	info := UpdateInfo{
		Available:   true,
		Version:     remoteVersion,
		ReleaseName: rel.Name,
		Body:        rel.Body,
		DownloadURL: exeAsset.BrowserDownloadURL,
		Size:        exeAsset.Size,
		SHA256:      sha,
	}

	u.mu.Lock()
	u.cached = info
	u.mu.Unlock()

	return info, nil
}

// Cached returns the last check result without making a network call.
func (u *Updater) Cached() UpdateInfo {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cached
}

// Download fetches the binary from info.DownloadURL into a temp file and
// verifies its SHA256 checksum. It returns the path to the temp file.
func (u *Updater) Download(info UpdateInfo) (string, error) {
	resp, err := http.Get(info.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "winctl-update-*.exe")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	h := sha256.New()
	w := io.MultiWriter(tmp, h)

	if _, err := io.Copy(w, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("writing update file: %w", err)
	}
	_ = tmp.Close()

	gotHash := hex.EncodeToString(h.Sum(nil))
	if info.SHA256 == "" {
		log.Printf("warning: no SHA256 checksum in release metadata — integrity not verified")
	} else if gotHash != info.SHA256 {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", info.SHA256, gotHash)
	} else {
		log.Printf("SHA256 verified: %s", gotHash)
	}

	return tmp.Name(), nil
}

// BackgroundCheck runs an immediate update check, then repeats at the given
// interval until ctx is cancelled. Intended to be called as a goroutine.
func BackgroundCheck(u *Updater, ctx context.Context, intervalMinutes int) {
	check := func() {
		if info, err := u.Check(); err != nil {
			log.Printf("update check: %v", err)
		} else if info.Available {
			log.Printf("update available: v%s", info.Version)
		}
	}
	check()
	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			check()
		case <-ctx.Done():
			return
		}
	}
}

// isNewer returns true if remote is a newer semver than current.
func isNewer(remote, current string) bool {
	rParts := parseSemver(remote)
	cParts := parseSemver(current)
	if rParts == nil || cParts == nil {
		return false
	}
	for i := range 3 {
		if rParts[i] > cParts[i] {
			return true
		}
		if rParts[i] < cParts[i] {
			return false
		}
	}
	return false
}

// parseSemver parses "X.Y.Z" into [3]int. Returns nil on failure.
func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}
