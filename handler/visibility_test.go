package handler

import (
	"testing"

	"github.com/murlog-org/murlog"
)

func TestDetectVisibility(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want murlog.Visibility
	}{
		{
			name: "public — Public in to",
			obj: map[string]interface{}{
				"to": "https://www.w3.org/ns/activitystreams#Public",
				"cc": "https://example.com/users/alice/followers",
			},
			want: murlog.VisibilityPublic,
		},
		{
			name: "unlisted — Public in cc",
			obj: map[string]interface{}{
				"to": "https://example.com/users/alice/followers",
				"cc": "https://www.w3.org/ns/activitystreams#Public",
			},
			want: murlog.VisibilityUnlisted,
		},
		{
			name: "followers — followers collection in to",
			obj: map[string]interface{}{
				"to": "https://example.com/users/alice/followers",
			},
			want: murlog.VisibilityFollowers,
		},
		{
			name: "followers — followers collection in cc",
			obj: map[string]interface{}{
				"to": "https://example.com/users/bob",
				"cc": "https://example.com/users/alice/followers",
			},
			want: murlog.VisibilityFollowers,
		},
		{
			name: "direct — no Public, no followers collection",
			obj: map[string]interface{}{
				"to": "https://example.com/users/bob",
			},
			want: murlog.VisibilityDirect,
		},
		{
			name: "direct — multiple recipients, no followers collection",
			obj: map[string]interface{}{
				"to": []interface{}{
					"https://example.com/users/bob",
					"https://example.com/users/carol",
				},
			},
			want: murlog.VisibilityDirect,
		},
		{
			name: "direct — empty to/cc",
			obj:  map[string]interface{}{},
			want: murlog.VisibilityDirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectVisibility(tt.obj)
			if got != tt.want {
				t.Errorf("detectVisibility() = %d, want %d", got, tt.want)
			}
		})
	}
}
