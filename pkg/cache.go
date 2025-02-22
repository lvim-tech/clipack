package pkg

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SaveToCache запазва пакети в кеш
func SaveToCache(packages []*Package, config *Config) error {
	cachePath := filepath.Join(config.Paths.Registry, "packages_cache.gob")
	file, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("error creating cache file: %v", err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(packages); err != nil {
		return fmt.Errorf("error encoding packages to cache: %v", err)
	}

	timestampPath := filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
	timestampFile, err := os.Create(timestampPath)
	if err != nil {
		return fmt.Errorf("error creating timestamp file: %v", err)
	}
	defer timestampFile.Close()

	timestampEncoder := gob.NewEncoder(timestampFile)
	if err := timestampEncoder.Encode(time.Now()); err != nil {
		return fmt.Errorf("error encoding timestamp to cache: %v", err)
	}

	return nil
}

// LoadFromCache зарежда пакети от кеша
func LoadFromCache(config *Config) ([]*Package, error) {
	cachePath := filepath.Join(config.Paths.Registry, "packages_cache.gob")
	file, err := os.Open(cachePath)
	if err != nil {
		return nil, fmt.Errorf("error opening cache file: %v", err)
	}
	defer file.Close()

	var packages []*Package
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&packages); err != nil {
		return nil, fmt.Errorf("error decoding packages from cache: %v", err)
	}

	return packages, nil
}

// GetCacheTimestamp връща последния кеш timestamp
func GetCacheTimestamp(config *Config) (time.Time, error) {
	timestampPath := filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
	file, err := os.Open(timestampPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("error opening timestamp file: %v", err)
	}
	defer file.Close()

	var timestamp time.Time
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&timestamp); err != nil {
		return time.Time{}, fmt.Errorf("error decoding timestamp from cache: %v", err)
	}

	return timestamp, nil
}
