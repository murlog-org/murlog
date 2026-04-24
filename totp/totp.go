// Package totp implements TOTP (RFC 6238) for two-factor authentication.
// 二要素認証用の TOTP (RFC 6238) を実装するパッケージ。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"time"
)

const (
	// SecretSize is the number of random bytes for the secret key (20 bytes = 160 bits).
	// 秘密鍵のランダムバイト数 (20 バイト = 160 ビット)。
	SecretSize = 20

	// Digits is the number of TOTP digits.
	// TOTP の桁数。
	Digits = 6

	// Period is the time step in seconds (RFC 6238 default).
	// タイムステップ秒数 (RFC 6238 デフォルト)。
	Period = 30

	// Window is the number of time steps to check before/after current.
	// 現在の前後にチェックするタイムステップ数。
	Window = 1
)

// GenerateSecret creates a new base32-encoded secret key.
// 新しい Base32 エンコード秘密鍵を生成する。
func GenerateSecret() (string, error) {
	b := make([]byte, SecretSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Validate checks if the given code is valid for the secret at the current time.
// Allows ±Window time steps for clock skew.
// 指定コードが現在時刻で有効かチェックする。クロックスキュー許容で前後 Window ステップ。
func Validate(secret, code string) bool {
	return ValidateAt(secret, code, time.Now())
}

// ValidateAt checks if the given code is valid at the specified time.
// 指定時刻で指定コードが有効かチェックする。
func ValidateAt(secret, code string, t time.Time) bool {
	counter := t.Unix() / Period
	for i := -int64(Window); i <= int64(Window); i++ {
		expected := generateCode(secret, counter+i)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// generateCode computes the TOTP code for a given secret and counter.
// 指定秘密鍵とカウンターから TOTP コードを計算する。
func generateCode(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}

	// Counter to big-endian 8 bytes. / カウンターをビッグエンディアン 8 バイトに。
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	// HMAC-SHA1. / HMAC-SHA1。
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 Section 5.4).
	// ダイナミックトランケーション。
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	otp := code % uint32(math.Pow10(Digits))

	return fmt.Sprintf("%0*d", Digits, otp)
}

// URI builds an otpauth:// URI for QR code generation.
// QR コード生成用の otpauth:// URI を組み立てる。
func URI(secret, issuer, account string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		url.PathEscape(issuer),
		url.PathEscape(account),
		secret,
		url.QueryEscape(issuer),
		Digits,
		Period,
	)
}
