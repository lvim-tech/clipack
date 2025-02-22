package pkg

import (
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// PackageCache структура за кеширане на пакети
type PackageCache struct {
	Packages    []*Package
	LastUpdated time.Time
}

// GetCacheFilePath връща пътя към кеш файла
func GetCacheFilePath(config *Config) string {
	cacheDir := filepath.Join(config.Paths.Registry)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("Error creating cache directory: %v", err)
	}
	return filepath.Join(cacheDir, "packages_cache.gob")
}

// LoadFromCache зарежда пакетите от кеша
func LoadFromCache(config *Config) ([]*Package, error) {
	cacheFilePath := GetCacheFilePath(config)
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

	// Проверка за валидност на кеша (например, ако кешът е по-стар от 24 часа)
	if time.Since(cache.LastUpdated) > 24*time.Hour {
		return nil, fmt.Errorf("cache is outdated")
	}

	return cache.Packages, nil
}

// SaveToCache записва пакетите в кеша
func SaveToCache(packages []*Package, config *Config) error {
	cacheFilePath := GetCacheFilePath(config)
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

	return nil
}
