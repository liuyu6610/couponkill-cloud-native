# 本机一键端到端：不需要 Docker 和 Kafka/Redis 服务器。
#   - PostgreSQL：用一台真实的 PostgreSQL 16（环境变量 CK_TEST_PG_DSN，URL 形式，需要能建库的账号），
#     脚本在里面建一个临时库，结束时删除
#   - Redis / Kafka：cmd/devinfra 的内存替身（miniredis + kfake）
#   - 五个服务：go build 到临时目录后以后台进程启动，端口都用 18xxx 避开本机已有进程
#   - 然后运行 test/e2e，最后无论成败都清理进程与临时库
#
# 用法：
#   $env:CK_TEST_PG_DSN = 'postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable'
#   scripts\e2e-local.ps1                       # 全部场景
#   scripts\e2e-local.ps1 -TestPattern Seckill  # 只跑名字匹配的场景
#   scripts\e2e-local.ps1 -BootOnly             # 只检查五个服务能启动并就绪
# 需要 psql 在 PATH 里，或用 -Psql 指定完整路径。
param([switch]$BootOnly, [string]$TestPattern = '', [string]$Psql = '')
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$work = Join-Path ([IO.Path]::GetTempPath()) ('couponkill-e2e-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
$bin = Join-Path $work 'bin'
New-Item -ItemType Directory -Force -Path $bin | Out-Null

$dsn = $env:CK_TEST_PG_DSN
if (-not $dsn) { $dsn = 'postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable' }
if ($dsn -notmatch '^postgres(?:ql)?://(?<user>[^:@/]+)(?::(?<pw>[^@]*))?@(?<host>[^:/]+)(?::(?<port>\d+))?/') {
    throw "CK_TEST_PG_DSN 必须是 postgres://user[:password]@host[:port]/db 形式: $dsn"
}
$pgHost = $Matches.host; $pgPort = if ($Matches.port) { $Matches.port } else { '5432' }; $pgUser = $Matches.user
$pgPw = $Matches.pw
if (-not $Psql) { $cmd = Get-Command psql -ErrorAction SilentlyContinue; if ($cmd) { $Psql = $cmd.Source } }
if (-not $Psql -or -not (Test-Path $Psql)) { throw '找不到 psql：把它放进 PATH，或用 -Psql 指定完整路径' }
if ($pgPw) { $env:PGPASSWORD = $pgPw }
$dbName = 'ck_e2e_' + [guid]::NewGuid().ToString('N').Substring(0, 8)
function Invoke-Pg($sql) { & $Psql -h $pgHost -p $pgPort -U $pgUser -d postgres -Atc $sql | Out-Null }

$P = @{ gw = 18088; user = 18081; coupon = 18080; order = 18082; connector = 18085; redis = 16379; kafka = 19092 }
$env:CK_ENV = 'dev'
$env:CK_LOG_LEVEL = 'info'
$env:CK_PG_DSN = "postgres://${pgUser}$(if ($pgPw) { ':' + $pgPw })@${pgHost}:${pgPort}/${dbName}?sslmode=disable"
$env:CK_REDIS_ADDR = "127.0.0.1:$($P.redis)"
$env:CK_KAFKA_BROKERS = "127.0.0.1:$($P.kafka)"
$env:CK_SEED_DEMO = 'true'
$env:CK_USER_URL = "http://127.0.0.1:$($P.user)"
$env:CK_COUPON_URL = "http://127.0.0.1:$($P.coupon)"
$env:CK_ORDER_URL = "http://127.0.0.1:$($P.order)"
$env:CK_CONNECTOR_URL = "http://127.0.0.1:$($P.connector)"
# 端到端从同一个 IP 并发打网关：放宽网关限流，否则测的是限流而不是业务
$env:CK_GATEWAY_IP_RPS = '5000'; $env:CK_GATEWAY_IP_BURST = '5000'
$env:CK_GATEWAY_USER_RPS = '500'; $env:CK_GATEWAY_USER_BURST = '500'
$env:CK_GATEWAY_SECKILL_RPS = '500'; $env:CK_GATEWAY_SECKILL_BURST = '500'
$env:CK_GATEWAY_UPSTREAM_TIMEOUT = '10s'; $env:CK_GATEWAY_SECKILL_TIMEOUT = '10s'
# 让 sweeper / 预约调度在测试时间尺度内可观察
$env:CK_ORDER_SWEEP_INTERVAL = '2s'; $env:CK_ORDER_SWEEP_STALE = '8s'
$env:CK_ORDER_RESERVATION_SCAN_INTERVAL = '500ms'
$env:CK_CONNECTOR_MOCK_STOCK = '500'

$procs = @()
function Start-Svc($name, $port, $extraArgs = @()) {
    if ($port) { $env:CK_HTTP_PORT = "$port" }
    $p = Start-Process -FilePath (Join-Path $bin "$name.exe") -ArgumentList $extraArgs -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $work "$name.out.log") -RedirectStandardError (Join-Path $work "$name.err.log")
    $script:procs += $p
}
function Wait-Http($url, $secs) {
    for ($i = 0; $i -lt ($secs * 4); $i++) {
        try { $r = Invoke-WebRequest -Uri $url -SkipHttpErrorCheck -TimeoutSec 2; if ($r.StatusCode -eq 200) { return $true } } catch { }
        Start-Sleep -Milliseconds 250
    }
    return $false
}

