package handler

// accountJSON is the unified API representation of a user account (local persona or remote actor).
// ユーザーアカウント (ローカルペルソナまたはリモート Actor) の統一 API 表現。

import (
	"context"
	"time"

	"github.com/murlog-org/murlog"
)

type accountJSON struct {
	ID             string               `json:"id,omitempty"`
	URI            string               `json:"uri,omitempty"`
	Acct           string               `json:"acct"`
	Username       string               `json:"username"`
	DisplayName    string               `json:"display_name"`
	Summary        string               `json:"summary"`
	Fields         []murlog.CustomField  `json:"fields,omitempty"`
	AvatarURL      string               `json:"avatar_url,omitempty"`
	HeaderURL      string               `json:"header_url,omitempty"`
	FeaturedURL    string               `json:"featured_url,omitempty"`
	Primary        bool                 `json:"primary"`
	Locked         bool                 `json:"locked"`
	ShowFollows    bool                 `json:"show_follows"`
	Discoverable   bool                 `json:"discoverable"`
	PostCount      int                  `json:"post_count"`
	FollowingCount int                  `json:"following_count"`
	FollowersCount int                  `json:"followers_count"`
	CreatedAt      string               `json:"created_at,omitempty"`
	UpdatedAt      string               `json:"updated_at,omitempty"`
}

// toAccountFromPersona converts a local Persona to accountJSON with resolved media URLs.
// ローカル Persona をメディア URL 解決済みの accountJSON に変換する。
func (h *Handler) toAccountFromPersona(ctx context.Context, base string, p *murlog.Persona) accountJSON {
	fields := p.Fields()
	if fields == nil {
		fields = []murlog.CustomField{}
	}
	return accountJSON{
		ID:             p.ID.String(),
		Acct:           "@" + p.Username,
		Username:       p.Username,
		DisplayName:    p.DisplayName,
		Summary:        p.Summary,
		Fields:         fields,
		AvatarURL:      h.resolveMediaURL(base, p.AvatarPath),
		HeaderURL:      h.resolveMediaURL(base, p.HeaderPath),
		Primary:        p.Primary,
		Locked:         p.Locked,
		ShowFollows:    p.ShowFollows,
		Discoverable:   p.Discoverable,
		PostCount:      p.PostCount,
		FollowingCount: p.FollowingCount,
		FollowersCount: p.FollowersCount,
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
	}
}

// toAccountFromRemoteActor converts a cached RemoteActor to accountJSON.
// キャッシュ済み RemoteActor を accountJSON に変換する。
func toAccountFromRemoteActor(ra *murlog.RemoteActor) accountJSON {
	return accountJSON{
		URI:         ra.URI,
		Acct:        formatAcct(ra),
		Username:    ra.Username,
		DisplayName: ra.DisplayName,
		Summary:     ra.Summary,
		AvatarURL:   ra.AvatarURL,
		HeaderURL:   ra.HeaderURL,
		FeaturedURL: ra.FeaturedURL,
		Fields:      ra.Fields(),
	}
}
