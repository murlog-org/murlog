package hashtag

import (
	"strings"
	"testing"
)

func TestParseHashtags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"single tag", "hello #world", []string{"world"}},
		{"multiple tags", "#hello #world", []string{"hello", "world"}},
		{"duplicate", "#test #test", []string{"test"}},
		{"case normalization", "#Hello #HELLO", []string{"hello"}},
		{"with text", "check out #golang and #rust", []string{"golang", "rust"}},
		{"start of line", "#murlog is cool", []string{"murlog"}},
		{"unicode", "#日本語 #café", []string{"日本語", "café"}},
		{"underscore", "#hello_world", []string{"hello_world"}},
		{"no tags", "hello world", nil},
		{"empty", "", nil},
		{"in URL", "https://example.com/#section", nil},
		{"HTML entity", "&#123; not a tag", nil},
		{"adjacent to mention", "@user@host #tag", []string{"tag"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHashtags(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseHashtags(%q) = %v, want %v", tt.content, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseHashtags(%q)[%d] = %q, want %q", tt.content, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReplaceWithHTML(t *testing.T) {
	tests := []struct {
		name    string
		content string
		check   func(t *testing.T, result string)
	}{
		{
			"basic tag",
			"hello #world",
			func(t *testing.T, r string) {
				if !strings.Contains(r, `<a href="https://example.com/tags/world"`) {
					t.Errorf("missing link: %s", r)
				}
				if !strings.Contains(r, `class="hashtag"`) {
					t.Errorf("missing hashtag class: %s", r)
				}
				if !strings.Contains(r, `rel="tag"`) {
					t.Errorf("missing rel=tag: %s", r)
				}
			},
		},
		{
			"preserves prefix",
			"hello #world",
			func(t *testing.T, r string) {
				if !strings.HasPrefix(r, "hello ") {
					t.Errorf("prefix not preserved: %s", r)
				}
			},
		},
		{
			"preserves case in display",
			"#GoLang",
			func(t *testing.T, r string) {
				if !strings.Contains(r, "<span>GoLang</span>") {
					t.Errorf("original case not preserved: %s", r)
				}
				if !strings.Contains(r, "/tags/golang") {
					t.Errorf("URL not lowercased: %s", r)
				}
			},
		},
		{
			"no tags unchanged",
			"hello world",
			func(t *testing.T, r string) {
				if r != "hello world" {
					t.Errorf("content changed: %s", r)
				}
			},
		},
		{
			"invalid tag not matched",
			`#<script>alert</script>`,
			func(t *testing.T, r string) {
				// <script> is not valid in tag name, so no replacement.
				// <script> はタグ名として無効なので置換されない。
				if r != `#<script>alert</script>` {
					t.Errorf("should not replace invalid tag: %s", r)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceWithHTML(tt.content, "https://example.com")
			tt.check(t, result)
		})
	}
}
