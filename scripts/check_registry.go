package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
    "io/ioutil"

    "github.com/google/go-github/v41/github"
    "golang.org/x/oauth2"
    "gopkg.in/yaml.v2"
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
    ownerRepo := strings.TrimPrefix(pkg.Homepage, "https://github.com/")
    ownerRepo = strings.TrimSuffix(ownerRepo, ".git")
    parts := strings.Split(ownerRepo, "/")
    if len(parts) != 2 {
        return pkg.Version, pkg.Commit, fmt.Errorf("invalid repository URL: %s", pkg.Homepage)
    }
    owner, repo := parts[0], parts[1]

    // Check for the latest release
    release, _, err := client.Repositories.GetLatestRelease(context.Background(), owner, repo)
    if err != nil {
        // If there is no release, fall back to getting the latest tag
        tags, _, err := client.Repositories.ListTags(context.Background(), owner, repo, nil)
        if err != nil || len(tags) == 0 {
            return pkg.Version, pkg.Commit, fmt.Errorf("error getting latest release or tag: %v", err)
        }
        newVersion := tags[0].GetName()
        // Check for the latest commit
        commits, _, err := client.Repositories.ListCommits(context.Background(), owner, repo, nil)
        if err != nil {
            return pkg.Version, pkg.Commit, fmt.Errorf("error getting commits: %v", err)
        }
        newCommit := commits[0].GetSHA()
        return newVersion, newCommit, nil
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
    if len(os.Args) < 2 {
        log.Fatalf("Usage: %s <token>", os.Args[0])
    }
    token := os.Args[1]

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

    for _, file := range index.Packages {
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
            pkg.UpdatedAt = time.Now().Format(time.RFC3339)
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
        output, err := cmd.CombinedOutput()
        if err != nil {
            log.Fatalf("Error committing changes to git: %v, output: %s", err, output)
        }

        cmd = exec.Command("git", "push")
        cmd.Dir = "registry"
        output, err = cmd.CombinedOutput()
        if err != nil {
            log.Fatalf("Error pushing changes to git: %v, output: %s", err, output)
        }
    } else {
        fmt.Println("No updates found.")
    }
}
