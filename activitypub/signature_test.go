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
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRSA(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest("POST", "https://remote.example/users/bob/inbox", strings.NewReader(string(body)))

	keyID := "https://example.com/users/alice#main-key"
	if err := SignRequest(req, keyID, privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	sigHeader := req.Header.Get("Signature")
	if sigHeader == "" {
		t.Fatal("Signature header is empty")
	}

	gotKeyID, err := ParseSignatureKeyID(sigHeader)
	if err != nil {
		t.Fatalf("ParseSignatureKeyID: %v", err)
	}
	if gotKeyID != keyID {
		t.Fatalf("keyId = %q, want %q", gotKeyID, keyID)
	}

	if err := VerifyRequest(req, pubPEM, body); err != nil {
		t.Fatalf("VerifyRequest: %v", err)
	}
}

func TestSignAndVerifyWithoutBody(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://remote.example/users/bob", nil)

	if err := SignRequest(req, "https://example.com/users/alice#main-key", privPEM, nil); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// GET should not have Digest header. / GET には Digest ヘッダがないこと。
	if req.Header.Get("Digest") != "" {
		t.Fatal("GET request should not have Digest header")
	}

	if err := VerifyRequest(req, pubPEM, nil); err != nil {
		t.Fatalf("VerifyRequest: %v", err)
	}
}

func TestVerifyTamperedRequest(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest("POST", "https://remote.example/users/bob/inbox", strings.NewReader(string(body)))

	if err := SignRequest(req, "https://example.com/users/alice#main-key", privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// Tamper with Date header. / Date ヘッダを改竄。
	req.Header.Set("Date", "Mon, 01 Jan 2024 00:00:00 GMT")

	if err := VerifyRequest(req, pubPEM, body); err == nil {
		t.Fatal("VerifyRequest should fail for tampered request")
	}
}

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if !strings.Contains(pub, "PUBLIC KEY") {
		t.Fatal("public key PEM missing header")
	}
	if !strings.Contains(priv, "RSA PRIVATE KEY") {
		t.Fatal("private key PEM missing header")
	}
}

func TestVerifyEd25519(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	pubASN1, _ := x509.MarshalPKIXPublicKey(pubKey)
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubASN1}))

	// Build a signed request manually (Ed25519).
	// Ed25519 で手動署名したリクエストを構築。
	req, _ := http.NewRequest("GET", "https://remote.example/users/bob", nil)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Host", "remote.example")

	headers := []string{"(request-target)", "host", "date"}
	signingString := buildSigningString(req, headers)
	sig := ed25519.Sign(privKey, []byte(signingString))

	sigHeader := `keyId="https://example.com/users/alice#main-key",algorithm="ed25519",headers="(request-target) host date",signature="` +
		base64Encode(sig) + `"`
	req.Header.Set("Signature", sigHeader)

	if err := VerifyRequest(req, pubPEM, nil); err != nil {
		t.Fatalf("VerifyRequest (Ed25519): %v", err)
	}
}

