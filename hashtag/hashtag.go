// Package hashtag provides hashtag parsing and HTML conversion for ActivityPub posts.
// ActivityPub 投稿のハッシュタグ解析と HTML 変換を提供するパッケージ。
package hashtag

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// hashtagRe matches #tag patterns in plain text.
// Unicode letters and digits are supported. Preceded by start-of-string or non-word (excluding / and #).
// プレーンテキスト中の #tag パターンにマッチする。Unicode 文字・数字対応。
var hashtagRe = regexp.MustCompile(`(?:^|[^\w/&#])#([\p{L}\p{N}_]+)`)

// ParseHashtags extracts unique hashtag strings from text content (normalized to lowercase).
// テキストからユニークなハッシュタグ文字列を抽出する（小文字正規化）。
func ParseHashtags(content string) []string {
	matches := hashtagRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	var result []string
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}

// ReplaceWithHTML replaces #tag patterns in plain text with HTML links.
// プレーンテキスト中の #tag を HTML リンクに置換する。
func ReplaceWithHTML(content string, baseURL string) string {
	return hashtagRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := hashtagRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}

		tag := strings.ToLower(sub[1])
		original := sub[1] // preserve original casing / 元の大文字小文字を保持

		// Preserve any leading character before the #.
		// # の前の先頭文字を保持する。
		prefix := ""
		hashIdx := strings.LastIndex(match, "#"+original)
		if hashIdx < 0 {
			// Try case-insensitive match. / 大文字小文字を無視してマッチ。
			hashIdx = strings.LastIndex(strings.ToLower(match), "#"+tag)
		}
		if hashIdx > 0 {
			prefix = match[:hashIdx]
		}

		href := fmt.Sprintf("%s/tags/%s", baseURL, html.EscapeString(tag))
		return fmt.Sprintf(`%s<a href="%s" class="hashtag" rel="tag">#<span>%s</span></a>`,
			prefix, href, html.EscapeString(original))
	})
}
