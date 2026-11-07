# 本地开发：起基础设施（docker compose：PostgreSQL / Redis / Kafka），编译并以后台进程运行 5 个 Go 服务。
#
#   powershell -NoProfile -File scripts/dev-up.ps1            # 全部
#   powershell -NoProfile -File scripts/dev-up.ps1 -SkipInfra # 基础设施已经在跑（或用自己的）
#   powershell -NoProfile -File scripts/dev-up.ps1 -NoBuild   # 复用 .dev/bin 里上次编译的二进制
#   powershell -NoProfile -File scripts/dev-down.ps1          # 停掉
#
# 配置：CK_* 环境变量。优先级：当前 shell 里已设置的 > 仓库根 .env（复制自 .env.example）> 服务内置开发默认值。
# 服务以 CK_SEED_DEMO=true 启动（演示用户 demo/demo1234、admin/admin1234 与演示券），除非 .env 另行指定。
# 日志与 pid 在 .dev/（已 gitignore）：.dev/logs/<svc>.log、.dev/logs/<svc>.err.log、.dev/pids/<svc>.pid。
[CmdletBinding()]
param(
    [switch]$SkipInfra,
    [switch]$NoBuild,
    # 每个服务等待 /readyz 变为 200 的最长秒数（首次启动要跑迁移与演示数据）
    [int]$TimeoutSec = 120,
    [string]$EnvFile = ".env"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location -LiteralPath $Root

# 启动顺序：下游先于网关；端口与设计文档 D3 一致
$Services = [ordered]@{ user = 8081; coupon = 8080; order = 8082; connector = 8085; gateway = 8088 }

$DevDir = Join-Path $Root ".dev"
$BinDir = Join-Path $DevDir "bin"
$LogDir = Join-Path $DevDir "logs"
$PidDir = Join-Path $DevDir "pids"
foreach ($d in @($BinDir, $LogDir, $PidDir)) { New-Item -ItemType Directory -Force -Path $d | Out-Null }

function Write-Step([string]$Message) { Write-Host "== $Message" -ForegroundColor Cyan }

function Test-TcpPort([string]$HostName, [int]$Port, [int]$TimeoutMs = 800) {
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $task = $client.ConnectAsync($HostName, $Port)
        return ($task.Wait($TimeoutMs) -and $client.Connected)
    }
    catch { return $false }
    finally { $client.Dispose() }
}

function Wait-Until([scriptblock]$Condition, [int]$TimeoutSeconds, [string]$What) {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 500
    }
    throw "等待超时（${TimeoutSeconds}s）：$What"
}

function Import-EnvFile([string]$Path) {
    # 把 .env 里的 CK_* 导出为当前进程环境变量；shell 里已经设置的变量优先，不覆盖
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $count = 0
    foreach ($line in Get-Content -LiteralPath $Path -Encoding UTF8) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith("#")) { continue }
        $eq = $trimmed.IndexOf("=")
        if ($eq -lt 1) { continue }
        $key = $trimmed.Substring(0, $eq).Trim()
        $value = $trimmed.Substring($eq + 1).Trim().Trim('"').Trim("'")
        if ($key -notlike "CK_*") { continue }
        if ($value -eq "") { continue }
        if (-not [Environment]::GetEnvironmentVariable($key, "Process")) {
            [Environment]::SetEnvironmentVariable($key, $value, "Process")
            $count++
        }
    }
    Write-Host "已从 $Path 载入 $count 个 CK_* 变量"
}

function Get-RunningPid([string]$Name) {
    $pidFile = Join-Path $PidDir "$Name.pid"
    if (-not (Test-Path -LiteralPath $pidFile)) { return $null }
    $procId = [int](Get-Content -LiteralPath $pidFile -Raw).Trim()
    if (Get-Process -Id $procId -ErrorAction SilentlyContinue) { return $procId }
    Remove-Item -LiteralPath $pidFile -Force
    return $null
}

# ---- 1. 配置 ----------------------------------------------------------------
Write-Step "载入配置"
Import-EnvFile (Join-Path $Root $EnvFile)
if (-not $env:CK_SEED_DEMO) { $env:CK_SEED_DEMO = "true" }
if (-not $env:CK_ENV) { $env:CK_ENV = "dev" }
if ($env:CK_ENV -eq "prod") { throw "dev-up 只用于本地开发，CK_ENV 不能是 prod" }

foreach ($svc in $Services.Keys) {
    $existing = Get-RunningPid $svc
    if ($existing) { throw "$svc 已在运行（pid $existing）。先执行 scripts/dev-down.ps1" }
}

