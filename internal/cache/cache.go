// Package cache keeps downloaded data on disk between runs. It only ever makes luma faster:
// anything missing or unreadable is a miss, and callers fetch as if there was no cache.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Dir returns $LUMA_CACHE_DIR if set, else the luma folder in the user's cache dir.
func Dir() (string, error) {
	if dir := os.Getenv("LUMA_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "luma"), nil
}

// path returns the file for key, hashed since keys are URLs.
func path(key string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])), nil
}

// Get returns the data saved under key if it was saved less than maxAge ago, else nil.
func Get(key string, maxAge time.Duration) []byte {
	p, err := path(key)
	if err != nil {
		return nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil
	}
	// a negative age means the clock moved back, so the real age is unknown
	if age := time.Since(info.ModTime()); age < 0 || age >= maxAge {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

// Save writes data under key, replacing it in one rename so a concurrent Get never sees half a file.
func Save(key string, data []byte) error {
	p, err := path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(p), "*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	// CreateTemp makes it 0600, a shared LUMA_CACHE_DIR should still be readable by others
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}

// GetJSON decodes the data Get returns into v, and reports false on a miss or if it doesn't decode.
func GetJSON(key string, maxAge time.Duration, v any) bool {
	data := Get(key, maxAge)
	return data != nil && json.Unmarshal(data, v) == nil
}

// SaveJSON encodes v and saves it under key.
func SaveJSON(key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return Save(key, data)
}
