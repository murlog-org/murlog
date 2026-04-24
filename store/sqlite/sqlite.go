//go:build sqlite || all_stores || (!mysql && !postgres)

// Package sqlite implements store.Store using SQLite (modernc.org/sqlite).
// SQLite (modernc.org/sqlite) による store.Store の実装。
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/internal/sqlutil"
	"github.com/murlog-org/murlog/store"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func init() {
	store.Register("sqlite", func(dsn string) (store.Store, error) {
		return New(dsn)
	})
}

type sqliteStore struct {
	db  *sql.DB
	dsn string
}

// New creates a new SQLite-backed store.
// SQLite はシングルファイル DB なので、コネクションプールは1に制限する。
// database/sql のデフォルト (無制限) だと、接続ごとに別の DB が見える問題が起きる。
// SQLite バックエンドの Store を生成する。
func New(dsn string) (store.Store, error) {
	// Set PRAGMAs via DSN so they are applied at connection time,
	// before any user queries. This avoids SQLITE_BUSY when setting
	// busy_timeout on a locked database during CGI concurrent startup.
	// PRAGMA を DSN 経由で接続時に即適用する。CGI 同時起動時に
	// busy_timeout 設定自体が SQLITE_BUSY で失敗するのを防ぐ。
	if dsn != ":memory:" && !strings.Contains(dsn, "_pragma=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "_txlock=immediate&_pragma=busy_timeout(100)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Limit to a single connection — SQLite does not benefit from multiple
	// connections and database/sql may open separate in-memory databases
	// when the file cannot be created (e.g. under Playwright webServer).
	// コネクションを1つに制限 — SQLite は複数接続の恩恵がなく、
	// ファイル未作成時にプール内で別々のインメモリ DB が生まれる問題を防ぐ。
	db.SetMaxOpenConns(1)
	return &sqliteStore{db: db, dsn: dsn}, nil
}

// Migrate runs database migrations sequentially.
// Tracks applied versions in schema_version table.
// If pending migrations exist and the DB is a file (not :memory:),
// a pre-migration backup is created via VACUUM INTO.
// データベースマイグレーションを順番に実行する。
// 適用済みバージョンを schema_version テーブルで管理。
// 適用すべきマイグレーションがある場合、ファイル DB なら
// VACUUM INTO でマイグレーション前バックアップを作成する。
func (s *sqliteStore) Migrate(ctx context.Context) error {
	// Collect pending migrations first (read-only).
	// まず適用待ちマイグレーションを収集する (読み取りのみ)。
	pending, err := s.pendingMigrations(ctx)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	// Backup DB before applying migrations (file DB only, best-effort).
	// VACUUM INTO cannot run inside a transaction, so run it before.
	// マイグレーション適用前に DB をバックアップする (ファイル DB のみ、ベストエフォート)。
	// VACUUM INTO はトランザクション内で実行できないため、先に実行する。
	s.backupBeforeMigrate(ctx)

	// Acquire exclusive lock via IMMEDIATE transaction to prevent
	// concurrent CGI processes from running migrations simultaneously.
	// IMMEDIATE トランザクションで排他ロックを取得し、
	// CGI 同時起動でマイグレーションが重複実行されるのを防ぐ。
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migrate tx: %w", err)
	}

	// Re-check current version inside transaction (another process may have migrated).
	// トランザクション内で再確認 (他プロセスが先にマイグレーション済みの可能性)。
	var current int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		tx.Rollback()
		return fmt.Errorf("get schema version: %w", err)
	}

	applied := 0
	for _, m := range pending {
		if m.version <= current {
			continue
		}
		if _, err := tx.ExecContext(ctx, string(m.data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
		applied++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrate: %w", err)
	}
	if applied > 0 {
		log.Printf("migrate: applied %d migration(s)", applied)
	}
	return nil
}

// migration holds a parsed migration file.
// パース済みマイグレーションファイルを保持する。
type migration struct {
	version int
	name    string
	data    []byte
}

// pendingMigrations returns migrations not yet applied to the database.
// まだ適用されていないマイグレーションを返す。
func (s *sqliteStore) pendingMigrations(ctx context.Context) ([]migration, error) {
	// Ensure schema_version table exists.
	// schema_version テーブルの存在を保証する。
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	)`); err != nil {
		return nil, fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return nil, fmt.Errorf("get schema version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var pending []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Parse version from filename (e.g. "001_init.sql" → 1).
		// ファイル名からバージョンを取得 (例: "001_init.sql" → 1)。
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			continue
		}
		if version <= current {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		pending = append(pending, migration{version: version, name: entry.Name(), data: data})
	}
	return pending, nil
}

// backupBeforeMigrate creates a pre-migration backup of the database.
// Errors are logged but do not block migration (best-effort).
// マイグレーション前の DB バックアップを作成する。
// エラーはログに記録するがマイグレーションはブロックしない (ベストエフォート)。
func (s *sqliteStore) backupBeforeMigrate(ctx context.Context) {
	dbPath := s.dbPath()
	if dbPath == "" {
		return
	}
	dest := filepath.Join(filepath.Dir(dbPath), "murlog-premigrate.db")
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`VACUUM INTO '%s'`, dest)); err != nil {
		log.Printf("migrate: pre-migration backup failed: %v", err)
		return
	}
	log.Printf("migrate: pre-migration backup saved to %s", dest)
}

// dbPath extracts the file path from the DSN. Returns "" for in-memory DBs.
// DSN からファイルパスを抽出する。インメモリ DB の場合は "" を返す。
func (s *sqliteStore) dbPath() string {
	dsn := s.dsn
	if dsn == ":memory:" || strings.HasPrefix(dsn, ":memory:?") {
		return ""
	}
	if i := strings.IndexByte(dsn, '?'); i >= 0 {
		dsn = dsn[:i]
	}
	return dsn
}

// Close closes the database connection.
// データベース接続を閉じる。
func (s *sqliteStore) DB() *sql.DB {
	return s.db
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

// CountLocalPosts returns the number of locally-authored posts.
// ローカル投稿の数を返す。
func (s *sqliteStore) CountLocalPosts(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts WHERE origin = 'local'`).Scan(&n)
	return n, err
}

