package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"syscall"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/app"
	"github.com/murlog-org/murlog/config"
	"github.com/murlog-org/murlog/worker"

	// Store drivers — build tags control which are included.
	// ストアドライバ — ビルドタグで含まれるものを制御。
	_ "github.com/murlog-org/murlog/store/sqlite"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	} else if os.Getenv("GATEWAY_INTERFACE") != "" {
		// Auto-detect CGI environment. / CGI 環境を自動検出。
		cmd = "cgi"
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("murlog " + murlog.VersionString())
		return
	case "cgi":
		runCGI()
	case "serve":
		runServe()
	case "worker":
		runWorker()
	case "reset":
		runReset()
	case "update":
		runUpdate()
	case "vacuum":
		runVacuum()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func loadConfig() *config.Config {
	// Resolve config path relative to the binary/script location.
	// バイナリ/スクリプトの場所を基準に設定ファイルを解決する。
	dir := scriptDir()
	cfgPath := filepath.Join(dir, "murlog.ini")
	// Set working directory for relative paths in config.
	// config 内の相対パス解決のため、カレントディレクトリを設定。
	os.Chdir(dir)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}

// scriptDir returns the directory of the running binary.
// CGI uses SCRIPT_FILENAME, otherwise os.Executable().
// 実行中バイナリのディレクトリを返す。CGI では SCRIPT_FILENAME を使用。
func scriptDir() string {
	// CGI: Apache sets SCRIPT_FILENAME to the absolute path of the script.
	// CGI: Apache が SCRIPT_FILENAME にスクリプトの絶対パスを設定する。
	if sf := os.Getenv("SCRIPT_FILENAME"); sf != "" {
		return filepath.Dir(sf)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	dir, _ := os.Getwd()
	return dir
}

func loadApp() *app.App {
	cfg := loadConfig()
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	return a
}

func runCGI() {
	a := loadApp()
	defer a.Close()
	if err := cgi.Serve(a.Handler()); err != nil {
		log.Fatalf("cgi: %v", err)
	}

	// Spawn a worker process if there are pending jobs.
	// pending ジョブがあればワーカープロセスを spawn。
	if a.Queue().HasPending(context.Background()) {
		spawnWorker()
	}
}

func runServe() {
	cfg := loadConfig()
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	defer a.Close()

	// Start worker goroutine for background job processing.
	// バックグラウンドジョブ処理用のワーカーゴルーチンを起動。
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go a.Worker().Run(ctx)

	srv := &http.Server{Addr: cfg.Listen, Handler: a.Handler()}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	log.Printf("murlog %s listening on %s", murlog.VersionString(), cfg.Listen)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

// workerMaxLifetime is the hard limit for --once mode to avoid triggering
// shared hosting process monitors. I/O-wait-heavy delivery jobs rarely hit this.
// --once モードのハードリミット。レンサバのプロセス監視回避用。
// I/O wait 主体の配送ジョブでは通常到達しない。
const workerMaxLifetime = 5 * time.Minute

func runWorker() {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	once := fs.Bool("once", false, "process pending jobs and exit (for cron)")
	limit := fs.Int("limit", 20, "max jobs per batch in --once mode")
	timeout := fs.Int("timeout", 30, "timeout in seconds per batch")
	fs.Parse(os.Args[2:])

	cfg := loadConfig()
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	defer a.Close()

	w := a.Worker()

	if *once {
		// Acquire exclusive lock to prevent concurrent worker processes.
		// 排他ロックで同時実行を防止。
		unlock, ok := tryWorkerLock(cfg.DataDir)
		if !ok {
			log.Println("worker: another worker is running, exiting")
			return
		}
		defer unlock()

		// Process jobs with hard timeout — context cancel ensures process exits
		// even if a job hangs, releasing flock automatically.
		// ハードタイムアウト付きでジョブを処理 — ジョブがハングしても context cancel で
		// プロセスが確実に終了し、flock が自動解放される。
		ctx, cancel := context.WithTimeout(context.Background(), workerMaxLifetime)
		defer cancel()

		total := 0
		for ctx.Err() == nil {
			n := w.RunBatch(ctx, *limit, time.Duration(*timeout)*time.Second)
			total += n
			if n == 0 || !a.Queue().HasPending(ctx) {
				break
			}
		}
		log.Printf("worker: processed %d jobs total", total)
		return
	}

	// Default: run as a long-lived daemon.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	w.Run(ctx)
}

// tryWorkerLock acquires an exclusive flock on a lock file.
// Returns an unlock function and true if acquired, or false if another worker holds the lock.
// ロックファイルに排他 flock を取得する。
// 取得できたら unlock 関数と true、他の worker がロック中なら false を返す。
func tryWorkerLock(dataDir string) (unlock func(), ok bool) {
	lockPath := filepath.Join(dataDir, "worker.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Printf("worker: open lock file: %v", err)
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false
	}
	called := false
	return func() {
		if called {
			return
		}
		called = true
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, true
}


// kickWorkerTick sends a fire-and-forget HTTP request to the worker-tick endpoint.
// worker-tick エンドポイントに fire-and-forget リクエストを送る。
// spawnWorker starts a detached worker process to handle pending jobs.
// Does not wait for it to finish — the child process runs independently.
// 切り離されたワーカープロセスを起動して pending ジョブを処理する。
// 完了を待たない — 子プロセスは独立して実行される。
func spawnWorker() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	// In CGI wrapper mode, SCRIPT_FILENAME points to the shell script (murlog.cgi).
	// The Go binary is murlog.bin in the same directory.
	// CGI ラッパーモードでは SCRIPT_FILENAME はシェルスクリプトを指す。
	// Go バイナリは同ディレクトリの murlog.bin。
	if sf := os.Getenv("SCRIPT_FILENAME"); sf != "" {
		dir := filepath.Dir(sf)
		bin := filepath.Join(dir, "murlog.bin")
		if _, err := os.Stat(bin); err == nil {
			exe = bin
		} else {
			exe = sf
		}
	}
	cmd := exec.Command(exe, "worker", "--once")
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		log.Printf("spawnWorker: %v", err)
	}
}

// runReset generates a reset file with a random token and prints the reset URL.
// リセットファイルにランダムトークンを書き込み、リセット URL を表示する。
func runReset() {
	cfg := loadConfig()

	// Need domain from DB to show the reset URL.
	// リセット URL 表示のため DB からドメインを取得。
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	domain, err := a.GetSetting("domain")
	if err != nil || domain == "" {
		a.Close()
		log.Fatal("domain not configured. Run initial setup first.")
	}
	protocol, _ := a.GetSetting("protocol")
	a.Close()
	if protocol != "http" {
		protocol = "https"
	}

	// Generate random token. / ランダムトークンを生成。
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatalf("generate token: %v", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Write reset file in data dir. / データディレクトリにリセットファイルを作成。
	resetPath := filepath.Join(cfg.DataDir, "murlog.reset")
	if err := os.WriteFile(resetPath, []byte(token), 0o600); err != nil {
		log.Fatalf("write reset file: %v", err)
	}

	fmt.Printf("Reset file created: %s\n", resetPath)
	fmt.Printf("Reset URL: %s://%s/admin/reset?token=%s\n", protocol, domain, token)
	fmt.Println("\nOpen the URL in your browser to set a new password.")
	fmt.Println("The reset file will be automatically deleted after use.")
}

func runUpdate() {
	// TODO
	fmt.Println("update: not implemented")
}

// runVacuum cleans up old completed jobs and runs VACUUM to shrink the DB file.
// 古い完了ジョブを削除し、VACUUM で DB ファイルを縮小する。
func runVacuum() {
	fs := flag.NewFlagSet("vacuum", flag.ExitOnError)
	retention := fs.Int("retention", 0, "days to keep completed jobs (0 = use config/default)")
	fs.Parse(os.Args[2:])

	a := loadApp()
	defer a.Close()

	days := *retention
	if days <= 0 {
		cfg := loadConfig()
		days = cfg.WorkerJobRetentionDays
	}
	if days <= 0 {
		days = worker.DefaultJobRetentionDays
	}

	ctx := context.Background()
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	n, err := a.Queue().Cleanup(ctx, cutoff)
	if err != nil {
		log.Fatalf("cleanup: %v", err)
	}
	log.Printf("vacuum: deleted %d completed jobs older than %d days", n, days)

	if _, err := a.Store().DB().ExecContext(ctx, "VACUUM"); err != nil {
		log.Fatalf("vacuum: %v", err)
	}
	log.Println("vacuum: VACUUM completed")
}
