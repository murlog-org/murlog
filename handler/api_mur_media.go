package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/media/imageproc"
)

// maxMediaSize is the maximum upload size (10 MB).
// アップロードの最大サイズ (10 MB)。
const maxMediaSize = 10 << 20

// allowedPrefixes lists valid media storage prefixes.
// 許可するメディアストレージのプレフィックス一覧。
var allowedPrefixes = map[string]bool{
	"attachments": true,
	"avatars":     true,
	"headers":     true,
}

// isAllowedPrefix returns true if the prefix is in the whitelist.
// プレフィックスがホワイトリストに含まれていれば true を返す。
func isAllowedPrefix(prefix string) bool {
	return allowedPrefixes[prefix]
}

// allowedMIME lists accepted image MIME types.
// 受け付ける画像 MIME タイプの一覧。
var allowedMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// attachmentJSON is the unified API representation of a media attachment.
// Used for local attachments (full fields) and remote/SSR (URL + Alt + MimeType only).
// メディア添付の統一 API 表現。ローカルは全フィールド、リモート/SSR は URL+Alt+MimeType のみ。
type attachmentJSON struct {
	ID       string `json:"id,omitempty"`
	URL      string `json:"url"`
	Path     string `json:"path,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Alt      string `json:"alt,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// toAttachmentJSON converts an Attachment to its API representation.
// Attachment を API 表現に変換する。
func (h *Handler) toAttachmentJSON(base string, a *murlog.Attachment) attachmentJSON {
	return attachmentJSON{
		ID:       a.ID.String(),
		URL:      h.attachmentURL(base, a),
		Path:     a.FilePath,
		MimeType: a.MimeType,
		Alt:      a.Alt,
		Width:    a.Width,
		Height:   a.Height,
	}
}

// attachmentURL returns the display URL for an attachment.
// Remote attachments use their original URL; local ones use media store URL.
// 添付ファイルの表示用 URL を返す。リモートは元 URL、ローカルはメディアストアの URL。
func (h *Handler) attachmentURL(base string, a *murlog.Attachment) string {
	if strings.HasPrefix(a.FilePath, "http://") || strings.HasPrefix(a.FilePath, "https://") {
		return a.FilePath
	}
	u := h.media.URL(a.FilePath)
	// If media store returns a relative path, prepend the base URL.
	// メディアストアが相対パスを返した場合、ベース URL を付与する。
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return base + u
	}
	return u
}

// handleMediaUpload handles POST /api/mur/v1/media (multipart file upload).
// メディアアップロードを処理する (multipart ファイルアップロード)。
func (h *Handler) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	if !h.isHTTPAuthed(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Limit request body size. / リクエストボディサイズを制限。
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaSize)

	if err := r.ParseMultipartForm(maxMediaSize); err != nil {
		http.Error(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate MIME type. / MIME タイプを検証。
	ct := header.Header.Get("Content-Type")
	ext, ok := allowedMIME[ct]
	if !ok {
		http.Error(w, "unsupported file type", http.StatusBadRequest)
		return
	}

	alt := r.FormValue("alt")

	// Read file into memory, strip EXIF, then decode dimensions.
	// ファイルをメモリに読み込み、EXIF を除去し、寸法を取得する。
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	// Strip EXIF metadata (GPS etc.) while preserving orientation.
	// EXIF メタデータ (GPS 等) を除去し、Orientation は保持する。
	stripped, err := imageproc.StripEXIF(data)
	if err != nil {
		http.Error(w, "image processing error", http.StatusInternalServerError)
		return
	}
	data = stripped

	// Release multipart form buffers to free memory before save.
	// save 前に multipart バッファを解放してメモリを空ける。
	r.MultipartForm.RemoveAll()

	// Decode image dimensions. / 画像の寸法を取得。
	var width, height int
	cfg, _, decErr := image.DecodeConfig(bytes.NewReader(data))
	if decErr == nil {
		width = cfg.Width
		height = cfg.Height
	}

	// Determine prefix from form field (default: attachments).
	// フォームフィールドからプレフィックスを決定（デフォルト: attachments）。
	prefix := r.FormValue("prefix")
	if !isAllowedPrefix(prefix) {
		prefix = "attachments"
	}

	// Generate unique filename with prefix. / プレフィックス付きユニークファイル名を生成。
	aid := id.New()
	filename := prefix + "/" + aid.String() + ext

	// Save to media store. / メディアストアに保存。
	if err := h.media.Save(filename, bytes.NewReader(data)); err != nil {
		http.Error(w, "save error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	att := &murlog.Attachment{
		ID:        aid,
		FilePath:  filename,
		MimeType:  ct,
		Alt:       alt,
		Width:     width,
		Height:    height,
		Size:      int64(len(data)),
		CreatedAt: now,
	}

	if err := h.store.CreateAttachment(r.Context(), att); err != nil {
		h.media.Delete(filename)
		http.Error(w, "create attachment failed", http.StatusInternalServerError)
		return
	}

	base := h.baseURL(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(h.toAttachmentJSON(base, att))
}

// rpcMediaDelete handles media.delete RPC method.
// media.delete RPC メソッドを処理する。
func (h *Handler) rpcMediaDelete(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[getParams](params)
	if rErr != nil {
		return nil, rErr
	}

	aid, err := id.Parse(req.ID)
	if err != nil {
		return nil, newRPCErr(codeInvalidParams, "invalid id")
	}

	att, err := h.store.GetAttachment(ctx, aid)
	if err != nil {
		return nil, newRPCErr(codeNotFound, "attachment not found")
	}

	// Delete file from media store (only for local files).
	// メディアストアからファイルを削除（ローカルファイルのみ）。
	if !strings.HasPrefix(att.FilePath, "http://") && !strings.HasPrefix(att.FilePath, "https://") {
		h.media.Delete(att.FilePath)
	}

	if err := h.store.DeleteAttachment(ctx, aid); err != nil {
		return nil, newRPCErr(codeInternalError, "delete attachment failed")
	}

	return statusOK, nil
}

// serveMedia serves media files from the media directory.
// メディアディレクトリからメディアファイルを配信する。
func (h *Handler) serveMedia(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	name = filepath.Clean(name)
	if name == "" || name == "." {
		http.NotFound(w, r)
		return
	}
	filePath := filepath.Join(h.cfg.MediaPath, name)
	// Ensure resolved path stays within MediaPath (prevent path traversal).
	// 解決済みパスが MediaPath 内に留まることを確認 (パストラバーサル防止)。
	if !strings.HasPrefix(filePath, filepath.Clean(h.cfg.MediaPath)+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filePath)
}
