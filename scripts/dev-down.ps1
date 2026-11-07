# 停止 scripts/dev-up.ps1 启动的 5 个服务进程（按 .dev/pids/*.pid），可选一并停掉基础设施。
#
#   powershell -NoProfile -File scripts/dev-down.ps1               # 只停服务
#   powershell -NoProfile -File scripts/dev-down.ps1 -Infra        # 同时 docker compose down
#   powershell -NoProfile -File scripts/dev-down.ps1 -Infra -Volumes   # 并删除数据卷（PG/Redis/Kafka 数据全部清空）
#
# Windows 没有 SIGTERM：服务进程是被直接结束的，不会走优雅停机；需要验证优雅停机请在前台 `go run ./cmd/<svc>` 后按 Ctrl+C。
[CmdletBinding()]
param(
    [switch]$Infra,
    [switch]$Volumes
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location -LiteralPath $Root

$PidDir = Join-Path $Root ".dev/pids"
$BinDir = Join-Path $Root ".dev/bin"
$stopped = 0

function Stop-Tree([int]$ProcessId) {
    # /T 连同子进程（cmd.exe 包着的服务二进制）一起结束
    & taskkill.exe /PID $ProcessId /T /F *> $null
}

function Test-DevProcess([int]$ProcessId) {
    # pid 可能已被别的进程复用：只处理命令行里带 .dev\bin 的 cmd.exe / 服务二进制
    $info = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $ProcessId" -ErrorAction SilentlyContinue
    if (-not $info) { return $false }
    $binWin = $BinDir.Replace("/", "\")
    return ($info.CommandLine -and $info.CommandLine.IndexOf($binWin, [StringComparison]::OrdinalIgnoreCase) -ge 0)
}

if (Test-Path -LiteralPath $PidDir) {
    foreach ($pidFile in Get-ChildItem -LiteralPath $PidDir -Filter "*.pid") {
        $name = [IO.Path]::GetFileNameWithoutExtension($pidFile.Name)
        $procId = 0
        if ([int]::TryParse((Get-Content -LiteralPath $pidFile.FullName -Raw).Trim(), [ref]$procId) -and (Test-DevProcess $procId)) {
            Write-Host "停止 $name (pid $procId)"
            Stop-Tree $procId
            $stopped++
        }
        Remove-Item -LiteralPath $pidFile.FullName -Force
    }
}

# pid 文件丢失时的兜底：按二进制路径找残留进程
$binWin = $BinDir.Replace("/", "\")
Get-CimInstance -ClassName Win32_Process -ErrorAction SilentlyContinue | Where-Object {
    $_.ExecutablePath -and $_.ExecutablePath.StartsWith($binWin, [StringComparison]::OrdinalIgnoreCase)
} | ForEach-Object {
    Write-Host "清理残留进程 $($_.Name) (pid $($_.ProcessId))" -ForegroundColor Yellow
    Stop-Tree $_.ProcessId
    $stopped++
}

Write-Host "已停止 $stopped 个服务进程"

if ($Infra) {
    $composeArgs = @("compose", "--profile", "app", "down")
    if ($Volumes) { $composeArgs += "-v" }
    Write-Host "docker $($composeArgs -join ' ')"
    & docker @composeArgs
    if ($LASTEXITCODE -ne 0) { throw "docker compose down 失败" }
}
