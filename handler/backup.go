package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/murlog-org/murlog/id"
)

// handleBackup handles GET /api/mur/v1/backup — download a consistent DB snapshot.
// Uses VACUUM INTO to create a point-in-time copy without blocking writes.
// VACUUM INTO で書き込みをブロックせずに整合性のある DB スナップショットをダウンロードする。
func (h *Handler) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !h.isHTTPAuthed(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Create temp file next to the DB (CGI environments may not have /tmp access).
	// DB と同じディレクトリに一時ファイルを作成（CGI 環境では /tmp にアクセスできない場合がある）。
	tmpDir := h.cfg.DataDir
	tmpFile := filepath.Join(tmpDir, ".murlog-backup-"+id.New().String()+".db")

	// Create a consistent snapshot (WAL merged, no locks held after).
	// 整合性のあるスナップショットを作成（WAL マージ済み、完了後ロック解放）。
	err := h.store.BackupTo(r.Context(), tmpFile)
	if err != nil {
		http.Error(w, "backup failed", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile)

	// Serve as downloadable file. / ダウンロード可能なファイルとして配信。
	domain := h.domain(r)
	filename := "murlog-" + domain + "-" + time.Now().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, tmpFile)
}
