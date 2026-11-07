# 对本地 PostgreSQL 跑 Go 测试：设置 CK_TEST_PG_DSN（与 CI 一致，CK_REQUIRE_PG=true：连不上就失败，而不是静默跳过）。
#
#   powershell -NoProfile -File scripts/pg-test.ps1                      # go test ./cmd/... ./internal/...
#   powershell -NoProfile -File scripts/pg-test.ps1 -Packages ./internal/order/...
#   powershell -NoProfile -File scripts/pg-test.ps1 -Run TestSeckill -Race
#   powershell -NoProfile -File scripts/pg-test.ps1 -PrintOnly            # 只打印/导出 DSN 的设置命令
#   powershell -NoProfile -File scripts/pg-test.ps1 -Dsn "postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable"
#
# DSN 取值优先级：-Dsn > 当前 shell 的 CK_TEST_PG_DSN > docker compose 的 postgres（127.0.0.1:5433，用户 couponkill，是超级用户）。
# 测试会在该服务器上各自创建并删除临时数据库，需要有 CREATEDB 权限；不要指向生产实例。
[CmdletBinding()]
param(
    [string]$Dsn = "",
    [string[]]$Packages = @("./cmd/...", "./internal/..."),
    [string]$Run = "",
    # 竞态检测需要 cgo（gcc）
    [switch]$Race,
    [switch]$PrintOnly
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location -LiteralPath $Root

$composeDsn = "postgres://couponkill:couponkill@127.0.0.1:5433/postgres?sslmode=disable"
if (-not $Dsn) { $Dsn = $env:CK_TEST_PG_DSN }
if (-not $Dsn) { $Dsn = $composeDsn }

# 从 DSN 里取 host:port，先做一次 TCP 探测，给出明确的错误提示
$uri = [Uri]($Dsn -replace "^postgres(ql)?://", "http://")
$pgHost = $uri.Host
$pgPort = if ($uri.Port -gt 0) { $uri.Port } else { 5432 }

if ($PrintOnly) {
    Write-Host "`$env:CK_TEST_PG_DSN = '$Dsn'"
    Write-Host "`$env:CK_REQUIRE_PG = 'true'"
    exit 0
}

$client = New-Object System.Net.Sockets.TcpClient
try {
    $reachable = $client.ConnectAsync($pgHost, $pgPort).Wait(1500) -and $client.Connected
}
catch { $reachable = $false }
finally { $client.Dispose() }
if (-not $reachable) {
    throw "连不上 PostgreSQL ${pgHost}:${pgPort}。先 `docker compose up -d postgres`，或用 -Dsn / CK_TEST_PG_DSN 指向可用的实例。"
}

$env:CK_TEST_PG_DSN = $Dsn
$env:CK_REQUIRE_PG = "true"
Write-Host "CK_TEST_PG_DSN -> ${pgHost}:${pgPort}（CK_REQUIRE_PG=true）" -ForegroundColor Cyan

$goArgs = @("test")
if ($Race) { $goArgs += "-race" }
$goArgs += "-count=1"
if ($Run) { $goArgs += @("-run", $Run) }
$goArgs += $Packages
Write-Host ">> go $($goArgs -join ' ')" -ForegroundColor DarkGray
& go @goArgs
exit $LASTEXITCODE
