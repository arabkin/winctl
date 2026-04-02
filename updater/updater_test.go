package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func serveRelease(t *testing.T, rel ghRelease) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rel)
	}))
}

func makeRelease(version string, assets []ghAsset) ghRelease {
	return ghRelease{
		TagName: "v" + version,
		Name:    "Release " + version,
		Body:    "Release notes for " + version,
		Assets:  assets,
	}
}

func exeAsset(url, digest string) ghAsset {
	return ghAsset{
		Name:               "winctl.exe",
		BrowserDownloadURL: url,
		Size:               1024,
		Digest:             digest,
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	rel := makeRelease("2.0.0", []ghAsset{
		exeAsset("https://example.com/winctl.exe", "sha256:abc123def456"),
	})
	srv := serveRelease(t, rel)
	defer srv.Close()

	u := New("1.0.2", srv.URL)
	info, err := u.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Available {
		t.Fatal("expected Available=true")
	}
	if info.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %s", info.Version)
	}
	if info.SHA256 != "abc123def456" {
		t.Fatalf("expected SHA256 abc123def456, got %s", info.SHA256)
	}
}

func TestCheckNoUpdateWhenSameVersion(t *testing.T) {
	rel := makeRelease("1.0.2", []ghAsset{
		exeAsset("https://example.com/winctl.exe", "sha256:abc123"),
	})
	srv := serveRelease(t, rel)
	defer srv.Close()

	u := New("1.0.2", srv.URL)
	info, err := u.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Fatal("expected Available=false for same version")
	}
}

func TestCheckNoUpdateWhenOlderVersion(t *testing.T) {
	rel := makeRelease("1.0.1", []ghAsset{
		exeAsset("https://example.com/winctl.exe", "sha256:abc123"),
	})
	srv := serveRelease(t, rel)
	defer srv.Close()

	u := New("1.0.2", srv.URL)
	info, err := u.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Fatal("expected Available=false for older version")
	}
}

func TestCheckHandlesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := New("1.0.2", srv.URL)
	_, err := u.Check()
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestCheckHandlesNoExeAsset(t *testing.T) {
	rel := makeRelease("2.0.0", []ghAsset{
		{
			Name:               "winctl.tar.gz",
			BrowserDownloadURL: "https://example.com/winctl.tar.gz",
			Size:               2048,
			Digest:             "sha256:aaa",
		},
	})
	srv := serveRelease(t, rel)
	defer srv.Close()

	u := New("1.0.2", srv.URL)
	_, err := u.Check()
	if err == nil {
		t.Fatal("expected error when no .exe asset found")
	}
}

func TestDownloadAndVerifySHA256(t *testing.T) {
	content := []byte("fake binary content for testing")
	h := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(h[:])

	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer dlSrv.Close()

	u := New("1.0.2", "")
	info := UpdateInfo{
		Available:   true,
		DownloadURL: dlSrv.URL,
		SHA256:      expectedHash,
	}

	path, err := u.Download(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatal("downloaded content does not match expected")
	}
}

func TestDownloadRejectsSHA256Mismatch(t *testing.T) {
	content := []byte("some content")

	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer dlSrv.Close()

	u := New("1.0.2", "")
	info := UpdateInfo{
		Available:   true,
		DownloadURL: dlSrv.URL,
		SHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
	}

	_, err := u.Download(info)
	if err == nil {
		t.Fatal("expected error for SHA256 mismatch")
	}
	if got := err.Error(); !contains(got, "SHA256 mismatch") {
		t.Fatalf("expected error containing 'SHA256 mismatch', got: %s", got)
	}
}

func TestDownloadHandlesHTTPError(t *testing.T) {
	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dlSrv.Close()

	u := New("1.0.2", "")
	info := UpdateInfo{
		Available:   true,
		DownloadURL: dlSrv.URL,
	}

	_, err := u.Download(info)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		remote  string
		current string
		want    bool
	}{
		{"2.0.0", "1.0.2", true},
		{"1.0.3", "1.0.2", true},
		{"1.1.0", "1.0.2", true},
		{"1.0.2", "1.0.2", false},
		{"1.0.1", "1.0.2", false},
		{"0.9.0", "1.0.2", false},
		{"not-a-version", "1.0.0", false},
		{"1.0", "1.0.0", false},
		{"", "1.0.0", false},
		{"1.0.0", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.remote+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewer(tt.remote, tt.current)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.remote, tt.current, got, tt.want)
			}
		})
	}
}

func TestCheckStripsVPrefix(t *testing.T) {
	rel := makeRelease("2.0.0", []ghAsset{
		exeAsset("https://example.com/winctl.exe", "sha256:abc123def456"),
	})
	// makeRelease prepends "v" to the tag, so TagName is "v2.0.0".
	srv := serveRelease(t, rel)
	defer srv.Close()

	u := New("1.0.0", srv.URL)
	info, err := u.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "2.0.0" {
		t.Errorf("expected Version='2.0.0' (no v prefix), got %q", info.Version)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
