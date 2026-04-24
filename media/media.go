// Package media defines the interface for media file storage.
// メディアファイルストレージのインターフェースを定義するパッケージ。
package media

import "io"

// Store defines the interface for media file storage.
// メディアファイルストレージのインターフェース。
type Store interface {
	Save(name string, r io.Reader) error
	Open(name string) (io.ReadCloser, error)
	Delete(name string) error
	URL(name string) string
}

// ResolveURL returns the absolute URL for a media path.
// If path is empty, returns "". If Store.URL returns a relative path, prepends baseURL.
// メディアパスの絶対 URL を返す。空パスは "" を返す。相対パスには baseURL を付与。
func ResolveURL(s Store, baseURL, path string) string {
	if path == "" {
		return ""
	}
	u := s.URL(path)
	if len(u) >= 8 && (u[:7] == "http://" || u[:8] == "https://") {
		return u
	}
	return baseURL + u
}
