package handler

import (
	"net/http"
	"strings"
)

// handleTagPage serves the tag page for a given hashtag.
// SSR template available: render with theme. Otherwise: SPA fallback.
// ハッシュタグのタグページを返す。SSR テンプレートがあればテーマ描画、なければ SPA。
func (h *Handler) handleTagPage(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	tag = strings.ToLower(tag)
	if tag == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	h.renderTagPage(w, r, tag)
}
