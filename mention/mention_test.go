package mention

import (
	"strings"
	"testing"
)

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
	}{
		{
			name:  "single mention",
			input: "hello @alice@example.com",
			want:  []string{"alice@example.com"},
		},
		{
			name:  "multiple mentions",
			input: "@alice@example.com @bob@mastodon.social hi",
			want:  []string{"alice@example.com", "bob@mastodon.social"},
		},
		{
			name:  "duplicate dedup",
			input: "@alice@example.com hey @alice@example.com",
			want:  []string{"alice@example.com"},
		},
		{
			name:  "case normalized",
			input: "@Alice@Example.COM",
			want:  []string{"alice@example.com"},
		},
		{
			name:  "no mentions",
			input: "just a normal post",
			want:  nil,
		},
		{
			name:  "email-like not preceded by word char",
			input: "contact user@example.com please",
			want:  nil, // "user@example.com" preceded by space+word "user" — not a mention
		},
		{
			name:  "mention at start",
			input: "@bob@example.org hello",
			want:  []string{"bob@example.org"},
		},
		{
			name:  "mention after newline",
			input: "hello\n@bob@example.org",
			want:  []string{"bob@example.org"},
		},
		{
			name:  "subdomain",
			input: "@user@social.example.co.jp",
			want:  []string{"user@social.example.co.jp"},
		},
		{
			name:  "no match for URL path",
			input: "https://example.com/@user@domain.com",
			want:  nil, // preceded by /
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMentions(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseMentions(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseMentions(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReplaceWithHTML(t *testing.T) {
	resolved := map[string]Resolved{
		"alice@example.com": {
			Acct:       "alice@example.com",
			ActorURI:   "https://example.com/users/alice",
			ProfileURL: "https://example.com/@alice",
		},
	}

	tests := []struct {
		name     string
		input    string
		resolved map[string]Resolved
		wantSub  string // substring that should appear in output / 出力に含まれるべき部分文字列
	}{
		{
			name:     "mention replaced",
			input:    "hello @alice@example.com!",
			resolved: resolved,
			wantSub:  `<span class="h-card"><a href="https://example.com/@alice" class="u-url mention">@<span>alice</span></a></span>`,
		},
		{
			name:     "unresolved left as-is",
			input:    "@bob@unknown.org hello",
			resolved: resolved,
			wantSub:  "@bob@unknown.org",
		},
		{
			name:     "empty resolved map",
			input:    "@alice@example.com",
			resolved: nil,
			wantSub:  "@alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplaceWithHTML(tt.input, tt.resolved)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("ReplaceWithHTML(%q) = %q, want substring %q", tt.input, got, tt.wantSub)
			}
		})
	}
}

func TestIsSafeURL(t *testing.T) {
	tests := []struct {
		url  string
		safe bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"https://example.com:3000/path", true},
		{"javascript:alert(1)", false},
		{"data:text/html,<h1>hi</h1>", false},
		{"", false},
		{"://invalid", false},
		{"ftp://example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := IsSafeURL(tt.url)
			if got != tt.safe {
				t.Errorf("IsSafeURL(%q) = %v, want %v", tt.url, got, tt.safe)
			}
		})
	}
}

func TestReplaceWithHTML_RejectsJavascriptURL(t *testing.T) {
	// M7: javascript: URL がリンクに使われないことを検証。
	// M7: Verify javascript: URLs are not used in links.
	resolved := map[string]Resolved{
		"evil@example.com": {
			Acct:       "evil@example.com",
			ActorURI:   "javascript:alert(1)",
			ProfileURL: "javascript:alert(1)",
		},
	}

	got := ReplaceWithHTML("hello @evil@example.com", resolved)
	if strings.Contains(got, "javascript:") {
		t.Errorf("ReplaceWithHTML should reject javascript: URL, got %q", got)
	}
	// Should fall back to plain text (no link).
	// プレーンテキストにフォールバックすること (リンクなし)。
	if strings.Contains(got, "<a ") {
		t.Errorf("ReplaceWithHTML should not produce link for javascript: URL, got %q", got)
	}
}