func TestVerifyRejectsExpiredDate(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Manually build a request with a Date 14 hours ago and sign it.
	// 14 時間前の Date で手動署名したリクエストを構築。
	req, _ := http.NewRequest("GET", "https://remote.example/users/bob", nil)
	oldDate := time.Now().UTC().Add(-14 * time.Hour).Format(http.TimeFormat)
	req.Header.Set("Date", oldDate)
	req.Header.Set("Host", "remote.example")

	headers := []string{"(request-target)", "host", "date"}
	signingString := buildSigningString(req, headers)

	privKey, _ := parseRSAPrivateKey(privPEM)
	hash := sha256Sum([]byte(signingString))
	sig, _ := rsaSignHelper(privKey, hash)

	sigHeader := `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="` +
		base64Encode(sig) + `"`
	req.Header.Set("Signature", sigHeader)

	err = VerifyRequest(req, pubPEM, nil)
	if err == nil {
		t.Fatal("VerifyRequest should reject expired Date")
	}
	if !strings.Contains(err.Error(), "too far") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyRejectsMissingDateInSignedHeaders(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Manually build a request signed without "date" in headers.
	req, _ := http.NewRequest("GET", "https://remote.example/users/bob", nil)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Host", "remote.example")

	// Sign with only (request-target) and host (no date).
	headers := []string{"(request-target)", "host"}
	signingString := buildSigningString(req, headers)

	privKey, _ := parseRSAPrivateKey(privPEM)
	hash := sha256Sum([]byte(signingString))
	sig, _ := rsaSignHelper(privKey, hash)

	sigHeader := `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host",signature="` +
		base64Encode(sig) + `"`
	req.Header.Set("Signature", sigHeader)

	err = VerifyRequest(req, pubPEM, nil)
	if err == nil {
		t.Fatal("VerifyRequest should reject signature without date header")
	}
	if !strings.Contains(err.Error(), "must include date") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSignatureHeader(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   map[string]string
		hasErr bool
	}{
		{
			name:  "standard",
			input: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="abc123=="`,
			want: map[string]string{
				"keyId":     "https://example.com/users/alice#main-key",
				"algorithm": "rsa-sha256",
				"headers":   "(request-target) host date",
				"signature": "abc123==",
			},
		},
		{
			name:  "hs2019 algorithm",
			input: `keyId="https://gts.example/users/bob/main-key",algorithm="hs2019",headers="(request-target) host date digest",signature="xyz789"`,
			want: map[string]string{
				"keyId":     "https://gts.example/users/bob/main-key",
				"algorithm": "hs2019",
				"headers":   "(request-target) host date digest",
				"signature": "xyz789",
			},
		},
		{
			name:   "empty",
			input:  "",
			hasErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSignatureHeader(tt.input)
			if tt.hasErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("invalid IP: %s", tt.ip)
		}
		got := isPrivateIP(ip)
		if got != tt.private {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestVerifyRejectsDigestMismatch(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest("POST", "https://remote.example/users/bob/inbox", strings.NewReader(string(body)))
	if err := SignRequest(req, "https://example.com/users/alice#main-key", privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// Pass a different body to VerifyRequest (body tampering).
	// VerifyRequest に異なるボディを渡す (ボディ改竄)。
	tamperedBody := []byte(`{"type":"Delete"}`)
	if err := VerifyRequest(req, pubPEM, tamperedBody); err == nil {
		t.Fatal("VerifyRequest should reject Digest mismatch")
	}
}

func TestVerifyRejectsMissingSignedHeader(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest("POST", "https://remote.example/users/bob/inbox", strings.NewReader(string(body)))
	if err := SignRequest(req, "https://example.com/users/alice#main-key", privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// Remove Digest header — signature includes "digest" but header is absent.
	// Digest ヘッダを削除 — 署名に "digest" が含まれるがヘッダが不在。
	req.Header.Del("Digest")

	if err := VerifyRequest(req, pubPEM, body); err == nil {
		t.Fatal("VerifyRequest should reject missing signed header")
	}
}

func TestVerifyRejectsUnsupportedDigestAlgorithm(t *testing.T) {
	pubPEM, privPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest("POST", "https://remote.example/users/bob/inbox", strings.NewReader(string(body)))
	if err := SignRequest(req, "https://example.com/users/alice#main-key", privPEM, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// Replace SHA-256 digest with unsupported algorithm.
	// 未対応アルゴリズムの Digest に差し替え。
	req.Header.Set("Digest", "SHA-512=abc123")

	if err := VerifyRequest(req, pubPEM, body); err == nil {
		t.Fatal("VerifyRequest should reject unsupported Digest algorithm")
	}
}

// helpers for manual signature construction
// 手動署名構築用ヘルパー

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func rsaSignHelper(key *rsa.PrivateKey, hash []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash)
}
