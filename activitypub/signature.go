package activitypub

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// SignRequest signs an HTTP request using HTTP Signatures (draft-cavage-http-signatures).
// Uses RSA-SHA256 for maximum compatibility with other ActivityPub implementations.
// HTTP Signatures (draft-cavage-http-signatures) でリクエストに署名する。
// 他の ActivityPub 実装との互換性のため RSA-SHA256 を使用。
//
// keyID: the key identifier (e.g. "https://example.com/users/alice#main-key")
// privateKeyPEM: RSA private key in PEM format
func SignRequest(r *http.Request, keyID, privateKeyPEM string, body []byte) error {
	privKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return fmt.Errorf("activitypub: parse private key: %w", err)
	}

	// Set required headers. / 必要なヘッダを設定。
	now := time.Now().UTC()
	r.Header.Set("Date", now.Format(http.TimeFormat))
	r.Header.Set("Host", r.URL.Host)

	if body != nil {
		digest := sha256.Sum256(body)
		r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(digest[:]))
	}

	// Build signing string. / 署名文字列を構築。
	headers := []string{"(request-target)", "host", "date"}
	if body != nil {
		headers = append(headers, "digest")
	}

	signingString := buildSigningString(r, headers)

	// Sign with RSA-SHA256. / RSA-SHA256 で署名。
	hash := sha256.Sum256([]byte(signingString))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hash[:])
	if err != nil {
		return fmt.Errorf("activitypub: sign: %w", err)
	}

	// Build Signature header. / Signature ヘッダを構築。
	sigHeader := fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID, strings.Join(headers, " "), base64.StdEncoding.EncodeToString(sig))
	r.Header.Set("Signature", sigHeader)

	return nil
}

// VerifyRequest verifies the HTTP Signature on an incoming request.
// Currently uses RSA-SHA256 (the de facto standard in ActivityPub).
// Key parsing is generic (x509.ParsePKIXPublicKey), so other key types
// may work if the ecosystem adopts them in the future.
// 受信リクエストの HTTP Signature を検証する。
// 現在は RSA-SHA256 を使用 (ActivityPub の事実上の標準)。
// 鍵パースは汎用的 (x509.ParsePKIXPublicKey) なため、
// 将来エコシステムが他の鍵種別を採用した場合にも対応可能。
//
// publicKeyPEM: public key in PEM format
func VerifyRequest(r *http.Request, publicKeyPEM string, body []byte) error {
	// Parse Signature header. / Signature ヘッダをパース。
	sigParams, err := parseSignatureHeader(r.Header.Get("Signature"))
	if err != nil {
		return err
	}

	// algorithm が指定されている場合、許可リストで検証する。
	// Validate algorithm field against allowlist when present.
	if alg, ok := sigParams["algorithm"]; ok {
		switch alg {
		case "rsa-sha256", "hs2019", "ed25519":
			// OK
		default:
			return fmt.Errorf("activitypub: unsupported signature algorithm %q", alg)
		}
	}

	headers := strings.Split(sigParams["headers"], " ")

	// Require "date" in signed headers (Mastodon compatibility).
	// 署名ヘッダーに "date" を必須とする (Mastodon 互換)。
	if !containsHeader(headers, "date") {
		return fmt.Errorf("activitypub: signature must include date header")
	}

	// H2: Date チェックを暗号検証の前に実行し、期限切れリクエストの高コスト演算を回避。
	// H2: Check Date before crypto verification to skip expensive ops on expired requests.
	if err := verifyDateWindow(r); err != nil {
		return err
	}

	// H1: Digest ヘッダとボディの SHA-256 が一致するか検証する。
	// H1: Verify that the Digest header matches the actual body SHA-256.
	if err := VerifyDigest(r, body); err != nil {
		return err
	}

	// M5: 署名対象ヘッダがリクエストに存在しない場合はエラーを返す。
	// M5: Reject if a signed header is missing from the request.
	signingString, err := buildSigningStringStrict(r, headers)
	if err != nil {
		return err
	}

	sig, err := base64.StdEncoding.DecodeString(sigParams["signature"])
	if err != nil {
		return fmt.Errorf("activitypub: decode signature: %w", err)
	}

	// Detect key type and verify accordingly.
	// 鍵の種別を判定し、対応するアルゴリズムで検証する。
	pubKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return fmt.Errorf("activitypub: parse public key: %w", err)
	}

	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		hash := sha256.Sum256([]byte(signingString))
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sig); err != nil {
			return fmt.Errorf("activitypub: RSA verify failed: %w", err)
		}
	case ed25519.PublicKey:
		// Ed25519 signs the raw message, not a hash.
		// Ed25519 はハッシュではなく生メッセージに署名する。
		if !ed25519.Verify(key, []byte(signingString), sig) {
			return fmt.Errorf("activitypub: Ed25519 verify failed")
		}
	default:
		return fmt.Errorf("activitypub: unsupported key type %T", pubKey)
	}

	return nil
}

// ParseSignatureKeyID extracts the keyId from a Signature header.
// Signature ヘッダから keyId を取り出す。
func ParseSignatureKeyID(sigHeader string) (string, error) {
	params, err := parseSignatureHeader(sigHeader)
	if err != nil {
		return "", err
	}
	keyID, ok := params["keyId"]
	if !ok {
		return "", fmt.Errorf("activitypub: missing keyId in Signature header")
	}
	return keyID, nil
}