# ---- 2. 基础设施 ---------------------------------------------------------------
if (-not $SkipInfra) {
    Write-Step "启动 PostgreSQL / Redis / Kafka（docker compose）"
    & docker compose up -d
    if ($LASTEXITCODE -ne 0) { throw "docker compose up 失败（Docker 是否在运行？）" }

    Write-Step "等待基础设施就绪"
    Wait-Until { Test-TcpPort "127.0.0.1" 5433 } 90 "PostgreSQL 127.0.0.1:5433"
    Wait-Until { Test-TcpPort "127.0.0.1" 6379 } 60 "Redis 127.0.0.1:6379"
    Wait-Until { Test-TcpPort "127.0.0.1" 9092 } 90 "Kafka 127.0.0.1:9092"
    # kafka-init 是一次性容器：建好两个主题后以 0 退出
    Wait-Until {
        $state = (& docker inspect -f "{{.State.Status}} {{.State.ExitCode}}" couponkill-kafka-init 2>$null)
        "$state".Trim() -eq "exited 0"
    } 120 "kafka-init 建主题（docker logs couponkill-kafka-init）"
    Write-Host "基础设施就绪"
}

# ---- 3. 编译 ---------------------------------------------------------------
if (-not $NoBuild) {
    Write-Step "编译 5 个服务 → .dev/bin"
    $env:CGO_ENABLED = "0"
    foreach ($svc in $Services.Keys) {
        & go build -trimpath -o (Join-Path $BinDir "$svc.exe") "./cmd/$svc"
        if ($LASTEXITCODE -ne 0) { throw "编译 ./cmd/$svc 失败" }
    }
}

# ---- 4. 启动 ---------------------------------------------------------------
# 经 cmd.exe 把 stdout/stderr 重定向到日志文件，而不是用 Start-Process 的 -RedirectStandard*：
# 后者走 CreateProcess 并继承父进程的句柄，脚本的输出管道（被 CI/其它进程捕获时）会被常驻的服务进程一直占着，
# 调用方永远等不到本脚本结束。不带重定向参数的 Start-Process 走 ShellExecute，不继承句柄。
# pid 文件里记录的是 cmd.exe 的 pid（它的退出码就是服务的退出码；dev-down 会连同子进程一起结束）。
Write-Step "启动服务"
$procs = [ordered]@{}
foreach ($svc in $Services.Keys) {
    $exe = Join-Path $BinDir "$svc.exe"
    if (-not (Test-Path -LiteralPath $exe)) { throw "找不到 $exe（去掉 -NoBuild 重新编译）" }
    $log = Join-Path $LogDir "$svc.log"
    $errLog = Join-Path $LogDir "$svc.err.log"
    $env:CK_HTTP_PORT = [string]$Services[$svc]
    $cmdLine = '"' + $exe + '" > "' + $log + '" 2> "' + $errLog + '"'
    $p = Start-Process -FilePath $env:ComSpec -ArgumentList @("/d", "/s", "/c", ('"' + $cmdLine + '"')) `
        -WorkingDirectory $Root -WindowStyle Hidden -PassThru
    Set-Content -LiteralPath (Join-Path $PidDir "$svc.pid") -Value $p.Id -Encoding ASCII
    $procs[$svc] = $p
    Write-Host ("  {0,-10} pid {1,-7} :{2}" -f $svc, $p.Id, $Services[$svc])
}
Remove-Item Env:\CK_HTTP_PORT -ErrorAction SilentlyContinue

# ---- 5. 健康等待 ---------------------------------------------------------------
Write-Step "等待各服务 /readyz"
$failed = @()
foreach ($svc in $Services.Keys) {
    $port = $Services[$svc]
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        if ($procs[$svc].HasExited) { break }
        try {
            $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$port/readyz" -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) { $ready = $true; break }
        }
        catch { Start-Sleep -Milliseconds 500 }
    }
    if ($ready) {
        Write-Host ("  {0,-10} ready  http://127.0.0.1:{1}" -f $svc, $port) -ForegroundColor Green
    }
    else {
        $failed += $svc
        $reason = if ($procs[$svc].HasExited) { "进程已退出（退出码 $($procs[$svc].ExitCode)）" } else { "${TimeoutSec}s 内 /readyz 未返回 200" }
        Write-Host ("  {0,-10} FAILED {1}" -f $svc, $reason) -ForegroundColor Red
        foreach ($f in @("$svc.err.log", "$svc.log")) {
            $path = Join-Path $LogDir $f
            if ((Test-Path -LiteralPath $path) -and (Get-Item -LiteralPath $path).Length -gt 0) {
                Write-Host "  --- $f（最后 15 行）" -ForegroundColor Yellow
                Get-Content -LiteralPath $path -Tail 15 | ForEach-Object { Write-Host "  $_" }
            }
        }
    }
}

if ($failed.Count -gt 0) {
    Write-Host "有服务未就绪：$($failed -join ', ')。已启动的进程仍在运行，查看 .dev/logs，或执行 scripts/dev-down.ps1 清理。" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "全部就绪。网关 http://127.0.0.1:8088   日志 .dev/logs/   停止 scripts/dev-down.ps1" -ForegroundColor Green
if ($env:CK_SEED_DEMO -eq "true") { Write-Host "演示账号：demo / demo1234（管理员 admin / admin1234）" }
