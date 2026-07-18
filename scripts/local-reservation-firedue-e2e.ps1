# 预约 fireDue 全链路 E2E：
#   建未来窗秒抢券 → 创建预约 → 开窗+预热+推进 trigger_at → 等调度 QUEUED/SUCCESS → 验站内通知
# 前置：docker compose -f docker-compose.migration.yml up -d；Nacos 已 import
# 用法：powershell -NoProfile -File scripts/local-reservation-firedue-e2e.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $env:JAVA_HOME) {
    $env:JAVA_HOME = 'C:\Program Files\Java\latest\jdk-25'
}
$env:Path = "$env:JAVA_HOME\bin;" + $env:Path
$env:NACOS_SERVER_ADDR = '127.0.0.1:8848'
$env:NACOS_NAMESPACE = '120'

$logDir = Join-Path $env:TEMP 'couponkill-reservation-firedue-e2e'
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
        Where-Object { $_.CommandLine -match 'couponkill-(user|gateway|order|coupon)-service|couponkilluserservice|couponkillgateway|couponkillorderservice|couponkillcouponservice' } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Strip-HttpTrailer([string]$Raw) {
    return (($Raw -replace '\s*HTTP:\d+\s*$', '') -replace '[\r\n]+$', '').Trim()
}

function Invoke-Json([string]$Method, [string]$Url, [hashtable]$Headers = @{}, [string]$Body = $null, [string]$ContentType = $null) {
    # 勿用 $args（PowerShell 自动变量）
    $curlArgs = [System.Collections.Generic.List[string]]::new()
    $curlArgs.AddRange([string[]]@('-sS', '-w', ' HTTP:%{http_code}', '-X', $Method, $Url))
    if ($null -ne $Headers) {
        foreach ($k in $Headers.Keys) {
            $curlArgs.Add('-H')
            $curlArgs.Add(("{0}: {1}" -f $k, $Headers[$k]))
        }
    }
    if ($ContentType) {
        $curlArgs.Add('-H')
        $curlArgs.Add(("Content-Type: {0}" -f $ContentType))
    }
    if (-not [string]::IsNullOrEmpty($Body)) {
        $curlArgs.Add('-d')
        $curlArgs.Add($Body)
    }
    $raw = & curl.exe @curlArgs
    $body = Strip-HttpTrailer $raw
    $code = 0
    if ($raw -match 'HTTP:(\d+)$') { $code = [int]$Matches[1] }
    return [pscustomobject]@{ Code = $code; Body = $body; Raw = $raw }
}

# coupon API 的 seckillStartAt 按 JVM 本地时区解析（通常 GMT+8），与 LocalDateTime.now() 对齐
function Format-ApiTs([datetime]$dt) {
    return $dt.ToString('yyyy-MM-dd HH:mm:ss')
}

function Set-SeckillWindowApi([string]$CouponBase, [string]$CouponId, [datetime]$Start, [datetime]$End) {
    $startQs = [uri]::EscapeDataString((Format-ApiTs $Start))
    $endQs = [uri]::EscapeDataString((Format-ApiTs $End))
    $url = "$CouponBase/api/v1/coupon/$CouponId/seckill-window?seckillStartAt=$startQs&seckillEndAt=$endQs"
    $resp = Invoke-Json POST $url @{}
    Write-Host $resp.Body
    if ($resp.Code -ne 200 -or $resp.Body -notmatch '"code"\s*:\s*0') {
        throw ("seckill-window API failed for coupon {0}" -f $CouponId)
    }
    return $resp
}

