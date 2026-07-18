# 站内通知联调：起 user/gateway/order → 写一条预约通知 → 登录拉取 unread/list
# 用法：powershell -NoProfile -File scripts/local-notification-smoke.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $env:JAVA_HOME) {
    $env:JAVA_HOME = 'C:\Program Files\Java\latest\jdk-25'
}
$env:Path = "$env:JAVA_HOME\bin;" + $env:Path
$env:NACOS_SERVER_ADDR = '127.0.0.1:8848'
$env:NACOS_NAMESPACE = '120'

$logDir = Join-Path $env:TEMP 'couponkill-notification-smoke'
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

function Get-ListenCommandLine([int]$Port) {
    $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -eq $conn) { return $null }
    $proc = Get-CimInstance Win32_Process -Filter "ProcessId=$($conn.OwningProcess)" -ErrorAction SilentlyContinue
    return $proc.CommandLine
}

function Assert-PortOwnedBy([int]$Port, [string]$Pattern, [string]$Label) {
    $cmd = Get-ListenCommandLine $Port
    if ([string]::IsNullOrWhiteSpace($cmd)) {
        throw ("{0}: port {1} has no listener" -f $Label, $Port)
    }
    if ($cmd -notmatch $Pattern) {
        $snip = $cmd.Substring(0, [Math]::Min(180, $cmd.Length))
        throw ("{0}: port {1} owned by unexpected process: {2}" -f $Label, $Port, $snip)
    }
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
    return [pscustomobject]@{ Proc = $proc; Name = $Name; Port = $Port; Out = $out; Err = $err }
}

function Stop-SmokeProcs($items) {
    foreach ($it in $items) {
        if ($null -eq $it) { continue }
        if ($it.Proc -and -not $it.Proc.HasExited) {
            Stop-Process -Id $it.Proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
    Get-CimInstance Win32_Process -Filter "Name='java.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match 'couponkill-(user|gateway|order)-service|couponkilluserservice|couponkillgateway|couponkillorderservice' } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Strip-HttpTrailer([string]$Raw) {
    return (($Raw -replace '\s*HTTP:\d+\s*$', '') -replace '[\r\n]+$', '').Trim()
}

$started = @()
try {
    Write-Host "== seed notification for demo user 10000 =="
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $sqlPath = Join-Path $logDir 'seed-notification.sql'
    $sql = @"
INSERT INTO user_notification (user_id, type, title, content, ref_id, read_flag, create_time)
VALUES (10000, 'RESERVATION_SUCCESS', E'\u9884\u7ea6\u5e2e\u62a2\u6210\u529f', E'\u5238 1001 \u8054\u8c03\u94c3\u94db\u5192\u70df', 1, FALSE, NOW());
"@
    [System.IO.File]::WriteAllText($sqlPath, $sql, [System.Text.UTF8Encoding]::new($false))
    docker cp $sqlPath couponkill-postgres:/tmp/seed-notification.sql | Out-Null
    docker exec couponkill-postgres psql -U postgres -d order_db_0 -f /tmp/seed-notification.sql | Out-Host
    $ErrorActionPreference = $prev

    Write-Host "== install common =="
    & mvn.cmd -pl couponkill-common -am install -DskipTests -q
    if ($LASTEXITCODE -ne 0) { throw "common install failed" }

    $userPort = Find-FreePort 27910
    $gwPort = Find-FreePort ($userPort + 1)
    $orderPort = Find-FreePort ($gwPort + 1)
    Write-Host ("== ports user={0} gateway={1} order={2} ==" -f $userPort, $gwPort, $orderPort)

    $started += Start-Module 'couponkill-user-service' 'user' $userPort
    $started += Start-Module 'couponkill-gateway' 'gateway' $gwPort
    $started += Start-Module 'couponkill-order-service' 'order' $orderPort

    foreach ($it in $started) {
        $ok = Wait-Port $it.Port 150
        Write-Host ("port {0} ({1}) ready={2}" -f $it.Port, $it.Name, $ok)
        if (-not $ok) {
            Get-Content $it.Err -Tail 80 -ErrorAction SilentlyContinue
            throw ("{0} not ready" -f $it.Name)
        }
    }
    Assert-PortOwnedBy $userPort 'couponkilluserservice|couponkill-user-service' 'user'
    Assert-PortOwnedBy $gwPort 'couponkillgateway|couponkill-gateway' 'gateway'
    Assert-PortOwnedBy $orderPort 'couponkillorderservice|couponkill-order-service' 'order'

    $base = "http://127.0.0.1:$gwPort"
    Write-Host "== login =="
    $loginRaw = & curl.exe -sS -w " HTTP:%{http_code}" -X POST "$base/api/v1/user/login" `
        -H "Content-Type: application/x-www-form-urlencoded" `
        -d "username=demo&password=demo1234"
    $loginBody = Strip-HttpTrailer $loginRaw
    Write-Host $loginBody
    $token = [regex]::Match($loginBody, '"token"\s*:\s*"([^"]+)"').Groups[1].Value
    if (-not $token) { throw "no token" }

    Write-Host "== unread-count =="
    $uRaw = & curl.exe -sS -w " HTTP:%{http_code}" `
        "$base/api/v1/order/notifications/unread-count" `
        -H "Authorization: Bearer $token"
    $uBody = Strip-HttpTrailer $uRaw
    Write-Host $uBody
    $u = $uBody | ConvertFrom-Json
    if ($u.code -ne 0 -and $u.code -ne 200) { throw "unread-count failed" }
    if ([int]$u.data.count -lt 1) { throw "expected unread >= 1" }

    Write-Host "== list mine =="
    $lRaw = & curl.exe -sS -w " HTTP:%{http_code}" `
        "$base/api/v1/order/notifications/mine?limit=5" `
        -H "Authorization: Bearer $token"
    $lBody = Strip-HttpTrailer $lRaw
    Write-Host $lBody
    $l = $lBody | ConvertFrom-Json
    if ($l.code -ne 0 -and $l.code -ne 200) { throw "list mine failed" }
    $items = @($l.data)
    if ($items.Count -lt 1) { throw "expected >=1 notification" }
    $hit = $items | Where-Object { $_.type -eq 'RESERVATION_SUCCESS' } | Select-Object -First 1
    if (-not $hit) { throw "seed RESERVATION_SUCCESS notification not found" }

    Write-Host ("PASS notifications unread={0} items={1} type={2}" -f $u.data.count, $items.Count, $hit.type)
}
finally {
    Write-Host "== cleanup =="
    Stop-SmokeProcs $started
}
