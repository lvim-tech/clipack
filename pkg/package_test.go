package pkg

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCloneURLPrefersDeclaredSource(t *testing.T) {
	p := &Package{
		Install: Install{
			Source: Source{URL: "https://example.com/declared.git"},
			Steps:  []string{"git clone https://example.com/from-step.git ."},
		},
	}

	if got := p.CloneURL(); got != "https://example.com/declared.git" {
		t.Errorf("CloneURL() = %q, want the declared source", got)
	}
}

func TestCloneURLFromStep(t *testing.T) {
	// The old implementation took strings.Split(step, " ")[2] unconditionally,
	// so any flag before the URL produced a bogus repository name.
	tests := []struct {
		name string
		step string
		want string
	}{
		{
			name: "plain clone",
			step: "git clone https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			name: "depth flag with value",
			step: "git clone --depth 1 https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			name: "branch flag with value",
			step: "git clone --branch v1.0 https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			name: "valueless flag",
			step: "git clone --recursive https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			name: "several flags",
			step: "git clone --depth 1 --single-branch --recursive https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			// -c takes its value as a separate field, and that value carries
			// no leading dash — the scan used to hand it back as the URL.
			name: "config flag with value",
			step: "git clone -c http.sslVerify=false https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			// A value-taking flag nobody listed: the URL still wins because it
			// is recognisable as one.
			name: "unknown flag with value",
			step: "git clone --made-up whatever=1 https://github.com/sharkdp/bat.git .",
			want: "https://github.com/sharkdp/bat.git",
		},
		{
			name: "scp style remote",
			step: "git clone git@github.com:sharkdp/bat.git .",
			want: "git@github.com:sharkdp/bat.git",
		},
		{
			name: "not a clone",
			step: "cargo build --release",
			want: "",
		},
		{
			name: "too short",
			step: "git clone",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cloneURLFromStep(tt.step); got != tt.want {
				t.Errorf("cloneURLFromStep(%q) = %q, want %q", tt.step, got, tt.want)
			}
		})
	}
}

func TestCloneURLNoSource(t *testing.T) {
	p := &Package{Install: Install{Steps: []string{"cargo build --release"}}}
	if got := p.CloneURL(); got != "" {
		t.Errorf("CloneURL() = %q, want empty", got)
	}
}

func TestRef(t *testing.T) {
	p := &Package{Version: "v1.2.3", Commit: "abcdef123456"}

	if got := p.Ref(MethodVersion); got != "v1.2.3" {
		t.Errorf("Ref(version) = %q, want v1.2.3", got)
	}
	if got := p.Ref(MethodCommit); got != "abcdef123456" {
		t.Errorf("Ref(commit) = %q, want abcdef123456", got)
	}
	// An unknown method falls back to the version rather than returning "".
	if got := p.Ref("nonsense"); got != "v1.2.3" {
		t.Errorf("Ref(nonsense) = %q, want v1.2.3", got)
	}
}

