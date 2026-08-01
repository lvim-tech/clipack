package pkg

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCache(t *testing.T) {
	config := testConfig(t)

	packages := []*Package{
		{Name: "bat", Version: "v0.25.0", Category: "cli", Tags: []string{"cat"}},
		{Name: "yazi", Version: "v25.4.8", Category: "file_managers"},
	}

	if err := SaveToCache(packages, config); err != nil {
		t.Fatalf("SaveToCache() error = %v", err)
	}

	loaded, err := LoadFromCache(config)
	if err != nil {
		t.Fatalf("LoadFromCache() error = %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d cached packages, want 2", len(loaded))
	}
	if loaded[0].Name != "bat" || loaded[0].Category != "cli" {
		t.Errorf("first package = %+v, want bat/cli — order and category must survive", loaded[0])
	}
	if len(loaded[0].Tags) != 1 || loaded[0].Tags[0] != "cat" {
		t.Errorf("tags = %v, want [cat]", loaded[0].Tags)
	}
}

func TestSaveToCacheCreatesMissingDirectory(t *testing.T) {
	config := testConfig(t)

	// GetCacheFilePath deliberately does not create anything, so SaveToCache
	// has to cope with the registry directory being absent.
	if err := os.RemoveAll(config.Paths.Registry); err != nil {
		t.Fatal(err)
	}

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatalf("SaveToCache() error = %v", err)
	}
	if !exists(GetCacheFilePath(config)) {
		t.Error("the cache file was not written")
	}
}

func TestGetCacheFilePathHasNoSideEffects(t *testing.T) {
	config := testConfig(t)
	if err := os.RemoveAll(config.Paths.Registry); err != nil {
		t.Fatal(err)
	}

	// The old getter called log.Fatalf on failure, which would tear down the
	// TUI mid-render. It must be a pure path join.
	_ = GetCacheFilePath(config)

	if exists(config.Paths.Registry) {
		t.Error("GetCacheFilePath created the registry directory as a side effect")
	}
}

func TestLoadFromCacheStale(t *testing.T) {
	config := testConfig(t)
	config.Registry.UpdateInterval = time.Nanosecond

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)

	_, err := LoadFromCache(config)
	if !errors.Is(err, ErrCacheStale) {
		t.Errorf("LoadFromCache() error = %v, want ErrCacheStale", err)
	}
}

func TestLoadFromCacheMissing(t *testing.T) {
	config := testConfig(t)
	if _, err := LoadFromCache(config); err == nil {
		t.Error("LoadFromCache() error = nil, want an error for a missing cache")
	}
}

func TestLoadFromCacheCorrupt(t *testing.T) {
	config := testConfig(t)
	if err := os.WriteFile(GetCacheFilePath(config), []byte("not a gob stream"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadFromCache(config); err == nil {
		t.Error("LoadFromCache() error = nil, want a decode error")
	}
}

func TestLoadFromCacheEmptyIsStale(t *testing.T) {
	config := testConfig(t)

	// An empty cache is indistinguishable from a broken fetch, so it must not
	// be served as "the registry has no packages".
	if err := SaveToCache(nil, config); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromCache(config); !errors.Is(err, ErrCacheStale) {
		t.Errorf("LoadFromCache() error = %v, want ErrCacheStale for an empty cache", err)
	}
}

func TestLoadFromCacheDefaultsInterval(t *testing.T) {
	config := testConfig(t)
	config.Registry.UpdateInterval = 0

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}

	// A zero interval must fall back to the 24h default rather than treating
	// every cache as instantly stale.
	if _, err := LoadFromCache(config); err != nil {
		t.Errorf("LoadFromCache() error = %v, want the default interval to apply", err)
	}
}

func TestSaveToCacheRemovesLegacyTimestampFile(t *testing.T) {
	config := testConfig(t)

	legacy := GetCacheTimestampFilePath(config)
	if err := os.WriteFile(legacy, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}
	if exists(legacy) {
		t.Error("the redundant timestamp sidecar from older versions was not cleaned up")
	}
}

func TestSaveToCacheLeavesNoTempFiles(t *testing.T) {
	config := testConfig(t)

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(config.Paths.Registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temporary file %q was left behind by the atomic write", entry.Name())
		}
	}
}

func TestCacheAge(t *testing.T) {
	config := testConfig(t)

	if _, err := CacheAge(config); err == nil {
		t.Error("CacheAge() error = nil for a missing cache, want an error")
	}

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}

	age, err := CacheAge(config)
	if err != nil {
		t.Fatalf("CacheAge() error = %v", err)
	}
	if age < 0 || age > time.Minute {
		t.Errorf("CacheAge() = %v, want a small positive duration", age)
	}
}

func TestClearCache(t *testing.T) {
	config := testConfig(t)

	if err := SaveToCache([]*Package{{Name: "bat"}}, config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GetCacheTimestampFilePath(config), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearCache(config); err != nil {
		t.Fatalf("ClearCache() error = %v", err)
	}
	if exists(GetCacheFilePath(config)) {
		t.Error("the cache file survived ClearCache")
	}

	// Clearing an already-clear cache is not an error.
	if err := ClearCache(config); err != nil {
		t.Errorf("ClearCache() on an empty cache error = %v, want nil", err)
	}
}
