# 起 user/gateway/order + Vite，写入端口文件后退出（不杀进程，供浏览器验铃铛）
# 清理：powershell -NoProfile -File scripts/local-bell-ui-down.ps1
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $env:JAVA_HOME) {
    $env:JAVA_HOME = 'C:\Program Files\Java\latest\jdk-25'
}
$env:Path = "$env:JAVA_HOME\bin;" + $env:Path
$env:NACOS_SERVER_ADDR = '127.0.0.1:8848'
$env:NACOS_NAMESPACE = '120'

$stateFile = Join-Path $env:TEMP 'couponkill-bell-ui-state.json'
$logDir = Join-Path $env:TEMP 'couponkill-bell-ui'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

function Find-FreePort([int]$Start) {
    for ($p = $Start; $p -lt ($Start + 200); $p++) {
        $c = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($null -eq $c) { return $p }
    }
    throw "no free port near $Start"
}

function Wait-Port([int]$Port, [int]$Seconds) {
    for ($i = 0; $i -lt $Seconds; $i++) {
        try {
            $c = New-Object System.Net.Sockets.TcpClient
            $iar = $c.BeginConnect('127.0.0.1', $Port, $null, $null)
            if ($iar.AsyncWaitHandle.WaitOne(500) -and $c.Connected) {
                $c.EndConnect($iar)
                $c.Close()
                return $true
            }
            $c.Close()
        } catch { }
        Start-Sleep -Seconds 2
    }
    return $false
}

function Start-Module([string]$ModuleDir, [string]$Name, [int]$Port) {
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $env:SERVER_PORT = "$Port"
    $proc = Start-Process -FilePath 'mvn.cmd' `
        -ArgumentList @('-f', (Join-Path $ModuleDir 'pom.xml'), 'spring-boot:run', '-DskipTests', '-q') `
        -WorkingDirectory $Root `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err `
        -PassThru `
        -WindowStyle Hidden
    return [pscustomobject]@{ ProcId = $proc.Id; Name = $Name; Port = $Port }
}

Write-Host "== seed notification =="
# 用 PG unicode 转义 + docker cp，避免 Windows 管道编码把中文写成 ???
$sqlPath = Join-Path $logDir 'seed-notification.sql'
$sql = @"
INSERT INTO user_notification (user_id, type, title, content, ref_id, read_flag, create_time)
VALUES (10000, 'RESERVATION_SUCCESS', E'\u9884\u7ea6\u5e2e\u62a2\u6210\u529f', E'\u5238 1001 \u94c3\u94db UI \u8054\u8c03', 1, FALSE, NOW());
"@
[System.IO.File]::WriteAllText($sqlPath, $sql, [System.Text.UTF8Encoding]::new($false))
docker cp $sqlPath couponkill-postgres:/tmp/seed-notification.sql | Out-Null
docker exec couponkill-postgres psql -U postgres -d order_db_0 -f /tmp/seed-notification.sql | Out-Host

Write-Host "== install common =="
& mvn.cmd -pl couponkill-common -am install -DskipTests -q
if ($LASTEXITCODE -ne 0) { throw "common install failed" }

$userPort = Find-FreePort 27940
$gwPort = Find-FreePort ($userPort + 1)
$orderPort = Find-FreePort ($gwPort + 1)
$vitePort = Find-FreePort 5173
Write-Host ("== ports user={0} gw={1} order={2} vite={3} ==" -f $userPort, $gwPort, $orderPort, $vitePort)

$procs = @()
$procs += Start-Module 'couponkill-user-service' 'user' $userPort
$procs += Start-Module 'couponkill-gateway' 'gateway' $gwPort
$procs += Start-Module 'couponkill-order-service' 'order' $orderPort

foreach ($it in $procs) {
    $ok = Wait-Port $it.Port 150
    Write-Host ("port {0} ({1}) ready={2}" -f $it.Port, $it.Name, $ok)
    if (-not $ok) { throw ("{0} not ready" -f $it.Name) }
}

$feDir = Join-Path $Root 'frontend\couponkill-frontend'
$viteOut = Join-Path $logDir 'vite.out.log'
$viteErr = Join-Path $logDir 'vite.err.log'
$viteEnv = @{
    VITE_PROXY_TARGET = "http://127.0.0.1:$gwPort"
}
# Start-Process 不直接传自定义 env 给子进程时用 cmd 包装
$viteCmd = "set VITE_PROXY_TARGET=http://127.0.0.1:$gwPort&& npx --yes vite --host 127.0.0.1 --port $vitePort --strictPort"
$viteProc = Start-Process -FilePath 'cmd.exe' `
    -ArgumentList @('/c', $viteCmd) `
    -WorkingDirectory $feDir `
    -RedirectStandardOutput $viteOut `
    -RedirectStandardError $viteErr `
    -PassThru `
    -WindowStyle Hidden

if (-not (Wait-Port $vitePort 90)) {
    Get-Content $viteErr -Tail 40 -ErrorAction SilentlyContinue
    throw "vite not ready"
}

$state = [ordered]@{
    userPort  = $userPort
    gwPort    = $gwPort
    orderPort = $orderPort
    vitePort  = $vitePort
    viteUrl   = "http://127.0.0.1:$vitePort"
    procIds   = @($procs.ProcId + $viteProc.Id)
    logDir    = $logDir
}
$state | ConvertTo-Json | Set-Content -Path $stateFile -Encoding UTF8
Write-Host ("READY vite={0} gw={1} state={2}" -f $state.viteUrl, $gwPort, $stateFile)
