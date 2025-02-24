package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/utils"
	"gopkg.in/yaml.v3"
)

const (
	registryRepoURL = "https://api.github.com/repos/lvim-tech/clipack-registry/contents"
)

type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Sha         string `json:"sha"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Message     string `json:"message"`
}

type IndexFile struct {
	Packages []string `yaml:"packages"`
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
			DisableKeepAlives:   false,
			MaxIdleConnsPerHost: 10,
		},
	}
}

func fetchGitHubFile(path string) (string, error) {
	client := newHTTPClient()

	url := registryRepoURL + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	req.Header.Set("User-Agent", "Clipack-Package-Manager")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetching file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error fetching file: status %d, body: %s", resp.StatusCode, string(body))
	}

	var content GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return "", fmt.Errorf("error decoding response: %v", err)
	}

	if content.DownloadURL == "" {
		return "", fmt.Errorf("no download URL available for %s", path)
	}

	req, err = http.NewRequest("GET", content.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating raw content request: %v", err)
	}

	req.Header.Set("User-Agent", "Clipack-Package-Manager")

	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetching raw content: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error fetching raw content: status %d, body: %s", resp.StatusCode, string(body))
	}

	rawContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading raw content: %v", err)
	}

	return string(rawContent), nil
}

func LoadAllPackagesFromRegistry(config *cnfg.Config) ([]*Package, error) {
	if err := utils.EnsureDirectoryExists(config.Paths.Registry); err != nil {
		return nil, fmt.Errorf("error creating registry directory: %v", err)
	}

	packages, err := LoadFromCache(config)
	if err == nil {
		return packages, nil
	}

	indexContent, err := fetchGitHubFile("/index.yaml")
	if err != nil {
		return nil, fmt.Errorf("error fetching index: %v", err)
	}

	var index IndexFile
	if err := yaml.Unmarshal([]byte(indexContent), &index); err != nil {
		return nil, fmt.Errorf("error parsing index.yaml: %v\n Content: %s", err, indexContent)
	}

	if index.Packages == nil || len(index.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in index.yaml")
	}

	var pkgs []*Package
	for _, pkgPath := range index.Packages {
		content, err := fetchGitHubFile("/" + pkgPath)
		if err != nil {
			continue
		}

		var pkg Package
		if err := yaml.Unmarshal([]byte(content), &pkg); err != nil {
			continue
		}

		parts := strings.Split(pkgPath, "/")
		if len(parts) >= 3 {
			pkg.Category = parts[1]
		}

		if pkg.Name == "" {
			continue
		}

		pkgs = append(pkgs, &pkg)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no valid packages found in registry")
	}

	if err := SaveToCache(pkgs, config); err != nil {
	}

	return pkgs, nil
}

func LoadPackageFromRegistry(name string, config *cnfg.Config) (*Package, error) {
	packages, err := LoadFromCache(config)
	if err == nil {
		for _, pkg := range packages {
			if pkg.Name == name {
				return pkg, nil
			}
		}
	}

	indexContent, err := fetchGitHubFile("/index.yaml")
	if err != nil {
		return nil, fmt.Errorf("error fetching index: %v", err)
	}

	var index IndexFile
	if err := yaml.Unmarshal([]byte(indexContent), &index); err != nil {
		return nil, fmt.Errorf("error parsing index.yaml: %v", err)
	}

	var pkgPath string
	for _, path := range index.Packages {
		if strings.HasSuffix(path, "/"+name+".yaml") {
			pkgPath = path
			break
		}
	}

	if pkgPath == "" {
		return nil, fmt.Errorf("package %s not found in registry", name)
	}

	content, err := fetchGitHubFile("/" + pkgPath)
	if err != nil {
		return nil, fmt.Errorf("error fetching package: %v", err)
	}

	var pkg Package
	if err := yaml.Unmarshal([]byte(content), &pkg); err != nil {
		return nil, fmt.Errorf("error parsing package YAML: %v", err)
	}

	parts := strings.Split(pkgPath, "/")
	if len(parts) >= 3 {
		pkg.Category = parts[1]
	}

	return &pkg, nil
}
