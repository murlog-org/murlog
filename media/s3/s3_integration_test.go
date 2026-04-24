package s3

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestIntegrationS3 tests the S3 store against a real S3-compatible service.
// 実際の S3 互換サービスに対して S3 ストアをテストする。
//
// Run with: go test -v -run TestIntegrationS3 -tags integration ./media/s3/
// Requires MURLOG_S3_* environment variables to be set.
// MURLOG_S3_* 環境変数が設定されている必要がある。
func TestIntegrationS3(t *testing.T) {
	bucket := os.Getenv("MURLOG_S3_BUCKET")
	if bucket == "" {
		t.Skip("MURLOG_S3_BUCKET not set, skipping integration test")
	}

	store := New(Options{
		Bucket:    bucket,
		Region:    os.Getenv("MURLOG_S3_REGION"),
		Endpoint:  os.Getenv("MURLOG_S3_ENDPOINT"),
		AccessKey: os.Getenv("MURLOG_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("MURLOG_S3_SECRET_KEY"),
		PublicURL: os.Getenv("MURLOG_S3_PUBLIC_URL"),
	})

	const testFile = "test-murlog-s3.txt"
	const testBody = "hello from murlog s3 integration test"

	// 1. Save — アップロード
	t.Run("Save", func(t *testing.T) {
		err := store.Save(testFile, strings.NewReader(testBody))
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		t.Log("Save OK")
	})

	// 2. URL — 公開 URL が取得できるか確認
	t.Run("URL", func(t *testing.T) {
		u := store.URL(testFile)
		t.Logf("Public URL: %s", u)

		resp, err := http.Get(u)
		if err != nil {
			t.Fatalf("GET public URL failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET public URL: status %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if string(body) != testBody {
			t.Errorf("body mismatch: got %q, want %q", string(body), testBody)
		}
		t.Log("Public URL OK — content matches")
	})

	// 3. Open — S3 API 経由でダウンロード
	t.Run("Open", func(t *testing.T) {
		rc, err := store.Open(testFile)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer rc.Close()

		body, _ := io.ReadAll(rc)
		if string(body) != testBody {
			t.Errorf("body mismatch: got %q, want %q", string(body), testBody)
		}
		t.Log("Open OK — content matches")
	})

	// 4. Delete — 削除
	t.Run("Delete", func(t *testing.T) {
		err := store.Delete(testFile)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		t.Log("Delete OK")
	})
}
