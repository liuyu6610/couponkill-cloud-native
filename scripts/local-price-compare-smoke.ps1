# 联调 price-compare：起 gateway+connector → 绑定/手工映射 → 匿名比价 → 清理
# 前置：docker compose -f docker-compose.migration.yml up -d；import-nacos-local.ps1
# 用法：powershell -NoProfile -File scripts/local-price-compare-smoke.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $env:JAVA_HOME) {
    $env:JAVA_HOME = 'C:\Program Files\Java\latest\jdk-25'
}
$env:Path = "$env:JAVA_HOME\bin;" + $env:Path
$env:NACOS_SERVER_ADDR = '127.0.0.1:8848'
$env:NACOS_NAMESPACE = '120'
$env:CONNECTOR_ADMIN_TOKEN = 'smoke-price-compare-token'

$logDir = Join-Path $env:TEMP 'couponkill-price-compare-smoke'
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
    # 用环境变量传端口/admin token，避免 mvn 把 --connector.* 当成自身 CLI
    $env:SERVER_PORT = "$Port"
    $args = @(
        '-f', (Join-Path $ModuleDir 'pom.xml'),
        'spring-boot:run', '-DskipTests', '-q'
    )
    $proc = Start-Process -FilePath 'mvn.cmd' `
        -ArgumentList $args `
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
        $p = $it.Proc
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
    }
    Get-CimInstance Win32_Process -Filter "Name='java.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match 'couponkill-(gateway|connector)-service|couponkillgateway|couponkillconnectorservice' } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Strip-HttpTrailer([string]$Raw) {
    return (($Raw -replace '\s*HTTP:\d+\s*$', '') -replace '[\r\n]+$', '').Trim()
}

$started = @()
try {
    Write-Host "== ensure coupon_price_map =="
    $ddl = @"
CREATE TABLE IF NOT EXISTS coupon_price_map (
    id               BIGSERIAL PRIMARY KEY,
    coupon_id        BIGINT       NOT NULL,
    platform         VARCHAR(32)  NOT NULL,
    external_sku_id  VARCHAR(128) NOT NULL,
    title            VARCHAR(256),
    manual_price     NUMERIC(18, 2),
    currency         VARCHAR(8)   NOT NULL DEFAULT 'CNY',
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    create_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    update_time      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_price_map_coupon_platform UNIQUE (coupon_id, platform)
);
"@
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $ddl | docker exec -i couponkill-postgres psql -U postgres -d connector_db | Out-Host
    $ErrorActionPreference = $prevEap

    Write-Host "== install common =="
    & mvn.cmd -pl couponkill-common -am install -DskipTests -q
    if ($LASTEXITCODE -ne 0) { throw "common install failed" }

    $gwPort = Find-FreePort 27880
    $connPort = Find-FreePort ($gwPort + 1)
    Write-Host ("== ports gateway={0} connector={1} ==" -f $gwPort, $connPort)

    Write-Host "== start gateway/connector =="
    $started += Start-Module 'couponkill-gateway' 'gateway' $gwPort
    $started += Start-Module 'couponkill-connector-service' 'connector' $connPort

    foreach ($it in $started) {
        $ok = Wait-Port $it.Port 150
        Write-Host ("port {0} ({1}) ready={2}" -f $it.Port, $it.Name, $ok)
        if (-not $ok) {
            Get-Content $it.Err -Tail 80 -ErrorAction SilentlyContinue
            Get-Content $it.Out -Tail 40 -ErrorAction SilentlyContinue
            throw ("{0} not ready" -f $it.Name)
        }
    }
    Assert-PortOwnedBy $gwPort 'couponkillgateway|couponkill-gateway' 'gateway'
    Assert-PortOwnedBy $connPort 'couponkillconnectorservice|couponkill-connector' 'connector'

    $base = "http://127.0.0.1:$gwPort"
    $adminToken = $env:CONNECTOR_ADMIN_TOKEN
    $couponId = 1001

    $bindFile = Join-Path $logDir 'bind.json'
    $mapFile = Join-Path $logDir 'map.json'
    $utf8 = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($bindFile, (@{
        platform = 'MOCK'
        externalSkuId = 'mock-sku:smoke'
        couponId = "$couponId"
        syncEnabled = $true
    } | ConvertTo-Json -Compress), $utf8)
    [System.IO.File]::WriteAllText($mapFile, (@{
        couponId = "$couponId"
        platform = 'TB'
        externalSkuId = 'tb-sku-smoke'
        title = 'TB manual same SKU'
        manualPrice = 79.9
        currency = 'CNY'
        enabled = $true
    } | ConvertTo-Json -Compress), $utf8)

    Write-Host "== upsert binding MOCK =="
    $bindRaw = & curl.exe -sS -w " HTTP:%{http_code}" -X POST `
        "$base/api/v1/connector/bindings" `
        -H "X-Admin-Token: $adminToken" `
        -H "Content-Type: application/json" `
        --data-binary "@$bindFile"
    Write-Host (Strip-HttpTrailer $bindRaw)

    Write-Host "== upsert TB manual map =="
    $mapRaw = & curl.exe -sS -w " HTTP:%{http_code}" -X POST `
        "$base/api/v1/connector/price-maps" `
        -H "X-Admin-Token: $adminToken" `
        -H "Content-Type: application/json" `
        --data-binary "@$mapFile"
    Write-Host (Strip-HttpTrailer $mapRaw)

    Write-Host "== anonymous price-compare =="
    $cmpRaw = & curl.exe -sS -w " HTTP:%{http_code}" `
        "$base/api/v1/connector/price-compare?couponId=$couponId"
    $cmpBody = Strip-HttpTrailer $cmpRaw
    Write-Host $cmpBody
    $cmp = $cmpBody | ConvertFrom-Json
    if ($cmp.code -ne 0 -and $cmp.code -ne 200) { throw "price-compare failed" }
    $items = @($cmp.data.items)
    if ($items.Count -lt 2) {
        throw ("expected >=2 items, got {0}" -f $items.Count)
    }
    $platforms = ($items | ForEach-Object { $_.platform }) -join ','
    Write-Host ("PASS price-compare items={0} platforms={1}" -f $items.Count, $platforms)
}
finally {
    Write-Host "== cleanup =="
    Stop-SmokeProcs $started
}