func TestMatches(t *testing.T) {
	p := &Package{
		Name:        "bat",
		Description: "A cat(1) clone with syntax highlighting",
		Category:    "cli",
		Tags:        []string{"cat", "syntax-highlighting"},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"bat", true},
		{"BAT", true},
		{"syntax", true},
		{"cli", true},
		{"highlighting", true},
		{"nothing-like-this", false},
	}

	for _, tt := range tests {
		if got := p.Matches(tt.query); got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestFindByName(t *testing.T) {
	packages := []*Package{{Name: "bat"}, {Name: "fzf"}}

	if got := FindByName(packages, "fzf"); got == nil || got.Name != "fzf" {
		t.Errorf("FindByName(fzf) = %v, want the fzf package", got)
	}
	if got := FindByName(packages, "missing"); got != nil {
		t.Errorf("FindByName(missing) = %v, want nil", got)
	}
	if got := FindByName(nil, "bat"); got != nil {
		t.Errorf("FindByName(nil, bat) = %v, want nil", got)
	}
}

func TestHasUpdate(t *testing.T) {
	tests := []struct {
		name      string
		registry  *Package
		installed *Package
		want      bool
	}{
		{
			name:      "same version",
			registry:  &Package{Version: "v1.0.0"},
			installed: &Package{Version: "v1.0.0", InstallMethod: MethodVersion},
			want:      false,
		},
		{
			name:      "newer version",
			registry:  &Package{Version: "v1.1.0"},
			installed: &Package{Version: "v1.0.0", InstallMethod: MethodVersion},
			want:      true,
		},
		{
			// A package pinned to a commit must be compared by commit, even
			// when the version string happens to match.
			name:      "same version different commit",
			registry:  &Package{Version: "v1.0.0", Commit: "bbbb"},
			installed: &Package{Version: "v1.0.0", Commit: "aaaa", InstallMethod: MethodCommit},
			want:      true,
		},
		{
			name:      "commit method matching commit",
			registry:  &Package{Version: "v9.9.9", Commit: "aaaa"},
			installed: &Package{Version: "v1.0.0", Commit: "aaaa", InstallMethod: MethodCommit},
			want:      false,
		},
		{
			// An old manifest may have no install_method recorded.
			name:      "empty method defaults to version",
			registry:  &Package{Version: "v1.1.0"},
			installed: &Package{Version: "v1.0.0"},
			want:      true,
		},
		{
			name:      "nil installed",
			registry:  &Package{Version: "v1.0.0"},
			installed: nil,
			want:      false,
		},
		{
			name:      "nil registry",
			registry:  nil,
			installed: &Package{Version: "v1.0.0"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasUpdate(tt.registry, tt.installed); got != tt.want {
				t.Errorf("HasUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadPackageFromBytes(t *testing.T) {
	// Mirrors the shape of a real registry file, including install.source,
	// which the old Package type had no field for and silently dropped.
	data := []byte(`
name: duf
version: v0.8.1
commit: ae480f3d59342a8963ffb7b4a5070a32086314fb
description: Disk Usage/Free Utility
homepage: https://github.com/muesli/duf
license: MIT
maintainer: muesli
updated_at: 2023-07-05T07:11:23Z
tags:
  - cli
  - disk
install:
  source:
    type: git
    url: https://github.com/muesli/duf.git
    ref: main
  environment:
    CGO_ENABLED: "0"
  steps:
    - git clone https://github.com/muesli/duf.git .
    - go build -o bin/duf
  binaries:
    - bin/duf
  man:
    - man/man1/duf.1
  additional-config:
    - filename: config.sh
      content: |
        echo hello
post-install:
  scripts:
    - filename: setup.sh
      content: "echo setup"
`)

	p, err := LoadPackageFromBytes(data)
	if err != nil {
		t.Fatalf("LoadPackageFromBytes() error = %v", err)
	}

	if p.Name != "duf" {
		t.Errorf("Name = %q, want duf", p.Name)
	}
	if p.Install.Source.URL != "https://github.com/muesli/duf.git" {
		t.Errorf("install.source.url = %q, want the duf repository", p.Install.Source.URL)
	}
	if p.Install.Environment["CGO_ENABLED"] != "0" {
		t.Errorf("install.environment = %v, want CGO_ENABLED=0", p.Install.Environment)
	}
	if len(p.Install.Steps) != 2 {
		t.Errorf("len(steps) = %d, want 2", len(p.Install.Steps))
	}
	if len(p.Install.AdditionalConfig) != 1 || p.Install.AdditionalConfig[0].Filename != "config.sh" {
		t.Errorf("additional-config = %v, want one config.sh entry", p.Install.AdditionalConfig)
	}
	if len(p.PostInstall.Scripts) != 1 || p.PostInstall.Scripts[0].Filename != "setup.sh" {
		t.Errorf("post-install.scripts = %v, want one setup.sh entry", p.PostInstall.Scripts)
	}
	if want := time.Date(2023, 7, 5, 7, 11, 23, 0, time.UTC); !p.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", p.UpdatedAt, want)
	}
}

func TestLoadPackageFromBytesInvalid(t *testing.T) {
	if _, err := LoadPackageFromBytes([]byte("name: [unterminated")); err == nil {
		t.Error("LoadPackageFromBytes() error = nil, want a parse error")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte("contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The destination directory does not exist yet; CopyFile must create it.
	dst := filepath.Join(dir, "nested", "deeper", "dest.txt")
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if string(got) != "contents" {
		t.Errorf("destination contents = %q, want %q", got, "contents")
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("destination mode = %v, want 0755 preserved from the source", info.Mode().Perm())
	}
}

func TestCopyFileOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A longer existing file: without O_TRUNC the tail would survive.
	if err := os.WriteFile(dst, []byte("much longer old contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "new" {
		t.Errorf("destination contents = %q, want %q", got, "new")
	}
}

func TestCopyFileErrors(t *testing.T) {
	dir := t.TempDir()

	if err := CopyFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst")); err == nil {
		t.Error("CopyFile(missing source) error = nil, want an error")
	}

	// Directories are not regular files and must be rejected explicitly.
	if err := CopyFile(dir, filepath.Join(dir, "dst")); err == nil {
		t.Error("CopyFile(directory) error = nil, want an error")
	}
}

func TestLoadInstalledPackages(t *testing.T) {
	config := testConfig(t)

	writeManifest(t, config.Paths.Configs, "bat", "name: bat\nversion: v0.25.0\ninstall_method: version\n")
	writeManifest(t, config.Paths.Configs, "fzf", "name: fzf\nversion: v0.62.0\ninstall_method: commit\n")

	// A directory without a manifest, and one with unparseable YAML: both are
	// skipped rather than failing the whole listing.
	if err := os.MkdirAll(filepath.Join(config.Paths.Configs, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, config.Paths.Configs, "broken", "name: [unterminated")

	installed, err := LoadInstalledPackages(config)
	if err != nil {
		t.Fatalf("LoadInstalledPackages() error = %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("got %d installed packages, want 2 (bat and fzf)", len(installed))
	}
}

func TestLoadInstalledPackagesMissingDirectory(t *testing.T) {
	config := testConfig(t)
	config.Paths.Configs = filepath.Join(t.TempDir(), "does-not-exist")

	// "Nothing installed yet" is a normal state, not an error.
	installed, err := LoadInstalledPackages(config)
	if err != nil {
		t.Errorf("LoadInstalledPackages() error = %v, want nil", err)
	}
	if len(installed) != 0 {
		t.Errorf("got %d packages, want 0", len(installed))
	}
}

func TestInstalledMap(t *testing.T) {
	config := testConfig(t)
	writeManifest(t, config.Paths.Configs, "bat", "name: bat\nversion: v0.25.0\n")

	m, err := InstalledMap(config)
	if err != nil {
		t.Fatalf("InstalledMap() error = %v", err)
	}
	if m["bat"] == nil || m["bat"].Version != "v0.25.0" {
		t.Errorf("InstalledMap()[bat] = %v, want the bat package", m["bat"])
	}
	if _, ok := m["missing"]; ok {
		t.Error("InstalledMap() contains an entry for a package that is not installed")
	}
}

// A manifest outlives what it describes. These are the shapes that produced a
// package clipack called installed while nothing of it was on disk.
func TestMissingArtifacts(t *testing.T) {
	config := testConfig(t)

	p := &Package{
		Name: "demo",
		Install: Install{
			Binaries:  []string{"out/demo", "out/demo-helper"},
			Resources: []Resource{{Source: "out/lib/demo", Target: "lib/demo"}},
		},
	}

	// Nothing installed at all.
	if got := p.MissingArtifacts(config); len(got) != 3 {
		t.Fatalf("got %d missing, want 3: %v", len(got), got)
	}
	if p.IsIntact(config) {
		t.Error("IsIntact() = true with nothing on disk")
	}

	write := func(rel string) {
		t.Helper()
		path := filepath.Join(config.Paths.Bin, rel)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("demo")
	write("demo-helper")
	if err := os.MkdirAll(filepath.Join(config.Paths.Base, "lib", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := p.MissingArtifacts(config); len(got) != 0 {
		t.Errorf("got %v missing, want none", got)
	}
	if !p.IsIntact(config) {
		t.Error("IsIntact() = false with everything on disk")
	}
}

// The case that started this: a stale unix socket sitting where the binary
// belongs. os.Stat finds it, so an existence check alone says installed — but
// it is not a program, and PATH skips it without a word.
func TestMissingArtifactsRejectsANonRegularFile(t *testing.T) {
	config := testConfig(t)
	p := &Package{Name: "tmux", Install: Install{Binaries: []string{"tmux"}}}

	sock := filepath.Join(config.Paths.Bin, "tmux")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("cannot create a unix socket here: %v", err)
	}
	defer l.Close()

	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("the socket was not created: %v", err)
	}
	if got := p.MissingArtifacts(config); len(got) != 1 {
		t.Errorf("got %v, want the socket reported as missing", got)
	}
}

// A resource target that is a FILE where a directory belongs is broken too.
func TestMissingArtifactsRejectsAFileWhereATreeBelongs(t *testing.T) {
	config := testConfig(t)
	p := &Package{
		Name:    "demo",
		Install: Install{Resources: []Resource{{Source: "out/x", Target: "lib/demo"}}},
	}

	if err := os.MkdirAll(filepath.Join(config.Paths.Base, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.Paths.Base, "lib", "demo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := p.MissingArtifacts(config); len(got) != 1 {
		t.Errorf("got %v, want the file reported as missing", got)
	}
}