// GenerateKeyPair generates an RSA key pair and returns PEM-encoded strings.
// Outgoing signatures use RSA for compatibility with Mastodon etc.
// RSA 鍵ペアを生成し、PEM エンコードされた文字列を返す。
// 送信署名には Mastodon 等との互換性のため RSA を使用。
func GenerateKeyPair() (publicKeyPEM, privateKeyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	pubASN1, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	return string(pubPEM), string(privPEM), nil
}

// verifyDigest checks that the Digest header matches the actual body SHA-256.
// Digest ヘッダと実際のボディの SHA-256 が一致するか検証する。
func VerifyDigest(r *http.Request, body []byte) error {
	digestHeader := r.Header.Get("Digest")
	if digestHeader == "" {
		// GET リクエスト等、Digest ヘッダがない場合はスキップ。
		// Skip for requests without Digest header (e.g. GET).
		return nil
	}
	const prefix = "SHA-256="
	if !strings.HasPrefix(digestHeader, prefix) {
		return fmt.Errorf("activitypub: unsupported Digest algorithm: %s", digestHeader)
	}
	expected := digestHeader[len(prefix):]
	actual := sha256.Sum256(body)
	actualB64 := base64.StdEncoding.EncodeToString(actual[:])
	if expected != actualB64 {
		return fmt.Errorf("activitypub: Digest mismatch")
	}
	return nil
}

// buildSigningStringStrict builds the signing string and rejects missing headers.
// 署名文字列を構築し、欠落ヘッダがあればエラーを返す。
func buildSigningStringStrict(r *http.Request, headers []string) (string, error) {
	var lines []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			lines = append(lines, fmt.Sprintf("(request-target): %s %s",
				strings.ToLower(r.Method), r.URL.RequestURI()))
		case "host":
			host := r.Header.Get("Host")
			if host == "" {
				host = r.Host
			}
			if host == "" {
				return "", fmt.Errorf("activitypub: signed header %q not present in request", h)
			}
			lines = append(lines, fmt.Sprintf("host: %s", host))
		default:
			val := r.Header.Get(http.CanonicalHeaderKey(h))
			if val == "" {
				return "", fmt.Errorf("activitypub: signed header %q not present in request", h)
			}
			lines = append(lines, fmt.Sprintf("%s: %s", h, val))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// buildSigningString builds the signing string per the HTTP Signatures spec.
// HTTP Signatures 仕様に従い署名文字列を構築する。
func buildSigningString(r *http.Request, headers []string) string {
	var lines []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			lines = append(lines, fmt.Sprintf("(request-target): %s %s",
				strings.ToLower(r.Method), r.URL.RequestURI()))
		case "host":
			// Go moves Host header to r.Host; r.Header.Get("Host") is empty on server side.
			// Go はサーバー側で Host ヘッダーを r.Host に移動するため r.Header.Get("Host") は空。
			host := r.Header.Get("Host")
			if host == "" {
				host = r.Host
			}
			lines = append(lines, fmt.Sprintf("host: %s", host))
		default:
			lines = append(lines, fmt.Sprintf("%s: %s", h, r.Header.Get(http.CanonicalHeaderKey(h))))
		}
	}
	return strings.Join(lines, "\n")
}

// sigParamRe matches key="value" pairs in a Signature header.
// Handles quoted values correctly (commas inside quotes are not treated as separators).
// Signature ヘッダの key="value" ペアにマッチする正規表現。
// 引用符内のカンマをセパレータとして扱わない。
var sigParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// parseSignatureHeader parses a Signature header value into key-value pairs.
// Signature ヘッダ値をキーと値のペアにパースする。
func parseSignatureHeader(header string) (map[string]string, error) {
	if header == "" {
		return nil, fmt.Errorf("activitypub: empty Signature header")
	}
	matches := sigParamRe.FindAllStringSubmatch(header, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("activitypub: no params in Signature header")
	}
	params := make(map[string]string, len(matches))
	for _, m := range matches {
		params[m[1]] = m[2]
	}
	return params, nil
}

// parsePublicKey parses a PEM-encoded public key (RSA or Ed25519).
// PEM エンコードされた公開鍵をパースする (RSA または Ed25519)。
func parsePublicKey(pemStr string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParsePKIXPublicKey(block.Bytes)
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// signatureMaxAge is the maximum age of a signed request (Mastodon-compatible).
// EXPIRATION_WINDOW_LIMIT (12h) + CLOCK_SKEW_MARGIN (1h) = 13h.
// 署名付きリクエストの最大許容時間 (Mastodon 互換)。
const signatureMaxAge = 13 * time.Hour

// verifyDateWindow checks that the Date header is within the acceptable time window.
// Date ヘッダーが許容時間窓内かを検証する。
func verifyDateWindow(r *http.Request) error {
	dateStr := r.Header.Get("Date")
	if dateStr == "" {
		return fmt.Errorf("activitypub: missing Date header")
	}
	date, err := http.ParseTime(dateStr)
	if err != nil {
		return fmt.Errorf("activitypub: parse Date header: %w", err)
	}
	diff := time.Since(date)
	if diff < 0 {
		diff = -diff
	}
	if diff > signatureMaxAge {
		return fmt.Errorf("activitypub: request Date too far from current time (%v)", diff)
	}
	return nil
}

// containsHeader checks if a header name exists in the signed headers list (case-insensitive).
// 署名ヘッダーリスト内にヘッダー名が存在するか確認する (大文字小文字無視)。
func containsHeader(headers []string, name string) bool {
	for _, h := range headers {
		if strings.EqualFold(h, name) {
			return true
		}
	}
	return false
}
