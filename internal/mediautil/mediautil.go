// Package mediautil provides shared media helpers used by handler and worker.
// handler と worker で共有するメディアユーティリティ。
package mediautil

import (
	"github.com/murlog-org/murlog/activitypub"
)

// ResolveActorIcon extracts the icon URL from an Actor.
// Actor からアイコン URL を抽出する。
func ResolveActorIcon(a *activitypub.Actor) string {
	return ResolveImageURL(a.Icon)
}

// ResolveActorHeader extracts the header image URL from an Actor.
// Actor からヘッダー画像 URL を抽出する。
func ResolveActorHeader(a *activitypub.Actor) string {
	return ResolveImageURL(a.Image)
}

// ResolveImageURL extracts a URL from an ActivityPub image field (icon or image).
// ActivityPub の画像フィールド (icon または image) から URL を抽出する。
func ResolveImageURL(v interface{}) string {
	if v == nil {
		return ""
	}
	if m, ok := v.(map[string]interface{}); ok {
		if u, ok := m["url"].(string); ok {
			return u
		}
	}
	return ""
}
