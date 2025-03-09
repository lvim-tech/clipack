package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type Index struct {
	Packages []string `yaml:"packages"`
}

type Package struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Commit      string `yaml:"commit"`
	Description string `yaml:"description"`
	Homepage    string `yaml:"homepage"`
	License     string `yaml:"license"`
	Maintainer  string `yaml:"maintainer"`
	UpdatedAt   string `yaml:"updated_at"`
	Tags        []string `yaml:"tags"`
	Install     struct {
		Source struct {
			Type string `yaml:"type"`
			URL  string `yaml:"url"`
			Ref  string `yaml:"ref"`
		} `yaml:"source"`
		Steps []string `yaml:"steps"`
		Binaries []string `yaml:"binaries"`
		AdditionalConfig []struct {
			Filename string `yaml:"filename"`
			Content  string `yaml:"content"`
		} `yaml:"additional-config"`
	} `yaml:"install"`
}

func checkForNewVersionAndCommit(client *github.Client, pkg *Package) (string, string, error) {
	ownerRepo := strings.TrimPrefix(pkg.Install.Source.URL, "https://github.com/")
	ownerRepo = strings.TrimSuffix(ownerRepo, ".git") // премахване на .git
	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 {
		return pkg.Version, pkg.Commit, fmt.Errorf("invalid repository URL: %s", pkg.Install.Source.URL)
	}
	owner, repo := parts[0], parts[1]

	// Check for the latest release
	release, _, err := client.Repositories.GetLatestRelease(context.Background(), owner, repo)
	if err != nil {
		return pkg.Version, pkg.Commit, fmt.Errorf("error getting latest release: %v", err)
	}
	newVersion := release.GetTagName()

	// Check for the latest commit
	commits, _, err := client.Repositories.ListCommits(context.Background(), owner, repo, nil)
	if err != nil {
		return pkg.Version, pkg.Commit, fmt.Errorf("error getting commits: %v", err)
	}
	newCommit := commits[0].GetSHA()

	return newVersion, newCommit, nil
}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatalf("GITHUB_TOKEN environment variable is required")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Четене на index.yaml
	indexData, err := ioutil.ReadFile(filepath.Join("registry", "index.yaml"))
	if err != nil {
		log.Fatalf("Error reading index.yaml: %v", err)
	}

	var index Index
	err = yaml.Unmarshal(indexData, &index)
	if err != nil {
		log.Fatalf("Error unmarshalling index.yaml: %v", err)
	}

	updated := false

	for _, file := range index.PPackages {
		filePath := filepath.Join("registry", file)
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Error reading file %s: %v", filePath, err)
		}

		var pkg Package
		err = yaml.Unmarshal(data, &pkg)
		if err != nil {
			log.Fatalf("Error unmarshalling YAML for file %s: %v", filePath, err)
		}

		newVersion, newCommit, err := checkForNewVersionAndCommit(client, &pkg)
		if err != nil {
			log.Printf("Error checking for new version and commit for %s: %v", pkg.Name, err)
			continue
		}

		if newVersion != pkg.Version || newCommit != pkg.Commit {
			pkg.Version = newVersion
			pkg.Commit = newCommit
			updated = true

			data, err = yaml.Marshal(&pkg)
			if err != nil {
				log.Fatalf("Error marshalling YAML for file %s: %v", filePath, err)
			}

			err = ioutil.WriteFile(filePath, data, 0644)
			if err != nil {
				log.Fatalf("Error writing file %s: %v", filePath, err)
			}
		}
	}

	if updated {
		// Комитване на промените
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = "registry"
		err := cmd.Run()
		if err != nil {
			log.Fatalf("Error adding changes to git: %v", err)
		}

		cmd = exec.Command("git", "commit", "-m", "Automated registry update")
		cmd.Dir = "registry"
		err := cmd.Run()
		if err != nil {
			log.Fatalf("Error committing changes to git: %v", err)
		}

		cmd = exec.Command("git", "push")
		cmd.Dir = "registry"
		err := cmd.Run()
		if err != nil {
			log.Fatalf("Error pushing changes to git: %v", err)
		}
	} else {
		fmt.Println("No updates found.")
	}
}
