package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver = %q, want sqlite", cfg.DBDriver)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want :8080", cfg.Listen)
	}
	if cfg.StorageType != "fs" {
		t.Errorf("StorageType = %q, want fs", cfg.StorageType)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/murlog.ini")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver = %q, want sqlite", cfg.DBDriver)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "murlog.ini")
	os.WriteFile(path, []byte(`
# DB settings
db_driver = sqlite
data_dir = ./test-data
listen = :9090
`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != "./test-data" {
		t.Errorf("DataDir = %q, want ./test-data", cfg.DataDir)
	}
	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %q, want :9090", cfg.Listen)
	}
}

func TestLoadBoolAndInt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "murlog.ini")
	os.WriteFile(path, []byte(`
trusted_proxy = true
worker_min_concurrency = 4
worker_max_concurrency = 16
`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.TrustedProxy {
		t.Error("TrustedProxy should be true")
	}
	if cfg.WorkerMinConcurrency != 4 {
		t.Errorf("WorkerMinConcurrency = %d, want 4", cfg.WorkerMinConcurrency)
	}
	if cfg.WorkerMaxConcurrency != 16 {
		t.Errorf("WorkerMaxConcurrency = %d, want 16", cfg.WorkerMaxConcurrency)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("MURLOG_DATA_DIR", "/tmp/env-data")
	t.Setenv("MURLOG_LISTEN", ":7070")
	t.Setenv("MURLOG_STORAGE_TYPE", "s3")

	cfg, err := Load("/nonexistent/murlog.ini")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != "/tmp/env-data" {
		t.Errorf("DataDir = %q, want /tmp/env-data", cfg.DataDir)
	}
	if cfg.Listen != ":7070" {
		t.Errorf("Listen = %q, want :7070", cfg.Listen)
	}
	if cfg.StorageType != "s3" {
		t.Errorf("StorageType = %q, want s3", cfg.StorageType)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "murlog.ini")

	cfg := Defaults()
	cfg.Path = path
	cfg.DataDir = "./saved-data"
	cfg.Listen = ":6060"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DataDir != "./saved-data" {
		t.Errorf("DataDir = %q, want ./saved-data", loaded.DataDir)
	}
	if loaded.Listen != ":6060" {
		t.Errorf("Listen = %q, want :6060", loaded.Listen)
	}
}

func TestSaveFilePermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "murlog.ini")

	cfg := Defaults()
	cfg.Path = path
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permission = %o, want 600", perm)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "murlog.ini")

	if Exists(path) {
		t.Error("Exists should be false when file doesn't exist")
	}

	os.WriteFile(path, []byte(""), 0644)
	if !Exists(path) {
		t.Error("Exists should be true when file exists")
	}

	os.Remove(path)
	t.Setenv("MURLOG_DATA_DIR", "/tmp/env-data")
	if !Exists(path) {
		t.Error("Exists should be true when MURLOG_DATA_DIR is set")
	}
}

func TestMainDBPath(t *testing.T) {
	cfg := &Config{DataDir: "/var/murlog/data"}
	if got := cfg.MainDBPath(); got != "/var/murlog/data/murlog.db" {
		t.Errorf("MainDBPath() = %q, want /var/murlog/data/murlog.db", got)
	}
}
