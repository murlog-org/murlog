// Package id provides UUIDv7 generation and handling.
// UUIDv7 の生成と操作を提供するパッケージ。
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ID is a 128-bit UUIDv7 stored as a byte array.
// 128bit の UUIDv7 をバイト配列として保持する型。
type ID [16]byte

// Nil is the zero-value ID.
// ゼロ値の ID。
var Nil ID

// New generates a new UUIDv7 using the current time.
// 現在時刻で新しい UUIDv7 を生成する。
func New() ID {
	return NewWithTime(time.Now())
}

// NewWithTime generates a new UUIDv7 using the given time.
// 指定時刻で新しい UUIDv7 を生成する。
func NewWithTime(t time.Time) ID {
	var id ID

	// 48-bit millisecond timestamp (big-endian).
	// 48bit ミリ秒タイムスタンプ (ビッグエンディアン)。
	ms := uint64(t.UnixMilli())
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)

	// Fill remaining 10 bytes with random data.
	// 残り 10 バイトをランダムデータで埋める。
	rand.Read(id[6:])

	// Set version (4 bits = 0b0111 → UUIDv7).
	// バージョンビット設定 (4bit = 0b0111 → UUIDv7)。
	id[6] = (id[6] & 0x0F) | 0x70

	// Set variant (2 bits = 0b10 → RFC 9562).
	// バリアントビット設定 (2bit = 0b10 → RFC 9562)。
	id[8] = (id[8] & 0x3F) | 0x80

	return id
}

// Parse parses a UUID string (with or without hyphens) into an ID.
// UUID 文字列（ハイフンあり/なし）を ID にパースする。
func Parse(s string) (ID, error) {
	// Strip hyphens. / ハイフンを除去。
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			clean = append(clean, s[i])
		}
	}
	if len(clean) != 32 {
		return Nil, fmt.Errorf("id: invalid UUID length: %d", len(clean))
	}

	var id ID
	_, err := hex.Decode(id[:], clean)
	if err != nil {
		return Nil, fmt.Errorf("id: invalid UUID hex: %w", err)
	}
	return id, nil
}

// FromBytes creates an ID from a byte slice.
// バイトスライスから ID を生成する。
func FromBytes(b []byte) (ID, error) {
	if len(b) != 16 {
		return Nil, fmt.Errorf("id: invalid byte length: %d", len(b))
	}
	var id ID
	copy(id[:], b)
	return id, nil
}

// String returns the standard UUID string representation (8-4-4-4-12).
// 標準 UUID 文字列表現 (8-4-4-4-12) を返す。
func (id ID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

// Bytes returns the ID as a byte slice.
// ID をバイトスライスとして返す。
func (id ID) Bytes() []byte {
	b := make([]byte, 16)
	copy(b, id[:])
	return b
}

// IsNil returns true if the ID is the zero value.
// ゼロ値なら true を返す。
func (id ID) IsNil() bool {
	return id == Nil
}

// Time extracts the timestamp from the UUIDv7.
// UUIDv7 からタイムスタンプを取り出す。
func (id ID) Time() time.Time {
	ms := uint64(id[0])<<40 | uint64(id[1])<<32 | uint64(id[2])<<24 |
		uint64(id[3])<<16 | uint64(id[4])<<8 | uint64(id[5])
	return time.UnixMilli(int64(ms))
}
