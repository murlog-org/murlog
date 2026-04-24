package totp

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSecret(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) == 0 {
		t.Fatal("secret is empty")
	}
	// Base32 encoded 20 bytes = 32 chars (without padding).
	if len(secret) != 32 {
		t.Errorf("secret length = %d, want 32", len(secret))
	}

	// Two secrets should be different. / 2回生成して異なることを確認。
	secret2, _ := GenerateSecret()
	if secret == secret2 {
		t.Error("two generated secrets should differ")
	}
}

func TestValidate(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Now()

	// Generate the current code. / 現在のコードを生成。
	counter := now.Unix() / Period
	code := generateCode(secret, counter)

	if len(code) != Digits {
		t.Fatalf("code length = %d, want %d", len(code), Digits)
	}

	// Valid at current time. / 現在時刻で有効。
	if !ValidateAt(secret, code, now) {
		t.Error("code should be valid at current time")
	}

	// Invalid code. / 無効なコード。
	if ValidateAt(secret, "000000", now) {
		// Could theoretically match, but astronomically unlikely.
		t.Log("warning: 000000 matched (statistically possible but unlikely)")
	}

	// Valid within window. / ウィンドウ内で有効。
	prevCode := generateCode(secret, counter-1)
	if !ValidateAt(secret, prevCode, now) {
		t.Error("previous period code should be valid within window")
	}

	// Invalid outside window. / ウィンドウ外で無効。
	oldCode := generateCode(secret, counter-5)
	if ValidateAt(secret, oldCode, now) {
		t.Error("code from 5 periods ago should be invalid")
	}
}

func TestURI(t *testing.T) {
	uri := URI("JBSWY3DPEHPK3PXP", "murlog", "alice@example.com")

	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("URI should start with otpauth://totp/, got %q", uri)
	}
	if !strings.Contains(uri, "secret=JBSWY3DPEHPK3PXP") {
		t.Error("URI should contain the secret")
	}
	if !strings.Contains(uri, "issuer=murlog") {
		t.Error("URI should contain the issuer")
	}
	if !strings.Contains(uri, "alice") {
		t.Error("URI should contain the account")
	}
}
