package pkg

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
)

// ErrCacheStale is returned when the cache is missing or past its interval.
var ErrCacheStale = errors.New("registry cache is missing or outdated")

// PackageCache holds the cached packages and the last updated timestamp.
type PackageCache struct {
	Packages    []*Package
	LastUpdated time.Time
}

// GetCacheFilePath returns the path to the cache file. It no longer creates
// directories or calls log.Fatalf: a path getter must never terminate the
// process, which would tear down the TUI mid-render.
func GetCacheFilePath(config *cnfg.Config) string {
	return filepath.Join(config.Paths.Registry, "packages_cache.gob")
}

// GetCacheTimestampFilePath returns the legacy timestamp file path. The
// timestamp now lives inside the cache file itself; this is kept only so old
// installations can have the stray file cleaned up.
func GetCacheTimestampFilePath(config *cnfg.Config) string {
	return filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
}

func readCache(config *cnfg.Config) (*PackageCache, error) {
	file, err := os.Open(GetCacheFilePath(config))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cache PackageCache
	if err := gob.NewDecoder(file).Decode(&cache); err != nil {
		return nil, fmt.Errorf("decoding cache: %w", err)
	}
	return &cache, nil
}

// LoadFromCache loads packages from the cache if they are still fresh.
func LoadFromCache(config *cnfg.Config) ([]*Package, error) {
	cache, err := readCache(config)
	if err != nil {
		return nil, err
	}

	interval := config.Registry.UpdateInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if time.Since(cache.LastUpdated) > interval {
		return nil, ErrCacheStale
	}
	if len(cache.Packages) == 0 {
		return nil, ErrCacheStale
	}

	return cache.Packages, nil
}

// SaveToCache writes packages to the cache atomically, so an interrupted write
// cannot leave a truncated gob file behind.
func SaveToCache(packages []*Package, config *cnfg.Config) error {
	if err := os.MkdirAll(config.Paths.Registry, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	cacheFilePath := GetCacheFilePath(config)
	tmp, err := os.CreateTemp(config.Paths.Registry, "packages_cache-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	cache := PackageCache{Packages: packages, LastUpdated: time.Now()}
	if err := gob.NewEncoder(tmp).Encode(&cache); err != nil {
		tmp.Close()
		return fmt.Errorf("encoding cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, cacheFilePath); err != nil {
		return fmt.Errorf("writing cache: %w", err)
	}

	// Drop the redundant sidecar written by older versions.
	os.Remove(GetCacheTimestampFilePath(config))
	return nil
}

// ClearCache removes the cached registry.
func ClearCache(config *cnfg.Config) error {
	if err := os.Remove(GetCacheFilePath(config)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(GetCacheTimestampFilePath(config)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
