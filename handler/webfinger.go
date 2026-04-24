package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// WebFinger response types.
// WebFinger レスポンス型。
type webFingerResponse struct {
	Subject string          `json:"subject"`
	Links   []webFingerLink `json:"links"`
}

type webFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

// handleWebFinger handles GET /.well-known/webfinger.
// GET /.well-known/webfinger を処理する。
//
// Lookup: ?resource=acct:username@domain
func (h *Handler) handleWebFinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		http.Error(w, "missing resource parameter", http.StatusBadRequest)
		return
	}

	// Parse acct: URI. / acct: URI をパースする。
	if !strings.HasPrefix(resource, "acct:") {
		http.Error(w, "unsupported resource scheme", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(resource[5:], "@", 2)
	if len(parts) != 2 || parts[1] != h.domain(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	username := parts[0]

	// Lookup persona. / ペルソナを検索する。
	persona, err := h.store.GetPersonaByUsername(r.Context(), username)
	if err != nil || persona == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	actorURL := h.baseURL(r) + "/users/" + persona.Username

	resp := webFingerResponse{
		Subject: resource,
		Links: []webFingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actorURL,
			},
		},
	}

	w.Header().Set("Content-Type", "application/jrd+json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}
