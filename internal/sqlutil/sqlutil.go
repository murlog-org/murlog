// Package sqlutil provides shared SQL helpers used by store/sqlite and queue/sqlqueue.
// store/sqlite と queue/sqlqueue で共有する SQL ユーティリティ。
package sqlutil

import (
	"fmt"
	"time"

	"github.com/murlog-org/murlog/id"
)

// FormatTime formats a time as RFC 3339 for storage.
// 保存用に time を RFC 3339 文字列に変換。
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseTime converts an RFC 3339 string to time.Time.
// RFC 3339 文字列を time.Time に変換する。
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlutil: ParseTime(%q): %w", s, err)
	}
	return t, nil
}

// ScanID converts a BLOB column to id.ID.
// BLOB カラムを id.ID に変換する。
func ScanID(b []byte) (id.ID, error) {
	// NULL カラムは空バイト列 → ゼロ ID を返す (エラーではない)。
	// NULL columns produce nil/empty bytes — return zero ID (not an error).
	if len(b) == 0 {
		return id.ID{}, nil
	}
	v, err := id.FromBytes(b)
	if err != nil {
		return id.ID{}, fmt.Errorf("sqlutil: ScanID(%x): %w", b, err)
	}
	return v, nil
}
