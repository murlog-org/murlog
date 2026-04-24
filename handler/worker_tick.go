package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"
)

const (
	// workerTickLimit is the max jobs per worker-tick request.
	// worker-tick 1回あたりの最大処理件数。
	workerTickLimit = 20

	// workerTickTimeout is the timeout for worker-tick processing.
	// worker-tick 処理のタイムアウト。
	workerTickTimeout = 25 * time.Second
)

// handleWorkerTick processes pending jobs.
// Auth: session cookie (SPA) or X-Worker-Secret header (CGI self-kick).
// pending ジョブを処理する。認証: セッション Cookie (SPA) または X-Worker-Secret (CGI self-kick)。
func (h *Handler) handleWorkerTick(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !h.isWorkerTickAuthed(ctx, r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if h.worker == nil {
		http.Error(w, "worker not available", http.StatusServiceUnavailable)
		return
	}

	n := h.worker.RunBatch(ctx, workerTickLimit, workerTickTimeout)

	// Clean up orphaned attachments (uploaded but never attached to a post).
	// 孤立した添付ファイル（アップロード後に投稿に紐づかなかったもの）を削除。
	h.cleanOrphanAttachments(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"processed":` + itoa(n) + `}`))
}

// isWorkerTickAuthed checks session cookie or X-Worker-Secret.
func (h *Handler) isWorkerTickAuthed(ctx context.Context, r *http.Request) bool {
	// Session cookie (SPA).
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		hash := hashToken(cookie.Value)
		sess, err := h.store.GetSession(ctx, hash)
		if err == nil && sess.ExpiresAt.After(time.Now()) {
			return true
		}
	}

	// X-Worker-Secret header (CGI self-kick).
	if secret := r.Header.Get("X-Worker-Secret"); secret != "" {
		stored, err := h.store.GetSetting(ctx, SettingWorkerSecret)
		if err == nil && stored != "" {
			return subtle.ConstantTimeCompare([]byte(secret), []byte(stored)) == 1
		}
	}

	return false
}

// cleanOrphanAttachments deletes unattached media older than 1 hour.
// 1時間以上前の未紐づけメディアを削除する。
func (h *Handler) cleanOrphanAttachments(ctx context.Context) {
	cutoff := time.Now().Add(-1 * time.Hour)
	orphans, err := h.store.ListOrphanAttachments(ctx, cutoff)
	if err != nil || len(orphans) == 0 {
		return
	}
	for _, a := range orphans {
		h.media.Delete(a.FilePath)
		h.store.DeleteAttachment(ctx, a.ID)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
