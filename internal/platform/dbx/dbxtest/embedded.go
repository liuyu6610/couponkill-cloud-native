package dbxtest

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

const (
	embeddedUser     = "postgres"
	embeddedPassword = "postgres"
	embeddedDatabase = "postgres"

	envBinaryRepo = "CK_TEST_PG_BINARY_REPO"
	envCacheDir   = "CK_TEST_PG_CACHE"

	startLockStaleAfter = 10 * time.Minute
	startLockWait       = 5 * time.Minute
)

// startEmbedded 启动一个内嵌 PostgreSQL 16 并返回其管理 DSN。
// 多个测试进程并发首次使用时，用缓存目录里的锁文件串行化“下载/解压/启动”，避免互相踩缓存。
func startEmbedded() (string, error) {
	cacheDir, err := cacheDir()
	if err != nil {
		return "", err
	}
	release, err := lockStart(cacheDir)
	if err != nil {
		return "", err
	}
	defer release()

	port, err := freePort()
	if err != nil {
		return "", err
	}
	runRoot := filepath.Join(cacheDir, "run")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return "", fmt.Errorf("创建内嵌 PostgreSQL 运行根目录: %w", err)
	}
	runtimeDir, err := os.MkdirTemp(runRoot, "pg-*")
	if err != nil {
		return "", fmt.Errorf("创建内嵌 PostgreSQL 运行目录: %w", err)
	}

	cfg := embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(uint32(port)).
		Username(embeddedUser).
		Password(embeddedPassword).
		Database(embeddedDatabase).
		RuntimePath(runtimeDir).
		CachePath(cacheDir).
		Encoding("UTF8").
		Locale("C").
		StartTimeout(90 * time.Second).
		Logger(io.Discard).
		StartParameters(map[string]string{
			"fsync":              "off",
			"synchronous_commit": "off",
			"full_page_writes":   "off",
			"max_connections":    "200",
		})
	if repo := os.Getenv(envBinaryRepo); repo != "" {
		cfg = cfg.BinaryRepositoryURL(repo)
	}

	if err := embeddedpostgres.NewDatabase(cfg).Start(); err != nil {
		_ = os.RemoveAll(runtimeDir)
		return "", fmt.Errorf("启动内嵌 PostgreSQL 16: %w", err)
	}
	if err := spawnReaper(runtimeDir); err != nil {
		return "", fmt.Errorf("启动内嵌 PostgreSQL 的清理守护进程: %w", err)
	}
	return fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable",
		embeddedUser, embeddedPassword, port, embeddedDatabase), nil
}

// cacheDir 返回内嵌 PostgreSQL 的二进制缓存与运行目录的根。优先级：
// 环境变量 CK_TEST_PG_CACHE；Windows 且存在 D 盘时用 D:\dev-data\embedded-pg（避免占用系统盘）；
// 否则用户缓存目录。
func cacheDir() (string, error) {
	dir := os.Getenv(envCacheDir)
	if dir == "" && runtime.GOOS == "windows" {
		if info, err := os.Stat(`D:\`); err == nil && info.IsDir() {
			dir = `D:\dev-data\embedded-pg`
		}
	}
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil || base == "" {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "couponkill", "embedded-postgres")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建缓存目录 %s: %w", dir, err)
	}
	return dir, nil
}

// freePort 让内核分配一个空闲端口。
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("选择空闲端口: %w", err)
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("无法解析监听地址")
	}
	return addr.Port, nil
}

// lockStart 通过原子创建锁文件在多个进程间互斥；陈旧（超过 10 分钟）的锁文件视为遗留并清除。
func lockStart(dir string) (release func(), err error) {
	path := filepath.Join(dir, "start.lock")
	deadline := time.Now().Add(startLockWait)
	for {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if openErr == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(openErr, os.ErrExist) {
			return nil, fmt.Errorf("创建启动锁 %s: %w", path, openErr)
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > startLockStaleAfter {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("等待启动锁 %s 超时", path)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// spawnReaper 启动一个独立的守护子进程：等本测试进程退出后停止内嵌 PostgreSQL 并删除运行目录。
// Go 测试没有“进程退出钩子”（无 TestMain 时更是如此），所以用 shell 子进程盯着父进程，
// 这样即便测试被超时杀掉也不会遗留 postgres 进程。子进程不继承任何标准流，不会让 go test 等它。
func spawnReaper(runtimeDir string) error {
	ctl := filepath.Join(runtimeDir, "bin", "pg_ctl")
	data := filepath.Join(runtimeDir, "data")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		const script = `try { Wait-Process -Id $env:CK_REAP_PID -ErrorAction SilentlyContinue } catch {}; ` +
			`& "$env:CK_REAP_CTL.exe" stop -D $env:CK_REAP_DATA -m immediate *> $null; ` +
			`for ($i = 0; $i -lt 20; $i++) { Remove-Item -Recurse -Force $env:CK_REAP_DIR -ErrorAction SilentlyContinue; ` +
			`if (-not (Test-Path $env:CK_REAP_DIR)) { break }; Start-Sleep -Milliseconds 500 }`
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		const script = `while kill -0 "$CK_REAP_PID" 2>/dev/null; do sleep 1; done; ` +
			`"$CK_REAP_CTL" stop -D "$CK_REAP_DATA" -m immediate >/dev/null 2>&1; rm -rf "$CK_REAP_DIR"`
		cmd = exec.Command("sh", "-c", script)
	}
	cmd.Env = append(os.Environ(),
		"CK_REAP_PID="+strconv.Itoa(os.Getpid()),
		"CK_REAP_CTL="+ctl,
		"CK_REAP_DATA="+data,
		"CK_REAP_DIR="+runtimeDir,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动守护进程: %w", err)
	}
	return cmd.Process.Release()
}