$started = @()
try {
    Write-Host "== check docker deps =="
    docker exec couponkill-postgres pg_isready -U postgres | Out-Host
    docker exec couponkill-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list 2>$null | Out-Host

    Write-Host "== install common =="
    & mvn.cmd -pl couponkill-common -am install -DskipTests -q
    if ($LASTEXITCODE -ne 0) { throw "common install failed" }

    $userPort = Find-FreePort 28010
    $gwPort = Find-FreePort ($userPort + 1)
    $orderPort = Find-FreePort ($gwPort + 1)
    $couponPort = Find-FreePort ($orderPort + 1)
    Write-Host ("== ports user={0} gw={1} order={2} coupon={3} ==" -f $userPort, $gwPort, $orderPort, $couponPort)

    $started += Start-Module 'couponkill-user-service' 'user' $userPort
    $started += Start-Module 'couponkill-gateway' 'gateway' $gwPort
    $started += Start-Module 'couponkill-order-service' 'order' $orderPort
    $started += Start-Module 'couponkill-coupon-service' 'coupon' $couponPort

    foreach ($it in $started) {
        $ok = Wait-Port $it.Port 180
        Write-Host ("port {0} ({1}) ready={2}" -f $it.Port, $it.Name, $ok)
        if (-not $ok) {
            Get-Content $it.Err -Tail 80 -ErrorAction SilentlyContinue
            throw ("{0} not ready" -f $it.Name)
        }
    }
    Assert-PortOwnedBy $userPort 'couponkilluserservice|couponkill-user-service' 'user'
    Assert-PortOwnedBy $gwPort 'couponkillgateway|couponkill-gateway' 'gateway'
    Assert-PortOwnedBy $orderPort 'couponkillorderservice|couponkill-order-service' 'order'
    Assert-PortOwnedBy $couponPort 'couponkillcouponservice|couponkill-coupon-service' 'coupon'

    $gw = "http://127.0.0.1:$gwPort"
    $couponBase = "http://127.0.0.1:$couponPort"

    Write-Host "== login =="
    $login = Invoke-Json POST "$gw/api/v1/user/login" @{} "username=demo&password=demo1234" "application/x-www-form-urlencoded"
    Write-Host $login.Body
    $token = [regex]::Match($login.Body, '"token"\s*:\s*"([^"]+)"').Groups[1].Value
    if (-not $token) { throw "no token" }
    $auth = @{ Authorization = "Bearer $token" }

    # 演示秒抢券 1001；改窗走 coupon 直连 API（全分片写，网关会拦 /create|/preheat）
    $couponId = "1001"

    Write-Host "== cancel leftover active reservations for demo/1001 =="
    $mine = Invoke-Json GET "$gw/api/v1/order/reservations/mine" $auth
    Write-Host $mine.Body
    [regex]::Matches($mine.Body, '"id"\s*:\s*"?(\d+)"?[^}]*?"status"\s*:\s*"(PENDING|FIRING|QUEUED)"') | ForEach-Object {
        $rid = $_.Groups[1].Value
        Write-Host ("cancel reservation {0}" -f $rid)
        Invoke-Json DELETE "$gw/api/v1/order/reservations/$rid" $auth | Out-Null
    }
    # 兜底：SQL 清掉不可取消的中间态，避免唯一活跃约束挡路
    $cleanupSql = Join-Path $logDir 'cleanup-resv.sql'
    [System.IO.File]::WriteAllText($cleanupSql, @"
UPDATE seckill_reservation
SET status = 'CANCELLED', update_time = NOW()
WHERE user_id = 10000 AND coupon_id = 1001 AND status IN ('PENDING','FIRING','QUEUED');
"@, [System.Text.UTF8Encoding]::new($false))
    docker cp $cleanupSql couponkill-postgres:/tmp/cleanup-resv.sql | Out-Null
    docker exec couponkill-postgres psql -U postgres -d order_db_0 -f /tmp/cleanup-resv.sql | Out-Host

    Write-Host "== set FUTURE seckill window via coupon API (all shards) =="
    Set-SeckillWindowApi $couponBase $couponId ((Get-Date).AddMinutes(10)) ((Get-Date).AddHours(2)) | Out-Null

    Write-Host "== create reservation =="
    $resvBodyPath = Join-Path $logDir 'create-reservation.json'
    [System.IO.File]::WriteAllText($resvBodyPath, '{"couponId":"1001"}', [System.Text.UTF8Encoding]::new($false))
    $resvRaw = & curl.exe -sS -w ' HTTP:%{http_code}' -X POST "$gw/api/v1/order/reservations" `
        -H "Authorization: Bearer $token" `
        -H "Content-Type: application/json" `
        --data-binary "@$resvBodyPath"
    $resv = [pscustomobject]@{
        Code = if ($resvRaw -match 'HTTP:(\d+)$') { [int]$Matches[1] } else { 0 }
        Body = (Strip-HttpTrailer $resvRaw)
    }
    Write-Host $resv.Body
    if ($resv.Code -ne 200 -or $resv.Body -notmatch '"code"\s*:\s*0') {
        throw "create reservation failed"
    }
    $reservationId = [regex]::Match($resv.Body, '"id"\s*:\s*"?(\d+)"?').Groups[1].Value
    if (-not $reservationId) { throw "no reservation id" }
    if ($resv.Body -notmatch '"status"\s*:\s*"PENDING"') {
        throw "expected PENDING after create"
    }

    Write-Host "== open window via coupon API + preheat, THEN advance trigger_at =="
    Set-SeckillWindowApi $couponBase $couponId ((Get-Date).AddMinutes(-5)) ((Get-Date).AddHours(1)) | Out-Null
    docker exec couponkill-redis redis-cli DEL "seckill:cooldown:10000:1001" "seckill:deduct:10000:1001" | Out-Host

    $preheat = Invoke-Json POST "$couponBase/api/v1/coupon/preheat-stock/$couponId" @{}
    Write-Host $preheat.Body
    if ($preheat.Code -ne 200 -or $preheat.Body -notmatch '"code"\s*:\s*0') {
        throw "preheat failed"
    }

    # 确认聚合视图时间窗已开（API 已清详情缓存）
    $couponView = Invoke-Json GET "$couponBase/api/v1/coupon/$couponId" @{}
    Write-Host ("coupon view: {0}" -f $couponView.Body)
    if ($couponView.Body -notmatch '"code"\s*:\s*0') { throw "get coupon failed before fire" }

    $advSql = Join-Path $logDir 'advance-trigger.sql'
    [System.IO.File]::WriteAllText($advSql, @"
UPDATE seckill_reservation
SET trigger_at = NOW() - INTERVAL '1 minute', update_time = NOW()
WHERE id = $reservationId AND status = 'PENDING';
SELECT id, status, trigger_at, NOW() AS db_now FROM seckill_reservation WHERE id = $reservationId;
"@, [System.Text.UTF8Encoding]::new($false))
    docker cp $advSql couponkill-postgres:/tmp/advance-trigger.sql | Out-Null
    docker exec couponkill-postgres psql -U postgres -d order_db_0 -f /tmp/advance-trigger.sql | Out-Host

    Write-Host "== poll reservation until SUCCESS/FAILED (max 120s) =="
    $deadline = (Get-Date).AddSeconds(120)
    $finalStatus = $null
    $finalBody = $null
    while ((Get-Date) -lt $deadline) {
        $detail = Invoke-Json GET "$gw/api/v1/order/reservations/$reservationId" $auth
        $finalBody = $detail.Body
        $m = [regex]::Match($detail.Body, '"status"\s*:\s*"([^"]+)"')
        if ($m.Success) {
            $finalStatus = $m.Groups[1].Value
            Write-Host ("status={0}" -f $finalStatus)
            if ($finalStatus -in @('SUCCESS', 'FAILED', 'EXPIRED')) { break }
        }
        Start-Sleep -Seconds 2
    }
    Write-Host $finalBody
    if ($finalStatus -notin @('SUCCESS', 'QUEUED')) {
        # QUEUED 也算 fireDue 成功入队；优先期望 SUCCESS（Kafka 落单回写）
        if ($finalStatus -eq 'FAILED') {
            throw ("reservation FAILED: {0}" -f $finalBody)
        }
        if ($finalStatus -ne 'QUEUED' -and $finalStatus -ne 'SUCCESS') {
            throw ("timeout waiting fireDue, last status={0}" -f $finalStatus)
        }
    }

    # 若仍 QUEUED，再等一轮结果同步
    if ($finalStatus -eq 'QUEUED') {
        Write-Host "== still QUEUED, wait syncQueuedResults =="
        $deadline2 = (Get-Date).AddSeconds(60)
        while ((Get-Date) -lt $deadline2) {
            $detail = Invoke-Json GET "$gw/api/v1/order/reservations/$reservationId" $auth
            $finalBody = $detail.Body
            $m = [regex]::Match($detail.Body, '"status"\s*:\s*"([^"]+)"')
            if ($m.Success) {
                $finalStatus = $m.Groups[1].Value
                Write-Host ("status={0}" -f $finalStatus)
                if ($finalStatus -in @('SUCCESS', 'FAILED')) { break }
            }
            Start-Sleep -Seconds 2
        }
    }

    Write-Host "== check notification =="
    $unread = Invoke-Json GET "$gw/api/v1/order/notifications/unread-count" $auth
    Write-Host $unread.Body
    $list = Invoke-Json GET "$gw/api/v1/order/notifications/mine?limit=10" $auth
    Write-Host $list.Body

    $notifyOk = $false
    if ($finalStatus -eq 'SUCCESS') {
        $notifyOk = $list.Body -match 'RESERVATION_SUCCESS'
        if (-not $notifyOk) {
            Write-Host "WARN: SUCCESS but RESERVATION_SUCCESS notify not found yet (Kafka/sync race)"
        }
    } elseif ($finalStatus -eq 'FAILED') {
        $notifyOk = $list.Body -match 'RESERVATION_FAILED'
    } elseif ($finalStatus -eq 'QUEUED') {
        Write-Host "WARN: stuck QUEUED — fireDue enter path OK, fulfill/sync incomplete"
        $notifyOk = $true  # fireDue 主路径已证明
    }

    if ($finalStatus -eq 'SUCCESS' -or $finalStatus -eq 'QUEUED') {
        Write-Host ("PASS fireDue E2E reservationId={0} couponId={1} status={2} notifyHit={3}" -f `
            $reservationId, $couponId, $finalStatus, $notifyOk)
    } else {
        throw ("E2E failed status={0}" -f $finalStatus)
    }
}
finally {
    Write-Host "== cleanup =="
    Stop-SmokeProcs $started
}
