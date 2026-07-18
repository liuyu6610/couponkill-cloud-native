# 本地 HTTP JWT 冒烟：启动 user/gateway/order/coupon → login → order 双路径 + coupon available → 清理
# 前置：docker compose -f docker-compose.migration.yml up -d；powershell -File scripts/import-nacos-local.ps1
# 用法：powershell -NoProfile -File scripts/local-http-smoke.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $env:JAVA_HOME) {
    $env:JAVA_HOME = 'C:\Program Files\Java\latest\jdk-25'
}
$env:Path = "$env:JAVA_HOME\bin;" + $env:Path
$env:NACOS_SERVER_ADDR = '127.0.0.1:8848'
$env:NACOS_NAMESPACE = '120'

$logDir = Join-Path $env:TEMP 'couponkill-http-smoke'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

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

# 端口被占用 ≠ 目标服务已就绪（曾出现 8082 被 service-catalog 占用导致网关 503）
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
        throw ("{0}: 端口 {1} 无监听进程" -f $Label, $Port)
    }
    if ($cmd -notmatch $Pattern) {
        throw ("{0}: 端口 {1} 被非目标进程占用 → {2}" -f $Label, $Port, $cmd.Substring(0, [Math]::Min(180, $cmd.Length)))
    }
}

function Start-Module([string]$ModuleDir, [string]$Name, [int]$Port) {
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    # 必须 -f 子模块 POM：禁止 -am spring-boot:run（会落到无 main 的 parent）
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
        $p = $it.Proc
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
    }
    Get-CimInstance Win32_Process -Filter "Name='java.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match 'couponkill-(user|gateway|order|coupon)-service' } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

