// Package s3 implements media.Store using S3-compatible object storage.
// S3 互換オブジェクトストレージによる media.Store の実装。
package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// signRequest adds AWS Signature V4 headers to an HTTP request.
// HTTP リクエストに AWS Signature V4 署名ヘッダーを付与する。
func signRequest(req *http.Request, accessKey, secretKey, region, service string, body []byte) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	// Set required headers. / 必須ヘッダーを設定。
	req.Header.Set("x-amz-date", amzdate)
	req.Header.Set("x-amz-content-sha256", hashSHA256(body))

	// Build canonical request. / 正規リクエストを構築。
	canonicalHeaders, signedHeaders := buildCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req),
		canonicalQueryString(req),
		canonicalHeaders,
		signedHeaders,
		hashSHA256(body),
	}, "\n")

	// Build string to sign. / 署名対象文字列を構築。
	scope := datestamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzdate,
		scope,
		hashSHA256([]byte(canonicalRequest)),
	}, "\n")

	// Calculate signature. / 署名を計算。
	signingKey := deriveSigningKey(secretKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Set Authorization header. / Authorization ヘッダーを設定。
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature,
	))
}

// canonicalURI returns the URI-encoded path component.
// M15: 各パスセグメントを URI エンコードする (AWS SigV4 仕様)。
// URI エンコードされたパスを返す。
func canonicalURI(req *http.Request) string {
	path := req.URL.Path
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// canonicalQueryString returns the sorted, URI-encoded query string.
// M15: キーと値を URI エンコードする (AWS SigV4 仕様)。
// ソート済み・URI エンコード済みクエリ文字列を返す。
func canonicalQueryString(req *http.Request) string {
	query := req.URL.Query()
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range query[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

// buildCanonicalHeaders returns canonical headers string and signed headers list.
// 正規ヘッダー文字列と署名対象ヘッダーリストを返す。
func buildCanonicalHeaders(req *http.Request) (canonical, signed string) {
	// Collect headers to sign: host + x-amz-*.
	// 署名対象ヘッダー: host + x-amz-*。
	headers := make(map[string]string)
	headers["host"] = req.Host
	if req.Host == "" {
		headers["host"] = req.URL.Host
	}
	for k, v := range req.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-amz-") || lower == "content-type" {
			headers[lower] = strings.TrimSpace(v[0])
		}
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalBuf, signedBuf strings.Builder
	for i, k := range keys {
		canonicalBuf.WriteString(k + ":" + headers[k] + "\n")
		if i > 0 {
			signedBuf.WriteString(";")
		}
		signedBuf.WriteString(k)
	}
	return canonicalBuf.String(), signedBuf.String()
}

// deriveSigningKey derives the signing key for AWS Signature V4.
// AWS Signature V4 の署名キーを導出する。
func deriveSigningKey(secretKey, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// hmacSHA256 computes HMAC-SHA256.
// HMAC-SHA256 を計算する。
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// hashSHA256 computes the hex-encoded SHA-256 hash.
// SHA-256 ハッシュの16進エンコードを計算する。
func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// signRequestWithTime is like signRequest but accepts a specific time (for testing).
// signRequest と同じだがテスト用に時刻を指定できる。
func signRequestWithTime(req *http.Request, accessKey, secretKey, region, service string, body []byte, now time.Time) {
	now = now.UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzdate)
	req.Header.Set("x-amz-content-sha256", hashSHA256(body))

	canonicalHeaders, signedHeaders := buildCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req),
		canonicalQueryString(req),
		canonicalHeaders,
		signedHeaders,
		hashSHA256(body),
	}, "\n")

	scope := datestamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzdate,
		scope,
		hashSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(secretKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature,
	))
}

// signBodyless signs a request that has no body (GET, DELETE).
// ボディなしのリクエスト (GET, DELETE) に署名する。
func signBodyless(req *http.Request, accessKey, secretKey, region, service string) {
	signRequest(req, accessKey, secretKey, region, service, nil)
}

// signWithBody reads the body and signs the request.
// ボディを読み取ってリクエストに署名する。
func signWithBody(req *http.Request, accessKey, secretKey, region, service string, body []byte) {
	signRequest(req, accessKey, secretKey, region, service, body)
}

// signBodylessWithTime signs a bodyless request at a specific time (for testing).
// テスト用: ボディなしリクエストに特定時刻で署名する。
func signBodylessWithTime(req *http.Request, accessKey, secretKey, region, service string, now time.Time) {
	signRequestWithTime(req, accessKey, secretKey, region, service, nil, now)
}

// maxUploadSize is the maximum upload size for S3 (10 MB).
// S3 アップロードの最大サイズ (10 MB)。
const maxUploadSize = 10 << 20

// readBody reads the body from a reader with a size limit (used by s3Store.Save).
// サイズ制限付きでリーダーからボディを読み取る (s3Store.Save で使用)。
func readBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxUploadSize))
}
