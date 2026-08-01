package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
)

func TestResolveRepo(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		repoURL    string
		branch     string
		wantOwner  string
		wantRepo   string
		wantBranch string
		wantErr    bool
	}{
		{
			name:       "git url",
			url:        "https://github.com/lvim-tech/clipack-registry.git",
			branch:     "main",
			wantOwner:  "lvim-tech",
			wantRepo:   "clipack-registry",
			wantBranch: "main",
		},
		{
			name:       "url without .git suffix",
			url:        "https://github.com/lvim-tech/clipack-registry",
			branch:     "develop",
			wantOwner:  "lvim-tech",
			wantRepo:   "clipack-registry",
			wantBranch: "develop",
		},
		{
			// The contents API form has an extra "repos" path segment.
			name:       "falls back to the api url",
			repoURL:    "https://api.github.com/repos/lvim-tech/clipack-registry/contents",
			wantOwner:  "lvim-tech",
			wantRepo:   "clipack-registry",
			wantBranch: "main",
		},
		{
			name:       "empty branch defaults to main",
			url:        "https://github.com/owner/repo.git",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantBranch: "main",
		},
		{
			name:    "no usable url",
			wantErr: true,
		},
		{
			name:    "url without a repository name",
			url:     "https://github.com/owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &cnfg.Config{Registry: cnfg.RegistryConfig{
				URL:             tt.url,
				RegistryRepoURL: tt.repoURL,
				Branch:          tt.branch,
			}}

			ref, err := resolveRepo(config)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveRepo() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRepo() error = %v", err)
			}
			if ref.Owner != tt.wantOwner || ref.Repo != tt.wantRepo || ref.Branch != tt.wantBranch {
				t.Errorf("resolveRepo() = %+v, want %s/%s@%s",
					ref, tt.wantOwner, tt.wantRepo, tt.wantBranch)
			}
		})
	}
}

func TestRawURL(t *testing.T) {
	ref := repoRef{Owner: "owner", Repo: "repo", Branch: "main"}
	want := rawBaseURL + "/owner/repo/main/packages/cli/bat.yaml"

	// Leading slashes must not produce a doubled separator.
	for _, path := range []string{"packages/cli/bat.yaml", "/packages/cli/bat.yaml"} {
		if got := rawURL(ref, path); got != want {
			t.Errorf("rawURL(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestAuthFailure(t *testing.T) {
	// GitHub answers an invalid token on raw and codeload with 404, not 401,
	// so 404 has to count as a possible credentials problem.
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if !authFailure(status) {
			t.Errorf("authFailure(%d) = false, want true", status)
		}
	}
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadGateway} {
		if authFailure(status) {
			t.Errorf("authFailure(%d) = true, want false", status)
		}
	}
}

func TestFetchRetriesWithoutToken(t *testing.T) {
	var attempts int

	// Imitates a public repository with an expired token configured: the
	// authenticated request 404s, the anonymous one succeeds.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, "public content")
	}))
	defer server.Close()

	config := &cnfg.Config{Registry: cnfg.RegistryConfig{Token: "expired-token"}}

	body, err := doGet(server.URL, config)
	if err != nil {
		t.Fatalf("doGet() error = %v, want the anonymous retry to succeed", err)
	}
	if string(body) != "public content" {
		t.Errorf("body = %q, want %q", body, "public content")
	}
	if attempts != 2 {
		t.Errorf("made %d requests, want 2 (authenticated then anonymous)", attempts)
	}
}

func TestFetchDoesNotRetryWithoutAToken(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := &cnfg.Config{}
	if _, err := doGet(server.URL, config); err == nil {
		t.Fatal("doGet() error = nil, want a 404 to be reported")
	}
	if attempts != 1 {
		t.Errorf("made %d requests, want 1 — there is no token to retry without", attempts)
	}
}

func TestDoGetReportsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "boom")
	}))
	defer server.Close()

	_, err := doGet(server.URL, &cnfg.Config{})
	if err == nil {
		t.Fatal("doGet() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to include the status and the body", err)
	}
}

