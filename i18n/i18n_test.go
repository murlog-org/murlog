package i18n

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndTranslate(t *testing.T) {
	dir := t.TempDir()

	// Write English locale. / 英語ロケールを書き込む。
	os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{
		"greeting": "Hello",
		"farewell": "Goodbye"
	}`), 0644)

	// Write Japanese locale. / 日本語ロケールを書き込む。
	os.WriteFile(filepath.Join(dir, "ja.json"), []byte(`{
		"greeting": "こんにちは"
	}`), 0644)

	if err := LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	// Japanese translation exists.
	if got := T("ja", "greeting"); got != "こんにちは" {
		t.Errorf("T(ja, greeting) = %q, want こんにちは", got)
	}

	// Japanese fallback to English.
	if got := T("ja", "farewell"); got != "Goodbye" {
		t.Errorf("T(ja, farewell) = %q, want Goodbye (fallback to en)", got)
	}

	// English translation.
	if got := T("en", "greeting"); got != "Hello" {
		t.Errorf("T(en, greeting) = %q, want Hello", got)
	}

	// Unknown key returns key itself.
	if got := T("en", "unknown.key"); got != "unknown.key" {
		t.Errorf("T(en, unknown.key) = %q, want unknown.key", got)
	}

	// Unknown language falls back to English.
	if got := T("fr", "greeting"); got != "Hello" {
		t.Errorf("T(fr, greeting) = %q, want Hello (fallback)", got)
	}
}

func TestDetectLang(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{}`), 0644)
	os.WriteFile(filepath.Join(dir, "ja.json"), []byte(`{}`), 0644)
	LoadDir(dir)

	tests := []struct {
		accept string
		want   string
	}{
		{"ja,en-US;q=0.9,en;q=0.8", "ja"},
		{"en-US,en;q=0.9", "en"},
		{"fr-FR,fr;q=0.9", "en"}, // unknown → default en
		{"", "en"},
	}

	for _, tt := range tests {
		r, _ := http.NewRequest("GET", "/", nil)
		if tt.accept != "" {
			r.Header.Set("Accept-Language", tt.accept)
		}
		got := DetectLang(r)
		if got != tt.want {
			t.Errorf("DetectLang(%q) = %q, want %q", tt.accept, got, tt.want)
		}
	}
}
