//go:build sqlite || all_stores || (!mysql && !postgres)

package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestLoginAttemptFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ip := "192.168.1.1"

	// Initially no record. / 初期状態はレコードなし。
	failCount, lockedUntil, err := s.GetLoginAttempt(ctx, ip)
	if err != nil {
		t.Fatalf("GetLoginAttempt: %v", err)
	}
	if failCount != 0 {
		t.Errorf("initial failCount = %d, want 0", failCount)
	}
	if !lockedUntil.IsZero() {
		t.Errorf("initial lockedUntil should be zero")
	}

	// Record failures without lock. / ロックなしで失敗を記録。
	for i := 0; i < 4; i++ {
		if err := s.RecordLoginFailure(ctx, ip, time.Time{}); err != nil {
			t.Fatalf("RecordLoginFailure %d: %v", i, err)
		}
	}

	failCount, _, _ = s.GetLoginAttempt(ctx, ip)
	if failCount != 4 {
		t.Errorf("failCount = %d, want 4", failCount)
	}

	// 5th failure with lock. / 5回目の失敗でロック。
	lockTime := time.Now().Add(5 * time.Minute)
	if err := s.RecordLoginFailure(ctx, ip, lockTime); err != nil {
		t.Fatalf("RecordLoginFailure with lock: %v", err)
	}

	failCount, lockedUntil, _ = s.GetLoginAttempt(ctx, ip)
	if failCount != 5 {
		t.Errorf("failCount = %d, want 5", failCount)
	}
	if lockedUntil.IsZero() {
		t.Error("lockedUntil should be set after lock")
	}
	if time.Now().After(lockedUntil) {
		t.Error("lockedUntil should be in the future")
	}

	// Clear on success. / 成功時にクリア。
	if err := s.ClearLoginAttempts(ctx, ip); err != nil {
		t.Fatalf("ClearLoginAttempts: %v", err)
	}

	failCount, lockedUntil, _ = s.GetLoginAttempt(ctx, ip)
	if failCount != 0 {
		t.Errorf("failCount after clear = %d, want 0", failCount)
	}
	if !lockedUntil.IsZero() {
		t.Error("lockedUntil should be zero after clear")
	}
}