func TestParsePackages(t *testing.T) {
	index := IndexFile{Packages: []string{
		"packages/cli/bat.yaml",
		"packages/file_managers/yazi.yaml",
	}}
	files := map[string][]byte{
		"packages/cli/bat.yaml":            []byte("name: bat\nversion: v0.25.0\n"),
		"packages/file_managers/yazi.yaml": []byte("name: yazi\nversion: v25.4.8\n"),
	}

	packages, err := parsePackages(index, files)
	if err != nil {
		t.Fatalf("parsePackages() error = %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(packages))
	}
	// Index order is the display order, and the category comes from the
	// directory the file sits in.
	if packages[0].Name != "bat" || packages[0].Category != "cli" {
		t.Errorf("packages[0] = %+v, want bat in category cli", packages[0])
	}
	if packages[1].Category != "file_managers" {
		t.Errorf("packages[1].Category = %q, want file_managers", packages[1].Category)
	}
}

func TestParsePackagesReportsSkipped(t *testing.T) {
	index := IndexFile{Packages: []string{
		"packages/cli/good.yaml",
		"packages/cli/missing.yaml",
		"packages/cli/broken.yaml",
		"packages/cli/nameless.yaml",
	}}
	files := map[string][]byte{
		"packages/cli/good.yaml":     []byte("name: good\n"),
		"packages/cli/broken.yaml":   []byte("name: [unterminated"),
		"packages/cli/nameless.yaml": []byte("version: v1.0.0\n"),
	}

	packages, err := parsePackages(index, files)
	if len(packages) != 1 {
		t.Fatalf("got %d packages, want just the good one", len(packages))
	}
	// The old code dropped bad entries silently; now the caller is told, so a
	// partial result is never cached as if it were complete.
	if err == nil {
		t.Fatal("parsePackages() error = nil, want the skipped files to be reported")
	}
	if !strings.Contains(err.Error(), "skipped 3") {
		t.Errorf("error = %v, want it to count the 3 skipped files", err)
	}
}

func TestParsePackagesNoneValid(t *testing.T) {
	index := IndexFile{Packages: []string{"packages/cli/missing.yaml"}}
	if _, err := parsePackages(index, nil); err == nil {
		t.Error("parsePackages() error = nil, want an error when nothing parsed")
	}
}

// tarball builds a gzipped tar in the layout GitHub serves, where every entry is
// prefixed with "<repo>-<branch>/".
func tarball(t *testing.T, prefix string, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// A directory entry, to check that non-regular entries are skipped.
	if err := tw.WriteHeader(&tar.Header{
		Name: prefix + "/packages/", Typeflag: tar.TypeDir, Mode: 0o755,
	}); err != nil {
		t.Fatal(err)
	}

	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: prefix + "/" + name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// registryFiles is the smallest registry that exercises the index and both
// package categories.
var registryFiles = map[string]string{
	"index.yaml": "packages:\n  - packages/cli/bat.yaml\n  - packages/file_managers/yazi.yaml\n",
	"packages/cli/bat.yaml": "name: bat\nversion: v0.25.0\ncommit: aaaa\n" +
		"description: A cat clone\ninstall:\n  source:\n    url: https://example.com/bat.git\n",
	"packages/file_managers/yazi.yaml": "name: yazi\nversion: v25.4.8\ncommit: bbbb\n",
	"README.md":                        "ignored, not yaml",
}

// serveRegistry stands in for codeload and raw.githubusercontent.com, and
// points the package's base URLs at itself for the duration of the test.
// codeloadStatus lets a test force the tarball path to fail.
func serveRegistry(t *testing.T, codeloadStatus int) *httptest.Server {
	t.Helper()

	archive := tarball(t, "registry-main", registryFiles)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tar.gz/") {
			if codeloadStatus != http.StatusOK {
				w.WriteHeader(codeloadStatus)
				return
			}
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(archive)
			return
		}

		// Raw file access: /owner/repo/branch/<path>
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		content, ok := registryFiles[parts[3]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, content)
	}))

	oldRaw, oldCodeload := rawBaseURL, codeloadBaseURL
	rawBaseURL, codeloadBaseURL = server.URL, server.URL
	t.Cleanup(func() {
		rawBaseURL, codeloadBaseURL = oldRaw, oldCodeload
		server.Close()
	})

	return server
}

func TestFetchRegistryFromTarball(t *testing.T) {
	serveRegistry(t, http.StatusOK)
	config := testConfig(t)

	packages, err := FetchRegistry(config)
	if err != nil {
		t.Fatalf("FetchRegistry() error = %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(packages))
	}
	if packages[0].Name != "bat" || packages[0].Category != "cli" {
		t.Errorf("packages[0] = %+v, want bat in cli", packages[0])
	}
	if packages[0].Install.Source.URL != "https://example.com/bat.git" {
		t.Errorf("install.source.url = %q, want it to be parsed", packages[0].Install.Source.URL)
	}
	if packages[1].Category != "file_managers" {
		t.Errorf("packages[1].Category = %q, want file_managers", packages[1].Category)
	}
}

func TestFetchRegistryCountsRequests(t *testing.T) {
	archive := tarball(t, "registry-main", registryFiles)

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write(archive)
	}))
	defer server.Close()

	oldRaw, oldCodeload := rawBaseURL, codeloadBaseURL
	rawBaseURL, codeloadBaseURL = server.URL, server.URL
	defer func() { rawBaseURL, codeloadBaseURL = oldRaw, oldCodeload }()

	if _, err := FetchRegistry(testConfig(t)); err != nil {
		t.Fatalf("FetchRegistry() error = %v", err)
	}

	// The whole point of the tarball path: one request for the entire registry
	// instead of two per file.
	if requests != 1 {
		t.Errorf("made %d requests, want exactly 1", requests)
	}
}

