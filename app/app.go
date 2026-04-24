// Package app assembles dependencies and provides the application entry point.
// 依存を組み立て、アプリケーションのエントリポイントを提供するパッケージ。
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/handler"
	"github.com/murlog-org/murlog/i18n"
	"github.com/murlog-org/murlog/media"
	mediafs "github.com/murlog-org/murlog/media/fs"
	medias3 "github.com/murlog-org/murlog/media/s3"
	"github.com/murlog-org/murlog/queue"
	"github.com/murlog-org/murlog/queue/sqlqueue"
	"github.com/murlog-org/murlog/store"
	"github.com/murlog-org/murlog/worker"
)

// App holds the application dependencies and provides the HTTP handler.
// アプリケーションの依存を保持し、HTTP ハンドラを提供する。
type App struct {
	cfg      *config.Config
	store    store.Store
	queue    queue.Queue
	worker   *worker.Worker
	media   media.Store
	handler *handler.Handler
}

// New creates a new App from the given config.
// 設定から新しい App を生成する。
func New(cfg *config.Config) (*App, error) {
	// Load i18n locales from WebDir/locales/.
	// WebDir/locales/ から i18n ロケールを読み込む。
	if err := i18n.LoadDir(filepath.Join(cfg.WebDir, "locales")); err != nil {
		return nil, err
	}

	// Resolve DataDir to absolute path so SQLite never depends on CWD.
	// DataDir を絶対パスに変換し、CWD に依存しないようにする。
	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	cfg.DataDir = dataDir

	// Ensure directories exist for data and media.
	// データとメディアのディレクトリが存在することを保証する。
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.MediaPath, 0755); err != nil {
		return nil, fmt.Errorf("create media dir: %w", err)
	}

	s, err := store.Open(cfg.DBDriver, cfg.MainDBPath())
	if err != nil {
		return nil, err
	}

	// Run migrations on every startup.
	// 起動ごとにマイグレーションを実行 (冪等、差分のみ適用)。
	if err := s.Migrate(context.Background()); err != nil {
		s.Close()
		return nil, err
	}

	// Select media store backend based on storage_type.
	// storage_type に応じてメディアストアを選択。
	var m media.Store
	switch cfg.StorageType {
	case "s3":
		m = medias3.New(medias3.Options{
			Bucket:    cfg.S3Bucket,
			Region:    cfg.S3Region,
			Endpoint:  cfg.S3Endpoint,
			AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey,
			PublicURL: cfg.S3PublicURL,
		})
	default:
		m = mediafs.New(cfg.MediaPath, "/media")
	}
	q := sqlqueue.New(s.DB())
	w := worker.New(q, s, m, cfg.WorkerMinConcurrency, cfg.WorkerMaxConcurrency, cfg.WorkerJobRetentionDays)

	// Ensure worker_secret exists in DB for worker-tick auth.
	// worker-tick 認証用の worker_secret を DB に確保。
	ensureWorkerSecret(s)

	h := handler.New(cfg, s, q, w, m)

	return &App{
		cfg:     cfg,
		store:   s,
		queue:   q,
		worker:  w,
		media:   m,
		handler: h,
	}, nil
}

// Handler returns the root HTTP handler.
// ルート HTTP ハンドラを返す。
func (a *App) Handler() http.Handler {
	return a.handler
}

// Store returns the store for use by worker/queue.
// worker/queue 用に Store を返す。
func (a *App) Store() store.Store {
	return a.store
}

// Queue returns the job queue for use by worker.
// worker 用にジョブキューを返す。
func (a *App) Queue() queue.Queue {
	return a.queue
}

// Worker returns the worker for use by serve/CGI.
// serve/CGI 用にワーカーを返す。
func (a *App) Worker() *worker.Worker {
	return a.worker
}

// GetSetting reads a setting from the DB. Used by CLI subcommands.
// DB から設定を読み取る。CLI サブコマンドで使用。
func (a *App) GetSetting(key string) (string, error) {
	return a.store.GetSetting(context.Background(), key)
}

// Close releases all resources.
// 全リソースを解放する。
func (a *App) Close() error {
	return a.store.Close()
}

// ensureWorkerSecret generates and stores a worker secret if not present.
// worker secret が未設定なら生成して保存する。
func ensureWorkerSecret(s store.Store) {
	ctx := context.Background()
	existing, err := s.GetSetting(ctx, handler.SettingWorkerSecret)
	if err != nil {
		log.Printf("ensureWorkerSecret: get setting: %v", err)
	}
	if existing != "" {
		return
	}
	b := make([]byte, 32)
	rand.Read(b)
	if err := s.SetSetting(ctx, handler.SettingWorkerSecret, hex.EncodeToString(b)); err != nil {
		log.Printf("ensureWorkerSecret: set setting: %v", err)
	}
}
