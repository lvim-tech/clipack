package pkg

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
)

// PackageCache holds the cached packages and the last updated timestamp
type PackageCache struct {
	Packages    []*Package
	LastUpdated time.Time
}

// GetCacheFilePath returns the path to the cache file
func GetCacheFilePath(config *cnfg.Config) string {
	cacheDir := filepath.Join(config.Paths.Registry)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("Error creating cache directory: %v", err)
	}
	return filepath.Join(cacheDir, "packages_cache.gob")
}

// GetCacheTimestampFilePath returns the path to the cache timestamp file
func GetCacheTimestampFilePath(config *cnfg.Config) string {
	return filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
}

// LoadFromCache loads packages from the cache
func LoadFromCache(config *cnfg.Config) ([]*Package, error) {
	cacheFilePath := GetCacheFilePath(config)
	timestampFilePath := GetCacheTimestampFilePath(config)

	timestamp, err := loadTimestamp(timestampFilePath)
	if err != nil || time.Since(timestamp) > config.Registry.UpdateInterval {
		return nil, fmt.Errorf("cache is outdated or missing")
	}

	file, err := os.Open(cacheFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cache PackageCache
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&cache); err != nil {
		return nil, err
	}

	return cache.Packages, nil
}

// SaveToCache saves packages to the cache
func SaveToCache(packages []*Package, config *cnfg.Config) error {
	cacheFilePath := GetCacheFilePath(config)
	timestampFilePath := GetCacheTimestampFilePath(config)

	file, err := os.Create(cacheFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	cache := PackageCache{
		Packages:    packages,
		LastUpdated: time.Now(),
	}
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(&cache); err != nil {
		return err
	}

	if err := saveTimestamp(timestampFilePath, cache.LastUpdated); err != nil {
		return err
	}

	return nil
}

// saveTimestamp saves the timestamp to a file
func saveTimestamp(path string, timestamp time.Time) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(timestamp); err != nil {
		return fmt.Errorf("could not encode timestamp: %v", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("could not write timestamp file: %v", err)
	}

	return nil
}

// loadTimestamp loads the timestamp from a file
func loadTimestamp(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not read timestamp file: %v", err)
	}

	var timestamp time.Time
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&timestamp); err != nil {
		return time.Time{}, fmt.Errorf("could not decode timestamp file: %v", err)
	}

	return timestamp, nil
}
