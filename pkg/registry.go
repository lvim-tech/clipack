package pkg

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
	"gopkg.in/yaml.v3"
)

// maxParallelFetches bounds concurrent HTTP requests in the per-file fallback.
const maxParallelFetches = 8

// Bounds on the registry archive. The compressed limit alone guards nothing:
// gzip expands, and 64 MiB of a compressible pattern inflates to hundreds of
// gigabytes, all of it read into memory by the tar reader. So the inflated
// stream is bounded too, and each retained file separately — a package
// definition is kilobytes of YAML.
const (
	maxTarballBytes     = 64 << 20  // 64 MiB, compressed
	maxInflatedBytes    = 512 << 20 // 512 MiB, decompressed
	maxRegistryFileSize = 4 << 20   // 4 MiB per package definition
)

// limitedReader fails the read once the total passes limit, rather than
// reporting a truncated stream the way io.LimitReader would: a registry that
// is too big and a registry that ends early call for different messages.
type limitedReader struct {
	r     io.Reader
	limit int64
	read  int64
	what  string
}

func (l *limitedReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
		return n, fmt.Errorf("%s exceeds %d bytes", l.what, l.limit)
	}
	return n, err
}

// Hosts the registry is read from. They are variables rather than constants so
// tests can point them at a local server; nothing else reassigns them.
var (
	rawBaseURL      = "https://raw.githubusercontent.com"
	codeloadBaseURL = "https://codeload.github.com"
)

// IndexFile represents the index file structure.
type IndexFile struct {
	Packages []string `yaml:"packages"`
}

var (
	httpClientOnce sync.Once
	httpClient     *http.Client
)

// sharedHTTPClient returns a process-wide client so connections are reused
// across the many requests a registry sync makes.
func sharedHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 32
		transport.MaxIdleConnsPerHost = maxParallelFetches
		transport.IdleConnTimeout = 60 * time.Second
		httpClient = &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		}
	})
	return httpClient
}

// repoRef identifies a GitHub repository and branch.
type repoRef struct {
	Owner  string
	Repo   string
	Branch string
}

// resolveRepo derives owner/repo/branch from the configured registry URLs.
func resolveRepo(config *cnfg.Config) (repoRef, error) {
	branch := config.Registry.Branch
	if branch == "" {
		branch = "main"
	}

	for _, raw := range []string{config.Registry.URL, config.Registry.RegistryRepoURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		// github.com/<owner>/<repo>[.git]
		// api.github.com/repos/<owner>/<repo>/contents
		if len(parts) >= 3 && parts[0] == "repos" {
			parts = parts[1:]
		}
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		return repoRef{
			Owner:  parts[0],
			Repo:   strings.TrimSuffix(parts[1], ".git"),
			Branch: branch,
		}, nil
	}

	return repoRef{}, errors.New("could not determine registry owner/repo from config (registry.url)")
}

// get issues a GET, sending the configured token when there is one.
func get(target, token string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Clipack-Package-Manager")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return sharedHTTPClient().Do(req)
}

// authFailure reports whether a status code suggests the credentials are the
// problem. GitHub answers an invalid token on raw.githubusercontent.com and
// codeload with 404 rather than 401, so a stale token looks exactly like a
// missing repository.
func authFailure(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusNotFound
}

// fetch performs a GET and, when a token is configured but the request fails in
// a way that looks credential-related, retries anonymously. Without this an
// expired token (in config.yaml or the environment) makes every public registry
// request fail.
func fetch(target string, config *cnfg.Config) (*http.Response, error) {
	token := config.RegistryToken()

	resp, err := get(target, token)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK || token == "" || !authFailure(resp.StatusCode) {
		return resp, nil
	}

	resp.Body.Close()
	return get(target, "")
}

// doGet fetches a URL and returns the whole body.
func doGet(target string, config *cnfg.Config) ([]byte, error) {
	resp, err := fetch(target, config)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s: status %d: %s", target, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return io.ReadAll(resp.Body)
}

// fetchTarball downloads the whole registry in a single request and returns
// repo-relative path -> contents. This replaces the previous approach of two
// GitHub API calls per package file, which needed ~43 round trips for a
// 21-package registry and burned through the 60/hour unauthenticated limit.
func fetchTarball(ref repoRef, config *cnfg.Config) (map[string][]byte, error) {
	target := fmt.Sprintf("%s/%s/%s/tar.gz/refs/heads/%s",
		codeloadBaseURL, ref.Owner, ref.Repo, ref.Branch)

	resp, err := fetch(target, config)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tarball fetch: status %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxTarballBytes))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	files := make(map[string][]byte)
	tr := tar.NewReader(&limitedReader{r: gz, limit: maxInflatedBytes, what: "registry archive"})
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := header.Name
		// Strip the "<repo>-<branch>/" prefix GitHub adds.
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxRegistryFileSize+1))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		if len(data) > maxRegistryFileSize {
			return nil, fmt.Errorf("%s exceeds %d bytes", name, maxRegistryFileSize)
		}
		files[name] = data
	}

	if len(files) == 0 {
		return nil, errors.New("tarball contained no yaml files")
	}
	return files, nil
}