// CountLocalPostsByPersona returns the number of locally-authored posts for a persona.
// ペルソナのローカル投稿数を返す。
func (s *sqliteStore) CountLocalPostsByPersona(ctx context.Context, personaID id.ID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts WHERE persona_id = ? AND origin = 'local'`, personaID.Bytes()).Scan(&n)
	return n, err
}

// CountPersonas returns the number of personas.
// ペルソナの数を返す。
func (s *sqliteStore) CountPersonas(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM personas`).Scan(&n)
	return n, err
}

// CountFollowers returns the number of followers for a persona.
// ペルソナのフォロワー数を返す。
func (s *sqliteStore) CountFollowers(ctx context.Context, personaID id.ID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM followers WHERE persona_id = ?`, personaID.Bytes()).Scan(&n)
	return n, err
}

// CountFollows returns the number of follows for a persona.
// ペルソナのフォロー数を返す。
func (s *sqliteStore) CountFollows(ctx context.Context, personaID id.ID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM follows WHERE persona_id = ?`, personaID.Bytes()).Scan(&n)
	return n, err
}

// PostInteractionCounts returns favourites/reblogs counts for the given post IDs.
// 指定した投稿 ID のいいね/リブログ数をバッチで返す。
func (s *sqliteStore) PostInteractionCounts(ctx context.Context, postIDs []id.ID) (map[id.ID]murlog.InteractionCounts, error) {
	result := make(map[id.ID]murlog.InteractionCounts, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs))
	for i, pid := range postIDs {
		placeholders[i] = "?"
		args[i] = pid.Bytes()
	}
	ph := strings.Join(placeholders, ",")

	// Favourites count. / いいね数。
	rows, err := s.db.QueryContext(ctx,
		`SELECT post_id, COUNT(*) FROM favourites WHERE post_id IN (`+ph+`) GROUP BY post_id`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rawID []byte
		var n int
		if err := rows.Scan(&rawID, &n); err != nil {
			rows.Close()
			return nil, err
		}
		pid, _ := id.FromBytes(rawID)
		c := result[pid]
		c.Favourites = n
		result[pid] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reblogs count. / リブログ数。
	rows, err = s.db.QueryContext(ctx,
		`SELECT post_id, COUNT(*) FROM reblogs WHERE post_id IN (`+ph+`) GROUP BY post_id`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rawID []byte
		var n int
		if err := rows.Scan(&rawID, &n); err != nil {
			rows.Close()
			return nil, err
		}
		pid, _ := id.FromBytes(rawID)
		c := result[pid]
		c.Reblogs = n
		result[pid] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// BackupTo creates a consistent DB snapshot at destPath using VACUUM INTO.
// VACUUM INTO はプレースホルダ非対応のため、パスにシングルクォートを含まないことを検証する。
// VACUUM INTO で destPath に整合性のある DB スナップショットを作成する。
func (s *sqliteStore) BackupTo(ctx context.Context, destPath string) error {
	if strings.ContainsRune(destPath, '\'') {
		return fmt.Errorf("backup: invalid character in path")
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`VACUUM INTO '%s'`, destPath))
	if err != nil {
		return fmt.Errorf("backup to %s: %w", destPath, err)
	}
	return nil
}

// scanner is the common interface for sql.Row and sql.Rows.
// sql.Row と sql.Rows の共通インタフェース。
type scanner interface {
	Scan(dest ...any) error
}

// scanRows iterates sql.Rows and converts each row using the given scan function.
// sql.Rows をイテレートし、各行を scan 関数で変換する。
func scanRows[T any](rows *sql.Rows, scan func(sc scanner) (*T, error)) ([]*T, error) {
	defer rows.Close()
	var items []*T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// now returns the current time. Replaceable in tests.
// 現在時刻を返す。テスト時に差し替え可能。
var now = time.Now

// formatTime delegates to sqlutil.FormatTime.
// sqlutil.FormatTime に委譲する。
var formatTime = sqlutil.FormatTime

// scanHelper accumulates errors from row-to-struct conversion helpers.
// After calling scanID / parseTime / unmarshalStringMap, check sh.err once.
// Scan 後の行→構造体変換でエラーを蓄積するヘルパー。
// scanID / parseTime / unmarshalStringMap を呼んだ後に sh.err を1回チェックする。
type scanHelper struct {
	err error
}

// scanID converts a BLOB column to id.ID, accumulating errors.
// BLOB カラムを id.ID に変換し、エラーを蓄積する。
func (h *scanHelper) scanID(b []byte) id.ID {
	if h.err != nil {
		return id.ID{}
	}
	v, err := sqlutil.ScanID(b)
	if err != nil {
		h.err = err
	}
	return v
}

// parseTime converts an RFC 3339 string to time.Time, accumulating errors.
// RFC 3339 文字列を time.Time に変換し、エラーを蓄積する。
func (h *scanHelper) parseTime(s string) time.Time {
	if h.err != nil {
		return time.Time{}
	}
	t, err := sqlutil.ParseTime(s)
	if err != nil {
		h.err = err
	}
	return t
}

// unmarshalStringMap converts a JSON string to map[string]string.
// JSON 文字列を map[string]string に変換する。
func (h *scanHelper) unmarshalStringMap(s string) map[string]string {
	if h.err != nil {
		return nil
	}
	m := map[string]string{}
	if s == "" {
		return m
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		h.err = fmt.Errorf("store/sqlite: unmarshalStringMap: %w", err)
	}
	return m
}

// Helper: convert bool to int for SQLite storage.
// ヘルパー: bool を SQLite 保存用の int に変換。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// marshalJSON converts a value to a JSON string.
// 値を JSON 文字列に変換する。
func marshalJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("store/sqlite: marshalJSON: %w", err)
	}
	return string(data), nil
}
