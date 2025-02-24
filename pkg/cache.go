package pkg

import (
	"bytes"
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

// GetCacheTimestampFilePath връща пътя към файла с времеви отпечатък на кеша
func GetCacheTimestampFilePath(config *Config) string {
	return filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
}

// LoadFromCache зарежда пакетите от кеша
func LoadFromCache(config *Config) ([]*Package, error) {
	cacheFilePath := GetCacheFilePath(config)
	timestampFilePath := GetCacheTimestampFilePath(config)

	// Проверка за валидност на кеша чрез времеви отпечатък
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

// SaveToCache записва пакетите в кеша
func SaveToCache(packages []*Package, config *Config) error {
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

	// Запис на времеви отпечатък
	if err := saveTimestamp(timestampFilePath, cache.LastUpdated); err != nil {
		return err
	}

	return nil
}

// saveTimestamp записва времевия отпечатък във файл
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

// loadTimestamp зарежда времевия отпечатък от файл
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
