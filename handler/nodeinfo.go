package handler

import (
	"encoding/json"
	"net/http"

	"github.com/murlog-org/murlog"
)

// handleNodeInfoDiscovery serves /.well-known/nodeinfo (JRD linking to nodeinfo endpoint).
// /.well-known/nodeinfo を返す（nodeinfo エンドポイントへの JRD リンク）。
func (h *Handler) handleNodeInfoDiscovery(w http.ResponseWriter, r *http.Request) {
	base := h.baseURL(r)

	jrd := map[string]interface{}{
		"links": []map[string]string{
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				"href": base + "/nodeinfo/2.0",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(jrd)
}

// handleNodeInfo serves /nodeinfo/2.0 (NodeInfo 2.0 schema).
// /nodeinfo/2.0 を返す（NodeInfo 2.0 スキーマ）。
func (h *Handler) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Count local posts and personas for usage stats.
	// NodeInfo 用にローカル投稿数とペルソナ数を取得。
	postCount, _ := h.store.CountLocalPosts(ctx)
	userCount, _ := h.store.CountPersonas(ctx)

	nodeinfo := map[string]interface{}{
		"version": "2.0",
		"software": map[string]string{
			"name":    "murlog",
			"version": murlog.Version,
		},
		"protocols": []string{"activitypub"},
		"services": map[string][]string{
			"inbound":  {},
			"outbound": {},
		},
		"usage": map[string]interface{}{
			"users": map[string]int{
				"total": userCount,
			},
			"localPosts": postCount,
		},
		"openRegistrations": false,
		"metadata": map[string]interface{}{
			"maxNoteTextLength": MaxPostContentLength,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(nodeinfo)
}
