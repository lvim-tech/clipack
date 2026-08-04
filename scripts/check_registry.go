// Command check_registry refreshes the version and commit of every package in
// the registry, then commits and pushes the result.
//
// It is run daily by .github/workflows/check-registry.yml, which clones
// clipack-registry into ./registry and passes a GitHub token as the only
// argument.
//
// Two properties matter more than they look:
//
//   - Package files are edited LINE BY LINE, not unmarshalled and marshalled
//     back. A round trip through a Go struct silently drops every field the
//     struct does not declare — which is how install.man, install.environment
//     and post-install would disappear the first time their package saw a new
//     release. It also destroys comments.
//
//   - A failure is a failure. An earlier version logged API errors and carried
//     on, so an expired token produced "No updates found" and exit 0. The
//     registry then sat unchanged for eight months while every scheduled run
//     reported success.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// registryDir is where the workflow clones clipack-registry.
const registryDir = "registry"

// Index is the registry's list of package files.
type Index struct {
	Packages []string `yaml:"packages"`
}

// Package is only ever used for READING. Writing goes through replaceField, so
// a field missing here cannot be lost.
type Package struct {
	Name     string `yaml:"name"`
	Version  string `yaml:"version"`
	Commit   string `yaml:"commit"`
	Homepage string `yaml:"homepage"`
}

func main() {
	// From the ENVIRONMENT, not from a command-line argument. An argument is
	// visible to every process on the machine through /proc and to anything
	// reading a process list — on a shared CI runner that is not a theoretical
	// audience. The first argument is still accepted so a local run can pass one.
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" && len(os.Args) > 1 {
		token = os.Args[1]
	}
	if token == "" {
		log.Fatal("set GITHUB_TOKEN (or pass a token as the first argument)")
	}

	ctx := context.Background()
	client := github.NewClient(oauth2.NewClient(ctx,
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})))

	// Fail before touching anything if the token is not usable. The absence of
	// this check is what hid an expired token for eight months.
	if _, _, err := client.Users.Get(ctx, ""); err != nil {
		log.Fatalf("the GitHub token is not usable: %v", err)
	}

	index, err := readIndex()
	if err != nil {
		log.Fatal(err)
	}

	var updated, failed []string
	for _, rel := range index.Packages {
		changed, err := updatePackage(ctx, client, rel)
		switch {
		case err != nil:
			log.Printf("%s: %v", rel, err)
			failed = append(failed, rel)
		case changed:
			updated = append(updated, rel)
		}
	}

	if len(updated) > 0 {
		if err := commitAndPush(updated); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("updated %d package(s): %s\n", len(updated), strings.Join(updated, ", "))
	} else {
		fmt.Println("no updates found")
	}

	// A partial run is reported as a failure, so the workflow goes red instead
	// of quietly falling behind.
	if len(failed) > 0 {
		log.Fatalf("%d package(s) could not be checked: %s",
			len(failed), strings.Join(failed, ", "))
	}
}

// readIndex loads the registry's package list.
func readIndex() (Index, error) {
	data, err := os.ReadFile(filepath.Join(registryDir, "index.yaml"))
	if err != nil {
		return Index{}, fmt.Errorf("reading index.yaml: %w", err)
	}

	var index Index
	if err := yaml.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("parsing index.yaml: %w", err)
	}
	if len(index.Packages) == 0 {
		return Index{}, fmt.Errorf("index.yaml lists no packages")
	}
	return index, nil
}

// updatePackage refreshes one package file, reporting whether it changed.
func updatePackage(ctx context.Context, client *github.Client, rel string) (bool, error) {
	path := filepath.Join(registryDir, rel)

	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading: %w", err)
	}

	var pkg Package
	if err := yaml.Unmarshal(raw, &pkg); err != nil {
		return false, fmt.Errorf("parsing: %w", err)
	}

	owner, repo, err := splitRepo(pkg.Homepage)
	if err != nil {
		return false, err
	}

	version, commit, err := latestRefs(ctx, client, owner, repo, pkg.Version)
	if err != nil {
		return false, err
	}
	if version == pkg.Version && commit == pkg.Commit {
		return false, nil
	}

	out := string(raw)
	if out, err = replaceField(out, "version", version); err != nil {
		return false, err
	}
	if out, err = replaceField(out, "commit", commit); err != nil {
		return false, err
	}
	if out, err = replaceField(out, "updated_at", quoted(time.Now().UTC().Format(time.RFC3339))); err != nil {
		return false, err
	}

	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, fmt.Errorf("writing: %w", err)
	}
	return true, nil
}

// latestRefs returns the newest release tag and the head of the default branch.
//
// version is the tag a stable install checks out; commit is what the "commit"
// install method follows, so it is deliberately ahead of the release.
func latestRefs(ctx context.Context, client *github.Client, owner, repo, current string) (string, string, error) {
	version := current

	release, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err == nil {
		version = release.GetTagName()
	} else {
		// Projects that tag without publishing releases still need a version.
		tags, _, tagErr := client.Repositories.ListTags(ctx, owner, repo, nil)
		if tagErr != nil {
			return "", "", fmt.Errorf("no release and no tags: %v / %v", err, tagErr)
		}
		if len(tags) > 0 {
			version = tags[0].GetName()
		}
	}

	commits, _, err := client.Repositories.ListCommits(ctx, owner, repo,
		&github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 1}})
	if err != nil {
		return "", "", fmt.Errorf("listing commits: %w", err)
	}
	if len(commits) == 0 {
		return "", "", fmt.Errorf("repository has no commits")
	}

	return version, commits[0].GetSHA(), nil
}

// splitRepo pulls owner and repository out of a GitHub URL.
func splitRepo(homepage string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(homepage, "/"), ".git")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[len(parts)-1] == "" {
		return "", "", fmt.Errorf("cannot read owner/repo from homepage %q", homepage)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

// replaceField rewrites one top-level scalar in place, leaving every other byte
// of the file — comments included, and fields this program knows nothing about
// — exactly as it was.
func replaceField(doc, field, value string) (string, error) {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(field) + `:.*$`)
	if !re.MatchString(doc) {
		return "", fmt.Errorf("no top-level %q field to update", field)
	}
	return re.ReplaceAllLiteralString(doc, field+": "+value), nil
}

// quoted wraps a timestamp so YAML keeps it a string.
func quoted(s string) string { return `"` + s + `"` }

// commitAndPush records the refreshed files in the registry repository.
func commitAndPush(updated []string) error {
	message := fmt.Sprintf("Update %d package(s): %s",
		len(updated), strings.Join(shortNames(updated), ", "))

	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", message},
		{"push"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = registryDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, out)
		}
	}
	return nil
}

// shortNames turns package paths into bare names for the commit subject.
func shortNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = strings.TrimSuffix(filepath.Base(p), ".yaml")
	}
	return out
}
