// Package activitypub provides minimal ActivityPub vocabulary types and protocol logic.
// 最小限の ActivityPub 語彙型とプロトコルロジックを提供するパッケージ。
package activitypub

import (
	"strings"

	"github.com/murlog-org/murlog"
)

// Actor represents an ActivityPub Actor.
// ActivityPub Actor を表す。
type Actor struct {
	Context                   interface{} `json:"@context"`
	ID                        string      `json:"id"`
	Type                      string      `json:"type"`
	PreferredUsername         string      `json:"preferredUsername"`
	Name                      string      `json:"name,omitempty"`
	Summary                   string      `json:"summary,omitempty"`
	URL                       string      `json:"url,omitempty"`
	Inbox                     string      `json:"inbox"`
	Outbox                    string      `json:"outbox"`
	Followers                 string      `json:"followers,omitempty"`
	Following                 string      `json:"following,omitempty"`
	Featured                  string      `json:"featured,omitempty"`
	ManuallyApprovesFollowers bool        `json:"manuallyApprovesFollowers"`
	Discoverable              bool        `json:"discoverable"`
	PublicKey                 PublicKey   `json:"publicKey"`
	Icon                      interface{} `json:"icon,omitempty"`
	Image                     interface{} `json:"image,omitempty"`
	Attachment                interface{} `json:"attachment,omitempty"` // PropertyValue array / PropertyValue 配列
}

// PublicKey is the public key attached to an Actor for HTTP Signatures.
// HTTP Signatures 用に Actor に紐づく公開鍵。
type PublicKey struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

// Activity represents a generic ActivityPub activity.
// 汎用の ActivityPub Activity を表す。
type Activity struct {
	Context interface{} `json:"@context"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Actor   string      `json:"actor"`
	Object  interface{} `json:"object"`
}

// Note represents an ActivityPub Note object.
// ActivityPub Note オブジェクトを表す。
type Note struct {
	Context      interface{}       `json:"@context"`
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	AttributedTo string            `json:"attributedTo"`
	InReplyTo    string            `json:"inReplyTo,omitempty"`    // parent note URI / リプライ先 Note URI
	Content      string            `json:"content"`
	ContentMap   map[string]string `json:"contentMap,omitempty"`   // lang -> content / 言語別コンテンツ
	Summary      string            `json:"summary,omitempty"`      // CW text / CW テキスト
	Sensitive    bool              `json:"sensitive,omitempty"`    // sensitive media flag / センシティブフラグ
	Published    string            `json:"published"`
	Updated      string            `json:"updated,omitempty"`      // last edit time / 最終編集日時
	To           []string          `json:"to"`
	CC           []string          `json:"cc,omitempty"`
	Tag          []NoteTag         `json:"tag,omitempty"`
	Attachment   []NoteAttachment  `json:"attachment,omitempty"`
}

// BuildLocalActor constructs an Actor object from a local Persona.
// resolveMediaURL converts a relative media path to an absolute URL.
// ローカル Persona から Actor オブジェクトを構築する。handler と worker で共用。
// resolveMediaURL は相対メディアパスを絶対 URL に変換する。
func BuildLocalActor(persona *murlog.Persona, baseURL string, resolveMediaURL func(path string) string) Actor {
	uri := baseURL + "/users/" + persona.Username
	actor := Actor{
		Context: []interface{}{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
		},
		ID:                        uri,
		Type:                      "Person",
		PreferredUsername:         persona.Username,
		Name:                      persona.DisplayName,
		Summary:                   persona.Summary,
		URL:                       uri,
		Inbox:                     uri + "/inbox",
		Outbox:                    uri + "/outbox",
		Followers:                 uri + "/followers",
		Following:                 uri + "/following",
		Featured:                  uri + "/collections/featured",
		ManuallyApprovesFollowers: persona.Locked,
		Discoverable:              persona.Discoverable,
		PublicKey: PublicKey{
			ID:           uri + "#main-key",
			Owner:        uri,
			PublicKeyPEM: persona.PublicKeyPEM,
		},
	}

	fields := persona.Fields()
	if len(fields) > 0 {
		attachment := make([]map[string]string, len(fields))
		for i, f := range fields {
			attachment[i] = map[string]string{
				"type":  "PropertyValue",
				"name":  f.Name,
				"value": f.Value,
			}
		}
		actor.Attachment = attachment
	}

	if persona.AvatarPath != "" {
		actor.Icon = map[string]string{
			"type":      "Image",
			"mediaType": MIMEFromPath(persona.AvatarPath),
			"url":       resolveMediaURL(persona.AvatarPath),
		}
	}
	if persona.HeaderPath != "" {
		actor.Image = map[string]string{
			"type":      "Image",
			"mediaType": MIMEFromPath(persona.HeaderPath),
			"url":       resolveMediaURL(persona.HeaderPath),
		}
	}

	return actor
}

// MIMEFromPath returns the MIME type based on file extension.
// ファイル拡張子から MIME タイプを返す。
func MIMEFromPath(path string) string {
	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// ResolveActorFields extracts PropertyValue fields from an Actor's attachment.
// Actor の attachment から PropertyValue フィールドを抽出する。
func ResolveActorFields(a *Actor) []murlog.CustomField {
	arr, ok := a.Attachment.([]interface{})
	if !ok {
		return nil
	}
	var fields []murlog.CustomField
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "PropertyValue" {
			continue
		}
		name, _ := m["name"].(string)
		value, _ := m["value"].(string)
		if name != "" {
			fields = append(fields, murlog.CustomField{Name: name, Value: value})
		}
	}
	return fields
}

// NewActivity constructs an Activity with standard @context.
// 標準 @context 付きの Activity を構築する。
func NewActivity(id, activityType, actor string, object interface{}) Activity {
	return Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		ID:      id,
		Type:    activityType,
		Actor:   actor,
		Object:  object,
	}
}

// NewUndoActivity constructs an Undo Activity wrapping an inner activity.
// 内部 Activity をラップする Undo Activity を構築する。
func NewUndoActivity(id, actor string, innerType, innerID, innerObject string) Activity {
	return NewActivity(id, "Undo", actor, map[string]string{
		"type":   innerType,
		"id":     innerID,
		"actor":  actor,
		"object": innerObject,
	})
}

// OrderedCollection builds an ActivityPub OrderedCollection response.
// ActivityPub OrderedCollection レスポンスを構築する。
func OrderedCollection(id string, totalItems int, orderedItems interface{}) map[string]interface{} {
	return map[string]interface{}{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           id,
		"type":         "OrderedCollection",
		"totalItems":   totalItems,
		"orderedItems": orderedItems,
	}
}

// NoteTag represents a tag on a Note (Mention, Hashtag, etc.).
// Note に付与されるタグ (Mention, Hashtag 等) を表す。
type NoteTag struct {
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
	Name string `json:"name,omitempty"`
}

// NoteAttachment represents a media attachment on a Note (Image/Document).
// Note に添付されたメディア (Image/Document) を表す。
type NoteAttachment struct {
	Type      string `json:"type"`                // "Document"
	MediaType string `json:"mediaType"`           // e.g. "image/jpeg"
	URL       string `json:"url"`
	Name      string `json:"name,omitempty"`      // alt text / 代替テキスト
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}
