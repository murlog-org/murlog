// Package mention provides mention parsing and HTML conversion for ActivityPub posts.
// ActivityPub 投稿のメンション解析と HTML 変換を提供するパッケージ。
package mention

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// mentionRe matches @user@domain patterns in plain text.
// Word boundary: must be preceded by start-of-string or non-word character (excluding /).
// プレーンテキスト中の @user@domain パターンにマッチする。
var mentionRe = regexp.MustCompile(`(?:^|[^\w/])@([a-zA-Z0-9_]+)@([a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?\.[a-zA-Z]{2,})`)

// ParseMentions extracts unique @user@domain strings from text content.
// テキストからユニークな @user@domain 文字列を抽出する。
func ParseMentions(content string) []string {
	matches := mentionRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	var result []string
	for _, m := range matches {
		acct := strings.ToLower(m[1]) + "@" + strings.ToLower(m[2])
		if !seen[acct] {
			seen[acct] = true
			result = append(result, acct)
		}
	}
	return result
}

// Resolved holds a successfully resolved mention.
// 解決済みメンションの情報。
type Resolved struct {
	Acct       string // "user@domain"
	ActorURI   string // "https://domain/users/user"
	ProfileURL string // human-readable profile URL (falls back to ActorURI) / プロフィール URL (Actor URI にフォールバック)
}

// ReplaceWithHTML replaces @user@domain patterns in plain text with Mastodon-compatible HTML links.
// Unresolved mentions are left as plain text.
// プレーンテキスト中の @user@domain を Mastodon 互換の HTML リンクに置換する。
// 未解決のメンションはプレーンテキストのまま残す。
func ReplaceWithHTML(content string, resolved map[string]Resolved) string {
	if len(resolved) == 0 {
		return content
	}

	return mentionRe.ReplaceAllStringFunc(content, func(match string) string {
		// The match may include a leading non-word char (space, newline, etc).
		// マッチには先頭の非単語文字 (空白, 改行等) が含まれる場合がある。
		sub := mentionRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}

		acct := strings.ToLower(sub[1]) + "@" + strings.ToLower(sub[2])
		r, ok := resolved[acct]
		if !ok {
			return match
		}

		profileURL := r.ProfileURL
		if profileURL == "" {
			profileURL = r.ActorURI
		}
		// M7: スキーム検証 — javascript: 等の XSS を防止。
		// M7: Validate URL scheme to prevent XSS via javascript: URLs.
		if !IsSafeURL(profileURL) {
			profileURL = r.ActorURI
			if !IsSafeURL(profileURL) {
				return match
			}
		}
		username := sub[1]

		// Preserve any leading character before the @.
		// @ の前の先頭文字を保持する。
		prefix := ""
		atIdx := strings.Index(match, "@"+sub[1])
		if atIdx > 0 {
			prefix = match[:atIdx]
		}

		link := fmt.Sprintf(`%s<span class="h-card"><a href="%s" class="u-url mention">@<span>%s</span></a></span>`,
			prefix, html.EscapeString(profileURL), html.EscapeString(username))
		return link
	})
}

// IsSafeURL checks that a URL uses https or http scheme.
// URL が https または http スキームであることを検証する。
func IsSafeURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http"
}