// rawURL builds a raw.githubusercontent.com URL for a repo-relative path.
func rawURL(ref repoRef, filePath string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		rawBaseURL, ref.Owner, ref.Repo, ref.Branch, strings.TrimPrefix(filePath, "/"))
}

// fetchRawFiles downloads the given paths concurrently. Unlike the previous
// implementation it reports failures instead of silently dropping packages,
// so a flaky network can no longer poison the cache with a partial registry.
func fetchRawFiles(ref repoRef, paths []string, config *cnfg.Config) (map[string][]byte, error) {
	type result struct {
		path string
		data []byte
		err  error
	}

	results := make([]result, len(paths))
	sem := make(chan struct{}, maxParallelFetches)
	var wg sync.WaitGroup

	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := doGet(rawURL(ref, p), config)
			results[i] = result{path: p, data: data, err: err}
		}(i, p)
	}
	wg.Wait()

	files := make(map[string][]byte, len(paths))
	var failures []string
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, r.path)
			continue
		}
		files[r.path] = r.data
	}

	if len(failures) > 0 {
		return files, fmt.Errorf("failed to fetch %d of %d registry files (first: %s)",
			len(failures), len(paths), failures[0])
	}
	return files, nil
}

// parsePackages turns a path -> contents map into packages, in index order.
func parsePackages(index IndexFile, files map[string][]byte) ([]*Package, error) {
	var pkgs []*Package
	var skipped []string

	for _, pkgPath := range index.Packages {
		data, ok := files[strings.TrimPrefix(pkgPath, "/")]
		if !ok {
			skipped = append(skipped, pkgPath)
			continue
		}

		var pkg Package
		if err := yaml.Unmarshal(data, &pkg); err != nil {
			skipped = append(skipped, pkgPath)
			continue
		}
		if pkg.Name == "" {
			skipped = append(skipped, pkgPath)
			continue
		}

		// packages/<category>/<name>.yaml
		if parts := strings.Split(pkgPath, "/"); len(parts) >= 3 {
			pkg.Category = parts[len(parts)-2]
		}

		pkgs = append(pkgs, &pkg)
	}

	if len(pkgs) == 0 {
		return nil, errors.New("no valid packages found in registry")
	}
	if len(skipped) > 0 {
		return pkgs, fmt.Errorf("skipped %d unreadable package(s), first: %s", len(skipped), skipped[0])
	}
	return pkgs, nil
}

// FetchRegistry always talks to the network and returns the full package set.
func FetchRegistry(config *cnfg.Config) ([]*Package, error) {
	ref, err := resolveRepo(config)
	if err != nil {
		return nil, err
	}

	// Fast path: one request for the entire registry.
	if files, err := fetchTarball(ref, config); err == nil {
		var index IndexFile
		if data, ok := files["index.yaml"]; ok {
			if err := yaml.Unmarshal(data, &index); err == nil && len(index.Packages) > 0 {
				if pkgs, perr := parsePackages(index, files); len(pkgs) > 0 {
					return pkgs, perr
				}
			}
		}
	}

	// Fallback: index + one raw request per package, in parallel.
	indexData, err := doGet(rawURL(ref, "index.yaml"), config)
	if err != nil {
		return nil, fmt.Errorf("fetching index.yaml: %w", err)
	}

	var index IndexFile
	if err := yaml.Unmarshal(indexData, &index); err != nil {
		return nil, fmt.Errorf("parsing index.yaml: %w", err)
	}
	if len(index.Packages) == 0 {
		return nil, errors.New("no packages listed in index.yaml")
	}

	files, fetchErr := fetchRawFiles(ref, index.Packages, config)
	pkgs, parseErr := parsePackages(index, files)
	if len(pkgs) == 0 {
		if fetchErr != nil {
			return nil, fetchErr
		}
		return nil, parseErr
	}
	if fetchErr != nil {
		return pkgs, fetchErr
	}
	return pkgs, parseErr
}

// LoadAllPackagesFromRegistry returns the cached package set when it is still
// fresh, otherwise refreshes it from the network and caches the result.
// A partial fetch is returned to the caller but never written to the cache.
func LoadAllPackagesFromRegistry(config *cnfg.Config) ([]*Package, error) {
	if packages, err := LoadFromCache(config); err == nil && len(packages) > 0 {
		return packages, nil
	}
	return RefreshRegistry(config)
}

// RefreshRegistry bypasses the cache, fetches, and stores the result.
func RefreshRegistry(config *cnfg.Config) ([]*Package, error) {
	packages, fetchErr := FetchRegistry(config)
	if len(packages) == 0 {
		return nil, fetchErr
	}

	// Only cache a complete fetch; a partial one would be served as if it
	// were the whole registry until the interval expires.
	if fetchErr == nil {
		if err := SaveToCache(packages, config); err != nil {
			return packages, fmt.Errorf("packages loaded but caching failed: %w", err)
		}
	}

	return packages, fetchErr
}

// LoadPackageFromRegistry loads a single package by name.
func LoadPackageFromRegistry(name string, config *cnfg.Config) (*Package, error) {
	if packages, err := LoadFromCache(config); err == nil {
		if p := FindByName(packages, name); p != nil {
			return p, nil
		}
	}

	packages, err := RefreshRegistry(config)
	if p := FindByName(packages, name); p != nil {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("package %q not found in registry", name)
}