$exit = 1
Push-Location $repo
try {
    foreach ($s in 'gateway', 'user', 'coupon', 'order', 'connector', 'devinfra') {
        & go build -o (Join-Path $bin "$s.exe") "./cmd/$s"
        if ($LASTEXITCODE -ne 0) { throw "go build ./cmd/$s 失败" }
    }
    Invoke-Pg "CREATE DATABASE $dbName"

    Start-Svc 'devinfra' $null @('-redis', "127.0.0.1:$($P.redis)", '-kafka-port', "$($P.kafka)")
    Start-Sleep -Seconds 2
    # 迁移在 advisory lock 保护下，服务可以并行启动
    Start-Svc 'user' $P.user; Start-Svc 'coupon' $P.coupon; Start-Svc 'order' $P.order
    Start-Svc 'connector' $P.connector; Start-Svc 'gateway' $P.gw

    $allUp = $true
    foreach ($s in @(@('user', $P.user), @('coupon', $P.coupon), @('order', $P.order), @('connector', $P.connector), @('gateway', $P.gw))) {
        $ok = Wait-Http "http://127.0.0.1:$($s[1])/readyz" 60
        '{0} {1} /readyz' -f ($(if ($ok) { 'UP  ' } else { 'DOWN' })), $s[0]
        if (-not $ok) { $allUp = $false; Get-Content (Join-Path $work "$($s[0]).err.log") -Tail 15 -ErrorAction SilentlyContinue }
    }

    if ($allUp -and -not $BootOnly) {
        $env:CK_E2E_BASE_URL = "http://127.0.0.1:$($P.gw)"
        $env:CK_E2E_DIRECT_ORDER_URL = "http://127.0.0.1:$($P.order)"
        $env:CK_REQUIRE_E2E = 'true'
        $targs = @('test', './test/e2e/...', '-count=1', '-v', '-timeout', '300s')
        if ($TestPattern) { $targs += @('-run', $TestPattern) }
        & go @targs
        $exit = $LASTEXITCODE
    } elseif ($allUp) { $exit = 0 }
}
finally {
    Pop-Location
    foreach ($p in $procs) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } }
    Start-Sleep -Milliseconds 800
    try { Invoke-Pg "DROP DATABASE IF EXISTS $dbName WITH (FORCE)" } catch { Write-Warning "删除临时库 $dbName 失败: $_" }
    if ($exit -ne 0) { "失败：服务日志保留在 $work" } else { Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue }
}
exit $exit