func TestFetchRegistryFallsBackToRawFiles(t *testing.T) {
	serveRegistry(t, http.StatusInternalServerError)
	config := testConfig(t)

	packages, err := FetchRegistry(config)
	if err != nil {
		t.Fatalf("FetchRegistry() error = %v, want the raw fallback to succeed", err)
	}
	if len(packages) != 2 {
		t.Errorf("got %d packages via the fallback, want 2", len(packages))
	}
}

func TestFetchRegistryFailsWhenIndexIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	oldRaw, oldCodeload := rawBaseURL, codeloadBaseURL
	rawBaseURL, codeloadBaseURL = server.URL, server.URL
	defer func() { rawBaseURL, codeloadBaseURL = oldRaw, oldCodeload }()

	if _, err := FetchRegistry(testConfig(t)); err == nil {
		t.Error("FetchRegistry() error = nil, want an error when nothing can be fetched")
	}
}

func TestRefreshRegistryDoesNotCachePartialFetches(t *testing.T) {
	// The index lists two packages but only one is served. The old code cached
	// the partial result, so the missing package vanished for 24 hours.
	partial := map[string]string{
		"index.yaml":            registryFiles["index.yaml"],
		"packages/cli/bat.yaml": registryFiles["packages/cli/bat.yaml"],
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tar.gz/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		content, ok := partial[parts[3]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, content)
	}))
	defer server.Close()

	oldRaw, oldCodeload := rawBaseURL, codeloadBaseURL
	rawBaseURL, codeloadBaseURL = server.URL, server.URL
	defer func() { rawBaseURL, codeloadBaseURL = oldRaw, oldCodeload }()

	config := testConfig(t)
	packages, err := RefreshRegistry(config)

	if len(packages) != 1 {
		t.Fatalf("got %d packages, want the one that could be fetched", len(packages))
	}
	if err == nil {
		t.Error("RefreshRegistry() error = nil, want the partial fetch to be reported")
	}
	if exists(GetCacheFilePath(config)) {
		t.Error("a partial fetch was written to the cache")
	}
}

func TestLoadAllPackagesFromRegistryPrefersTheCache(t *testing.T) {
	config := testConfig(t)

	if err := SaveToCache([]*Package{{Name: "cached-only"}}, config); err != nil {
		t.Fatal(err)
	}

	// No server is running: if the cache were bypassed this would fail.
	packages, err := LoadAllPackagesFromRegistry(config)
	if err != nil {
		t.Fatalf("LoadAllPackagesFromRegistry() error = %v", err)
	}
	if len(packages) != 1 || packages[0].Name != "cached-only" {
		t.Errorf("got %v, want the cached package", packages)
	}
}

func TestRefreshRegistryWritesTheCache(t *testing.T) {
	serveRegistry(t, http.StatusOK)
	config := testConfig(t)

	if _, err := RefreshRegistry(config); err != nil {
		t.Fatalf("RefreshRegistry() error = %v", err)
	}

	cached, err := LoadFromCache(config)
	if err != nil {
		t.Fatalf("LoadFromCache() after refresh error = %v", err)
	}
	if len(cached) != 2 {
		t.Errorf("got %d cached packages, want 2", len(cached))
	}
}

func TestLoadPackageFromRegistry(t *testing.T) {
	serveRegistry(t, http.StatusOK)
	config := testConfig(t)

	p, err := LoadPackageFromRegistry("yazi", config)
	if err != nil {
		t.Fatalf("LoadPackageFromRegistry() error = %v", err)
	}
	if p.Name != "yazi" {
		t.Errorf("Name = %q, want yazi", p.Name)
	}

	if _, err := LoadPackageFromRegistry("nonexistent", config); err == nil {
		t.Error("LoadPackageFromRegistry(nonexistent) error = nil, want an error")
	}
}

func TestFetchTarballRejectsNonArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "this is not gzip")
	}))
	defer server.Close()

	old := codeloadBaseURL
	codeloadBaseURL = server.URL
	defer func() { codeloadBaseURL = old }()

	ref := repoRef{Owner: "owner", Repo: "repo", Branch: "main"}
	if _, err := fetchTarball(ref, &cnfg.Config{}); err == nil {
		t.Error("fetchTarball() error = nil, want a gzip error")
	}
}

func TestFetchRawFilesReportsFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "ok.yaml") {
			fmt.Fprint(w, "name: ok\n")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	old := rawBaseURL
	rawBaseURL = server.URL
	defer func() { rawBaseURL = old }()

	ref := repoRef{Owner: "owner", Repo: "repo", Branch: "main"}
	files, err := fetchRawFiles(ref, []string{"ok.yaml", "missing.yaml"}, &cnfg.Config{})

	// Whatever was fetched is still returned, alongside the error.
	if len(files) != 1 {
		t.Errorf("got %d files, want the one that succeeded", len(files))
	}
	if err == nil {
		t.Fatal("fetchRawFiles() error = nil, want the failure to be reported")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("error = %v, want it to say how many files failed", err)
	}
}
