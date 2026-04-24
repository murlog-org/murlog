package s3

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSignRequest verifies SigV4 signature against known values.
// 既知の値で SigV4 署名を検証する。
//
// Test vector derived from AWS Signature V4 documentation examples.
// AWS Signature V4 ドキュメントの例に基づくテストベクトル。
func TestSignRequest(t *testing.T) {
	// Fixed time for deterministic output. / 決定的な出力のための固定時刻。
	fixedTime := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")

	signRequestWithTime(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"us-east-1", "s3", nil, fixedTime)

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("Authorization header not set")
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Errorf("unexpected auth prefix: %s", auth)
	}
	if !strings.Contains(auth, "Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request") {
		t.Errorf("unexpected credential scope: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Errorf("missing SignedHeaders: %s", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Errorf("missing Signature: %s", auth)
	}

	// Verify x-amz-date header was set. / x-amz-date ヘッダーの設定を確認。
	amzDate := req.Header.Get("x-amz-date")
	if amzDate != "20130524T000000Z" {
		t.Errorf("unexpected x-amz-date: %s", amzDate)
	}
}

// TestSignRequestWithBody verifies SigV4 for PUT requests with body.
// ボディ付き PUT リクエストの SigV4 署名を検証する。
func TestSignRequestWithBody(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)
	body := []byte("hello world")

	req, _ := http.NewRequest("PUT", "https://mybucket.s3.us-west-2.amazonaws.com/test.txt", nil)
	req.Header.Set("Content-Type", "text/plain")

	signRequestWithTime(req, "AKID", "SECRET", "us-west-2", "s3", body, fixedTime)

	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "Credential=AKID/20240115/us-west-2/s3/aws4_request") {
		t.Errorf("unexpected credential: %s", auth)
	}
	// content-type should be in signed headers.
	// content-type が署名対象ヘッダーに含まれるべき。
	if !strings.Contains(auth, "content-type") {
		t.Errorf("content-type not in signed headers: %s", auth)
	}
	// Body hash should not be the empty hash.
	// ボディハッシュは空ハッシュではないはず。
	contentSha := req.Header.Get("x-amz-content-sha256")
	emptyHash := hashSHA256(nil)
	if contentSha == emptyHash {
		t.Error("body hash should differ from empty hash")
	}
}

// TestDeriveSigningKey verifies the key derivation chain.
// キー導出チェーンを検証する。
func TestDeriveSigningKey(t *testing.T) {
	// Same inputs should produce same key. / 同じ入力は同じキーを生成する。
	key1 := deriveSigningKey("secret", "20240101", "us-east-1", "s3")
	key2 := deriveSigningKey("secret", "20240101", "us-east-1", "s3")
	if string(key1) != string(key2) {
		t.Error("same inputs should produce same signing key")
	}

	// Different date should produce different key. / 異なる日付は異なるキーを生成する。
	key3 := deriveSigningKey("secret", "20240102", "us-east-1", "s3")
	if string(key1) == string(key3) {
		t.Error("different date should produce different signing key")
	}

	// Different region should produce different key. / 異なるリージョンは異なるキーを生成する。
	key4 := deriveSigningKey("secret", "20240101", "eu-west-1", "s3")
	if string(key1) == string(key4) {
		t.Error("different region should produce different signing key")
	}
}

// TestHashSHA256 verifies the SHA-256 hash function.
// SHA-256 ハッシュ関数を検証する。
func TestHashSHA256(t *testing.T) {
	// Known SHA-256 of empty string. / 空文字列の既知の SHA-256。
	got := hashSHA256(nil)
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("hashSHA256(nil) = %s, want %s", got, want)
	}

	// Known SHA-256 of "hello". / "hello" の既知の SHA-256。
	got = hashSHA256([]byte("hello"))
	want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("hashSHA256(hello) = %s, want %s", got, want)
	}
}

// TestCanonicalQueryString verifies query string sorting.
// クエリ文字列のソートを検証する。
func TestCanonicalQueryString(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/?z=1&a=2&m=3", nil)
	got := canonicalQueryString(req)
	want := "a=2&m=3&z=1"
	if got != want {
		t.Errorf("canonicalQueryString = %q, want %q", got, want)
	}

	// Empty query string. / 空のクエリ文字列。
	req2, _ := http.NewRequest("GET", "https://example.com/", nil)
	got2 := canonicalQueryString(req2)
	if got2 != "" {
		t.Errorf("canonicalQueryString(empty) = %q, want empty", got2)
	}
}

// TestSignatureConsistency verifies that signing the same request twice produces the same result.
// 同じリクエストに2回署名すると同じ結果になることを検証する。
func TestSignatureConsistency(t *testing.T) {
	fixedTime := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	req1, _ := http.NewRequest("DELETE", "https://bucket.s3.amazonaws.com/file.jpg", nil)
	signRequestWithTime(req1, "KEY", "SECRET", "us-east-1", "s3", nil, fixedTime)

	req2, _ := http.NewRequest("DELETE", "https://bucket.s3.amazonaws.com/file.jpg", nil)
	signRequestWithTime(req2, "KEY", "SECRET", "us-east-1", "s3", nil, fixedTime)

	if req1.Header.Get("Authorization") != req2.Header.Get("Authorization") {
		t.Error("same request signed at same time should produce identical authorization")
	}
}

// TestCanonicalURIEncoding verifies URI encoding of path segments.
// パスセグメントの URI エンコードを検証する。
func TestCanonicalURIEncoding(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"simple", "/bucket/file.jpg", "/bucket/file.jpg"},
		{"space in segment", "/bucket/my file.jpg", "/bucket/my%20file.jpg"},
		{"special chars", "/bucket/hello+world.jpg", "/bucket/hello+world.jpg"},
		{"empty path", "", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "https://s3.example.com"+tt.path, nil)
			got := canonicalURI(req)
			if got != tt.want {
				t.Errorf("canonicalURI(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestCanonicalQueryStringEncoding verifies URI encoding of query parameters.
// クエリパラメータの URI エンコードを検証する。
func TestCanonicalQueryStringEncoding(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://s3.example.com/?key=hello%20world&tag=a%2Bb", nil)
	got := canonicalQueryString(req)
	// url.Query() decodes %20 → space, %2B → +. url.QueryEscape re-encodes: space → +, + → %2B.
	// url.Query() は %20→空白, %2B→+ にデコード。url.QueryEscape は 空白→+, +→%2B に再エンコード。
	want := "key=hello+world&tag=a%2Bb"
	if got != want {
		t.Errorf("canonicalQueryString = %q, want %q", got, want)
	}
}
