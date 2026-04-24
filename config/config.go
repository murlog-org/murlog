// Package config handles loading murlog.ini.
// murlog.ini の読み込みを担当するパッケージ。
//
// murlog.ini contains only DB connection and runtime settings.
// Application settings (domain, password, etc.) are stored in the DB.
// murlog.ini は DB 接続情報とランタイム設定のみ。
// アプリケーション設定 (domain, password 等) は DB に保存。
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// Config holds startup configuration read from murlog.ini.
// murlog.ini から読み込む起動時設定。
type Config struct {
	DBDriver string `ini:"db_driver"`
	DataDir  string `ini:"data_dir"`
	MediaPath string `ini:"media_path"`
	WebDir    string `ini:"web_dir"`
	Listen    string `ini:"listen"`

	// StorageType selects the media storage backend: "fs" (default) or "s3".
	// メディアストレージのバックエンド: "fs" (デフォルト) または "s3"。
	StorageType string `ini:"storage_type"`

	// S3-compatible storage settings (used when storage_type = "s3").
	// S3 互換ストレージ設定 (storage_type = "s3" 時に使用)。
	S3Bucket    string `ini:"s3_bucket"`
	S3Region    string `ini:"s3_region"`
	S3Endpoint  string `ini:"s3_endpoint"`
	S3AccessKey string `ini:"s3_access_key"`
	S3SecretKey string `ini:"s3_secret_key"`
	S3PublicURL string `ini:"s3_public_url"`

	// Worker settings. 0 means use defaults.
	// ワーカー設定。0 はデフォルト値を使用。
	WorkerMinConcurrency int `ini:"worker_min_concurrency"`
	WorkerMaxConcurrency int `ini:"worker_max_concurrency"`
	WorkerJobRetentionDays int `ini:"worker_job_retention_days"`

	// TrustedProxy enables X-Forwarded-For client IP extraction (for reverse proxy setups).
	// リバースプロキシ環境で X-Forwarded-For からクライアント IP を取得する。
	TrustedProxy bool `ini:"trusted_proxy"`

	// Path is the file path of murlog.ini (not serialized).
	// murlog.ini のファイルパス (シリアライズ対象外)。
	Path string `ini:"-"`
}

// Defaults returns a Config with default values.
// デフォルト値の Config を返す。
// CGI 環境では web_dir を "./dist" にする (zip の構成に合わせる)。
func Defaults() *Config {
	webDir := "./web/dist"
	if os.Getenv("GATEWAY_INTERFACE") != "" {
		webDir = "./dist"
	}
	return &Config{
		DBDriver:    "sqlite",
		DataDir:     "./data",
		MediaPath:   "./media",
		WebDir:      webDir,
		Listen:      ":8080",
		StorageType: "fs",
	}
}

// Load reads a murlog.ini file, then overlays environment variables.
// Returns defaults if the file does not exist.
// murlog.ini を読み込み、環境変数で上書きする。ファイルが存在しなければデフォルト値を返す。
func Load(path string) (*Config, error) {
	cfg := Defaults()
	cfg.Path = path

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// File not found — continue with defaults + env overlay.
	} else if err != nil {
		return nil, err
	} else if err := parseINI(data, cfg); err != nil {
		return nil, err
	}
	cfg.applyEnv()
	return cfg, nil
}

// parseINI parses INI-style config into a struct using `ini` tags.
// Lines starting with # are comments. Format: key = value.
// `ini` タグを使って INI 形式の設定を構造体にパースする。
// # で始まる行はコメント。形式: key = value。
func parseINI(data []byte, cfg *Config) error {
	fields := iniFields(cfg)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return fmt.Errorf("config: line %d: missing '='", lineNum)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		f, ok := fields[key]
		if !ok {
			continue // ignore unknown keys / 未知のキーは無視
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(val)
		case reflect.Int:
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("config: line %d: %q: not a number", lineNum, key)
			}
			f.SetInt(int64(n))
		case reflect.Bool:
			f.SetBool(val == "true" || val == "1")
		}
	}
	return scanner.Err()
}

// iniFields builds a map from ini tag name to reflect.Value for settable fields.
// ini タグ名から reflect.Value へのマップを構築する。
func iniFields(cfg *Config) map[string]reflect.Value {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	m := make(map[string]reflect.Value, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("ini")
		if tag == "" || tag == "-" {
			continue
		}
		m[tag] = v.Field(i)
	}
	return m
}

// applyEnv overlays MURLOG_* environment variables onto the config.
// MURLOG_* 環境変数で設定を上書きする。
func (c *Config) applyEnv() {
	if v := os.Getenv("MURLOG_DB_DRIVER"); v != "" {
		c.DBDriver = v
	}
	if v := os.Getenv("MURLOG_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("MURLOG_MEDIA_PATH"); v != "" {
		c.MediaPath = v
	}
	if v := os.Getenv("MURLOG_WEB_DIR"); v != "" {
		c.WebDir = v
	}
	if v := os.Getenv("MURLOG_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("MURLOG_STORAGE_TYPE"); v != "" {
		c.StorageType = v
	}
	if v := os.Getenv("MURLOG_S3_BUCKET"); v != "" {
		c.S3Bucket = v
	}
	if v := os.Getenv("MURLOG_S3_REGION"); v != "" {
		c.S3Region = v
	}
	if v := os.Getenv("MURLOG_S3_ENDPOINT"); v != "" {
		c.S3Endpoint = v
	}
	if v := os.Getenv("MURLOG_S3_ACCESS_KEY"); v != "" {
		c.S3AccessKey = v
	}
	if v := os.Getenv("MURLOG_S3_SECRET_KEY"); v != "" {
		c.S3SecretKey = v
	}
	if v := os.Getenv("MURLOG_S3_PUBLIC_URL"); v != "" {
		c.S3PublicURL = v
	}
	if v := os.Getenv("MURLOG_WORKER_MIN_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.WorkerMinConcurrency = n
		}
	}
	if v := os.Getenv("MURLOG_WORKER_MAX_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.WorkerMaxConcurrency = n
		}
	}
	if v := os.Getenv("MURLOG_WORKER_JOB_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.WorkerJobRetentionDays = n
		}
	}
}

// Exists checks if configuration is available (file exists or env vars set).
// 設定が利用可能か確認する（ファイルが存在するか、環境変数が設定されているか）。
func Exists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	// If MURLOG_DATA_DIR is set, config comes from env — no file needed.
	// MURLOG_DATA_DIR が設定されていれば環境変数由来 — ファイル不要。
	return os.Getenv("MURLOG_DATA_DIR") != ""
}

// MainDBPath returns the path to the main database file.
// メインデータベースファイルのパスを返す。
func (c *Config) MainDBPath() string {
	return filepath.Join(c.DataDir, "murlog.db")
}

// Save writes the config to the given path as INI format.
// 設定を INI 形式でファイルに書き出す。
func (c *Config) Save() error {
	if c.Path == "" {
		return fmt.Errorf("config: path not set")
	}
	f, err := os.OpenFile(c.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("ini")
		if tag == "" || tag == "-" {
			continue
		}
		fv := v.Field(i)
		// Skip zero values to keep the file clean.
		// ゼロ値はスキップしてファイルをきれいに保つ。
		if fv.IsZero() {
			continue
		}
		switch fv.Kind() {
		case reflect.String:
			fmt.Fprintf(f, "%s = %s\n", tag, fv.String())
		case reflect.Int:
			fmt.Fprintf(f, "%s = %d\n", tag, fv.Int())
		case reflect.Bool:
			fmt.Fprintf(f, "%s = %t\n", tag, fv.Bool())
		}
	}
	return nil
}