$started = @()
try {
    Write-Host "== install common (clean) =="
    & mvn.cmd -pl couponkill-common -am clean install -DskipTests -q
    if ($LASTEXITCODE -ne 0) { throw "common install failed" }

    function Strip-HttpTrailer([string]$Raw) {
        # curl -w 追加的 "HTTP:200" 可能与 JSON 同一行
        return (($Raw -replace '\s*HTTP:\d+\s*$', '') -replace '[\r\n]+$', '').Trim()
    }

    Write-Host "== start modules =="
    $started += Start-Module 'couponkill-user-service' 'user' 8081
    $started += Start-Module 'couponkill-gateway' 'gateway' 8088
    $started += Start-Module 'couponkill-order-service' 'order' 8082
    $started += Start-Module 'couponkill-coupon-service' 'coupon' 8080

    $ready = @{}
    $ownerPattern = @{
        user    = 'couponkilluserservice|couponkill-user-service'
        gateway = 'couponkillgateway|couponkill-gateway'
        order   = 'couponkillorderservice|couponkill-order-service'
        coupon  = 'couponkillcouponservice|couponkill-coupon-service'
    }
    foreach ($it in $started) {
        $ok = Wait-Port $it.Port 90
        $ready[$it.Name] = $ok
        Write-Host ("port {0} ({1}) ready={2}" -f $it.Port, $it.Name, $ok)
        if (-not $ok) {
            Write-Host "--- $($it.Name) err tail ---"
            Get-Content $it.Err -Tail 80 -ErrorAction SilentlyContinue
            Write-Host "--- $($it.Name) out tail ---"
            Get-Content $it.Out -Tail 40 -ErrorAction SilentlyContinue
        } elseif ($ownerPattern.ContainsKey($it.Name)) {
            Assert-PortOwnedBy $it.Port $ownerPattern[$it.Name] $it.Name
            Write-Host ("port {0} ({1}) owner=OK" -f $it.Port, $it.Name)
        }
    }

    if (-not ($ready['user'] -and $ready['gateway'])) {
        throw "user/gateway 未就绪，中止冒烟"
    }
    if (-not $ready['coupon']) {
        throw "coupon 未就绪，中止冒烟"
    }

    Write-Host "== login =="
    $loginRaw = & curl.exe -s -w "`nHTTP:%{http_code}" -X POST `
        "http://127.0.0.1:8088/api/v1/user/login" `
        -H "Content-Type: application/x-www-form-urlencoded" `
        -d "username=demo&password=demo1234"
    Write-Host $loginRaw

    $token = [regex]::Match($loginRaw, '"token"\s*:\s*"([^"]+)"').Groups[1].Value
    if (-not $token) {
        $token = [regex]::Match($loginRaw, '(eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+)').Groups[1].Value
    }
    if (-not $token) { throw "未能从登录响应解析 token" }
    Write-Host ("TOKEN_LEN={0}" -f $token.Length)

    # Long ID 契约：userId 必须以 JSON 字符串写出（非 number）
    $loginObj = (Strip-HttpTrailer $loginRaw) | ConvertFrom-Json
    $loginUserId = $loginObj.data.userId
    if ($null -eq $loginUserId) { throw "登录响应缺少 data.userId" }
    if ($loginUserId -isnot [string]) {
        throw ("登录响应 userId 类型={0}，期望 string（ToString 未生效）" -f $loginUserId.GetType().Name)
    }
    Write-Host ("LOGIN_USER_ID(string)={0}" -f $loginUserId)

    if (-not $ready['order']) {
        throw "order 未就绪，中止冒烟"
    }

    Write-Host "== dual order paths =="
    $old = & curl.exe -s -w "`nHTTP:%{http_code}" `
        -H "Authorization: Bearer $token" `
        "http://127.0.0.1:8088/order/user/me?pageNum=1&pageSize=5"
    $new = & curl.exe -s -w "`nHTTP:%{http_code}" `
        -H "Authorization: Bearer $token" `
        "http://127.0.0.1:8088/api/v1/order/user/me?pageNum=1&pageSize=5"
    Write-Host "OLD=>$old"
    Write-Host "NEW=>$new"

    Write-Host "== coupon available =="
    $coupon = & curl.exe -s -w "`nHTTP:%{http_code}" `
        -H "Authorization: Bearer $token" `
        "http://127.0.0.1:8088/api/v1/coupon/available"
    Write-Host "COUPON=>$coupon"

    $oldOk = ($old -match 'HTTP:200') -and ($old -match '"code"\s*:\s*0')
    $newOk = ($new -match 'HTTP:200') -and ($new -match '"code"\s*:\s*0')
    $couponOk = ($coupon -match 'HTTP:200') -and ($coupon -match '"code"\s*:\s*0')
    if (-not ($oldOk -and $newOk -and $couponOk)) {
        throw "冒烟未同时通过 oldOk=$oldOk newOk=$newOk couponOk=$couponOk"
    }

    # 订单/券 ID：用正则校验（避免控制台编码损坏中文后 ConvertFrom-Json 失败）
    function Assert-IdStrings([string]$Raw, [string]$Label) {
        $json = Strip-HttpTrailer $Raw
        if ($Label -eq 'ORDER') {
            if ($json -match '"data"\s*:\s*\[\s*\]') {
                Write-Host 'ORDER ID_STRING_CHECK=SKIP (empty list)'
                return
            }
            if ([regex]::IsMatch($json, '"userId"\s*:\s*\d')) {
                throw 'ORDER userId still JSON number'
            }
            if ([regex]::IsMatch($json, '"couponId"\s*:\s*\d')) {
                throw 'ORDER couponId still JSON number'
            }
            if (-not [regex]::IsMatch($json, '"userId"\s*:\s*"')) {
                throw 'ORDER missing string userId'
            }
        }
        if ($Label -eq 'COUPON') {
            if ([regex]::IsMatch($json, '"id"\s*:\s*\d')) {
                throw 'COUPON id still JSON number'
            }
            if (-not [regex]::IsMatch($json, '"id"\s*:\s*"')) {
                throw 'COUPON missing string id'
            }
        }
        Write-Host ("{0} ID_STRING_CHECK=PASS" -f $Label)
    }

    # 空订单列表时创建一笔常驻券订单，便于校验 userId/couponId 字符串
    $newJson = Strip-HttpTrailer $new
    if ($newJson -match '"data"\s*:\s*\[\s*\]') {
        Write-Host '== create order for ID check =='
        $created = & curl.exe -s -w "`nHTTP:%{http_code}" -X POST `
            "http://127.0.0.1:8088/api/v1/order/create?couponId=1002" `
            -H "Authorization: Bearer $token"
        Write-Host "CREATE=>$created"
        $new = & curl.exe -s -w "`nHTTP:%{http_code}" `
            -H "Authorization: Bearer $token" `
            "http://127.0.0.1:8088/api/v1/order/user/me?pageNum=1&pageSize=5"
        Write-Host "NEW_AFTER_CREATE=>$new"
    }

    Assert-IdStrings $new 'ORDER'
    Assert-IdStrings $coupon 'COUPON'

    Write-Host "== SMOKE PASS =="
}
finally {
    Write-Host "== cleanup =="
    Stop-SmokeProcs $started
}
