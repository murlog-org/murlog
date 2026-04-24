package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndOpen(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "/media")

	// Save / 保存
	if err := s.Save("attachments/test.jpg", strings.NewReader("jpeg-data")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File should exist on disk. / ファイルがディスク上に存在すること。
	path := filepath.Join(dir, "attachments", "test.jpg")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not found: %v", err)
	}

	// Open / 開く
	rc, err := s.Open("attachments/test.jpg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "jpeg-data" {
		t.Fatalf("Open: got %q, want %q", data, "jpeg-data")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "/media")

	s.Save("attachments/del.jpg", strings.NewReader("data"))

	if err := s.Delete("attachments/del.jpg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "attachments", "del.jpg")); !os.IsNotExist(err) {
		t.Fatal("file should not exist after Delete")
	}
}

func TestURL(t *testing.T) {
	s := New("/tmp/media", "/media")
	got := s.URL("attachments/abc.jpg")
	want := "/media/attachments/abc.jpg"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "/media")

	tests := []string{
		"../../etc/passwd",
		"../../../tmp/evil",
		"attachments/../../etc/shadow",
	}

	for _, name := range tests {
		// Save should fail. / Save が失敗すること。
		if err := s.Save(name, strings.NewReader("bad")); err == nil {
			t.Errorf("Save(%q) should fail (path traversal)", name)
		}

		// Open should fail. / Open が失敗すること。
		if _, err := s.Open(name); err == nil {
			t.Errorf("Open(%q) should fail (path traversal)", name)
		}

		// Delete should fail. / Delete が失敗すること。
		if err := s.Delete(name); err == nil {
			t.Errorf("Delete(%q) should fail (path traversal)", name)
		}
	}
}

func TestSaveCreatesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "/media")

	// Nested path should auto-create directories.
	// ネストされたパスでディレクトリが自動作成されること。
	if err := s.Save("avatars/deep/nested/file.png", strings.NewReader("png")); err != nil {
		t.Fatalf("Save nested: %v", err)
	}

	path := filepath.Join(dir, "avatars", "deep", "nested", "file.png")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested file not found: %v", err)
	}
}

// errReader is an io.Reader that always returns an error.
// 常にエラーを返す io.Reader。
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read error") }

func TestSaveCleansUpOnError(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "/media")

	// L9: 書き込みエラー時に不完全ファイルが残らないことを検証。
	// L9: Verify incomplete file is removed on write error.
	err := s.Save("attachments/fail.jpg", errReader{})
	if err == nil {
		t.Fatal("Save should fail with errReader")
	}

	path := filepath.Join(dir, "attachments", "fail.jpg")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("incomplete file should be removed on Save error")
	}
}
