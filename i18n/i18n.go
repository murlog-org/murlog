// Package i18n provides internationalization support using external JSON locale files.
// 外部 JSON ロケールファイルによる多言語対応を提供するパッケージ。
package i18n

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mu      sync.RWMutex
	bundles = map[string]map[string]string{}
)

// LoadDir loads all *.json locale files from the given directory.
// The filename (without extension) is used as the language code (e.g. "fr.json" → "fr").
// 指定ディレクトリから全 *.json ロケールファイルを読み込む。
// ファイル名（拡張子なし）が言語コードになる（例: "fr.json" → "fr"）。
func LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		lang := strings.TrimSuffix(name, ".json")
		if err := LoadFile(filepath.Join(dir, name), lang); err != nil {
			return err
		}
	}
	return nil
}

// LoadFile loads a single locale JSON file.
// 単一のロケール JSON ファイルを読み込む。
func LoadFile(path, lang string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	mu.Lock()
	bundles[lang] = m
	mu.Unlock()
	return nil
}

// T returns the translated string for the given key and language.
// Falls back to English, then returns the key itself.
// 指定言語でキーに対応する翻訳文字列を返す。
// 英語にフォールバックし、それもなければキーをそのまま返す。
func T(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if b, ok := bundles[lang]; ok {
		if v, ok := b[key]; ok {
			return v
		}
	}
	if b, ok := bundles["en"]; ok {
		if v, ok := b[key]; ok {
			return v
		}
	}
	return key
}

// DetectLang picks the best loaded language from the Accept-Language header.
// Returns "en" as default.
// Accept-Language ヘッダーからロード済み言語の中で最適なものを選択する。デフォルトは "en"。
func DetectLang(r *http.Request) string {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return "en"
	}
	mu.RLock()
	defer mu.RUnlock()
	// Parse Accept-Language, e.g. "ja,en-US;q=0.9,en;q=0.8"
	// Simplified: split by comma, check prefix match against loaded langs.
	for _, part := range strings.Split(accept, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		lang := strings.SplitN(tag, "-", 2)[0]
		if _, ok := bundles[lang]; ok {
			return lang
		}
	}
	return "en"
}

// Func returns a template.FuncMap with a "t" function bound to the given language.
// 指定言語にバインドされた "t" 関数を含む template.FuncMap を返す。
func Func(lang string) map[string]any {
	return map[string]any{
		"t": func(key string) string { return T(lang, key) },
	}
}
