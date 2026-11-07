#Requires -Version 7.0
# 秒杀压测：批量注册并登录 N 个用户，用有界并发打 POST /api/v1/order/seckill，轮询 /seckill/result 直到终态，
# 最后输出 受理 / 售罄 / 限流 / 繁忙 / 其它错误 的计数、终态分布与延迟分位数（p50 / p95 / p99）。
#
#   pwsh -File scripts/loadtest.ps1                                   # 默认 200 用户、并发 50，打 http://127.0.0.1:8088
#   pwsh -File scripts/loadtest.ps1 -Users 2000 -Concurrency 200 -BaseUrl http://staging.example.com
#   pwsh -File scripts/loadtest.ps1 -CouponId 123456789012345678       # 指定秒杀券（默认取可用券里第一张 type=2 的）
#   pwsh -File scripts/loadtest.ps1 -SkipRegister -Prefix lt20261107   # 复用上次注册的用户
#
# 注意：
#   * 网关对每 IP / 每用户 / 秒杀入口有令牌桶限流（CK_GATEWAY_IP_RPS 等）。压测机只有一个 IP，
#     想测系统极限请在被测环境调高限流，否则大部分请求会得到 429（脚本会如实统计为「限流」）。
#   * 每个用户只发一次秒杀请求；同一用户重复秒杀得到 409（重复）。库存只会被扣到 0，所以重跑前
#     需要在被测环境重新预热库存（coupon 服务的 /preheat-stock 或重启 coupon）。
#   * 注册走 POST /api/v1/user/register（JSON）。design §10 的 POST /api/v1/user/batch/generate（admin）
#     没有给出请求/响应形状，所以本脚本不用它。
#
# 退出码：0 正常；1 有「其它错误」（非 202/409/429/503）、登录失败过多，或成功落单数超过券的库存（超卖）。
[CmdletBinding()]
param(
    [string]$BaseUrl = "",
    [string]$CouponId = "",
    [int]$Users = 200,
    # 秒杀请求的最大并发
    [int]$Concurrency = 50,
    # 注册 / 登录阶段的最大并发（bcrypt 吃 CPU，别开太大）
    [int]$SetupConcurrency = 16,
    [string]$Prefix = "",
    [string]$Password = "Passw0rd!loadtest",
    [switch]$SkipRegister,
    # 轮询落单结果的最长秒数（从入队完成起算）与间隔
    [int]$PollTimeoutSec = 60,
    [int]$PollIntervalMs = 250,
    # 单个 HTTP 请求的超时秒数
    [int]$RequestTimeoutSec = 10
)

$ErrorActionPreference = "Stop"
if (-not $BaseUrl) { $BaseUrl = $env:CK_E2E_BASE_URL }
if (-not $BaseUrl) { $BaseUrl = "http://127.0.0.1:8088" }
$BaseUrl = $BaseUrl.TrimEnd("/")
if (-not $Prefix) { $Prefix = "lt" + (Get-Date -Format "yyyyMMddHHmmss") }
if ($Users -lt 1) { throw "-Users 必须 >= 1" }

# 一个共享的 HttpClient（线程安全）；连接数放开，避免客户端自己成为瓶颈
$handler = [System.Net.Http.SocketsHttpHandler]::new()
$handler.MaxConnectionsPerServer = [Math]::Max($Concurrency, $SetupConcurrency) * 2
$handler.PooledConnectionLifetime = [TimeSpan]::FromMinutes(5)
$client = [System.Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds($RequestTimeoutSec)

function Write-Step([string]$Message) { Write-Host "== $Message" -ForegroundColor Cyan }

function Get-Percentile([double[]]$Sorted, [double]$P) {
    # 最近秩法：已升序数组
    if ($Sorted.Count -eq 0) { return [double]::NaN }
    $rank = [Math]::Ceiling($P / 100.0 * $Sorted.Count)
    return $Sorted[[Math]::Max(0, [Math]::Min($Sorted.Count - 1, [int]$rank - 1))]
}

function Format-Latency([double[]]$Values) {
    if ($Values.Count -eq 0) { return "n/a（没有样本）" }
    $s = $Values | Sort-Object
    return ("p50={0:N1}ms  p95={1:N1}ms  p99={2:N1}ms  max={3:N1}ms  (n={4})" -f
        (Get-Percentile $s 50), (Get-Percentile $s 95), (Get-Percentile $s 99), $s[-1], $s.Count)
}

function Invoke-Json([string]$Method, [string]$Path, $Body = $null, [string]$Token = "") {
    $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), "$BaseUrl$Path")
    if ($Token) { $req.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $Token) }
    if ($null -ne $Body) {
        $req.Content = [System.Net.Http.StringContent]::new(($Body | ConvertTo-Json -Compress), [Text.Encoding]::UTF8, "application/json")
    }
    $resp = $client.SendAsync($req).GetAwaiter().GetResult()
    $text = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    $json = $null
    try { $json = $text | ConvertFrom-Json -Depth 20 } catch { $json = $null }
    return [pscustomobject]@{ Status = [int]$resp.StatusCode; Body = $json; Raw = $text }
}

# ---- 0. 连通性与秒杀券 -------------------------------------------------------------
Write-Step "检查 $BaseUrl"
$ready = Invoke-Json GET "/readyz"
if ($ready.Status -ne 200) { throw "$BaseUrl/readyz 返回 $($ready.Status)；栈起来了吗？（scripts/dev-up.ps1）" }

$stock = $null
if (-not $CouponId) {
    $list = Invoke-Json GET "/api/v1/coupon/available"
    if ($list.Status -ne 200) { throw "查询可用券失败：HTTP $($list.Status) $($list.Raw)" }
    $items = if ($list.Body.data -is [array]) { $list.Body.data } else { $list.Body.data.items }
    $pick = $items | Where-Object { $_.type -eq 2 } | Select-Object -First 1
    if (-not $pick) { throw "可用券里没有秒杀券（type=2）。用 -CouponId 指定，或以 CK_SEED_DEMO=true 启动。" }
    $CouponId = [string]$pick.id
    $stock = $pick.seckillTotalStock
    Write-Host "秒杀券：$($pick.name)（id $CouponId，库存 $stock）"
}
else {
    $detail = Invoke-Json GET "/api/v1/coupon/$CouponId"
    if ($detail.Status -eq 200) { $stock = $detail.Body.data.seckillTotalStock }
}

# ---- 1. 注册 + 登录 ---------------------------------------------------------------
$accounts = @()
$sw = [Diagnostics.Stopwatch]::StartNew()
Write-Step "注册并登录 $Users 个用户（前缀 $Prefix，并发 $SetupConcurrency）"
$setup = 1..$Users | ForEach-Object -ThrottleLimit $SetupConcurrency -Parallel {
    $i = $_
    $c = $using:client
    $base = $using:BaseUrl
    $name = "{0}u{1:d6}" -f $using:Prefix, $i
    $pass = $using:Password
    $phone = "13{0:d9}" -f $i
    $send = {
        param($path, $obj)
        $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Post, "$base$path")
        $req.Content = [System.Net.Http.StringContent]::new(($obj | ConvertTo-Json -Compress), [Text.Encoding]::UTF8, "application/json")
        try {
            $r = $c.SendAsync($req).GetAwaiter().GetResult()
            $t = $r.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            return [pscustomobject]@{ Status = [int]$r.StatusCode; Text = $t }
        }
        catch { return [pscustomobject]@{ Status = 0; Text = $_.Exception.Message } }
    }
    $registered = $true
    if (-not $using:SkipRegister) {
        $reg = & $send "/api/v1/user/register" @{ username = $name; password = $pass; phone = $phone }
        # 200 注册成功；409 已存在（重跑）也算可以继续登录
        $registered = ($reg.Status -eq 200 -or $reg.Status -eq 409)
    }
    $token = $null; $userId = $null; $err = $null
    if ($registered) {
        $login = & $send "/api/v1/user/login" @{ username = $name; password = $pass }
        if ($login.Status -eq 200) {
            $data = ($login.Text | ConvertFrom-Json).data
            $token = [string]$data.token
            $userId = [string]$data.userId
        }
        else { $err = "login HTTP $($login.Status)" }
    }
    else { $err = "register HTTP $($reg.Status)" }
    [pscustomobject]@{ Index = $i; Name = $name; Token = $token; UserId = $userId; Error = $err }
}
$accounts = @($setup | Where-Object { $_.Token })
$setupFailed = $Users - $accounts.Count
Write-Host ("可用用户 {0}/{1}，用时 {2:N1}s" -f $accounts.Count, $Users, $sw.Elapsed.TotalSeconds)
if ($setupFailed -gt 0) {
    $setup | Where-Object { -not $_.Token } | Group-Object Error | ForEach-Object { Write-Host ("  失败原因 {0}: {1}" -f $_.Name, $_.Count) -ForegroundColor Yellow }
}
if ($accounts.Count -eq 0) { throw "没有任何可用用户，无法压测" }

# ---- 2. 秒杀 ----------------------------------------------------------------------
Write-Step "秒杀：$($accounts.Count) 个请求，并发 $Concurrency，券 $CouponId"
$sw.Restart()
$fire = $accounts | ForEach-Object -ThrottleLimit $Concurrency -Parallel {
    $acc = $_
    $c = $using:client
    $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Post, "$($using:BaseUrl)/api/v1/order/seckill")
    $req.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $acc.Token)
    $req.Content = [System.Net.Http.StringContent]::new((@{ couponId = $using:CouponId } | ConvertTo-Json -Compress), [Text.Encoding]::UTF8, "application/json")
    $t0 = [Diagnostics.Stopwatch]::StartNew()
    $status = 0; $code = $null; $requestId = $null; $note = $null
    try {
        $r = $c.SendAsync($req).GetAwaiter().GetResult()
        $text = $r.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        $status = [int]$r.StatusCode
        try {
            $j = $text | ConvertFrom-Json
            $code = $j.code
            if ($j.data) { $requestId = $j.data.requestId }
        }
        catch { $note = "非 JSON 响应" }
    }
    catch { $note = $_.Exception.Message }
    $t0.Stop()
    [pscustomobject]@{
        UserId = $acc.UserId; Token = $acc.Token; Status = $status; Code = $code; RequestId = $requestId
        Ms = $t0.Elapsed.TotalMilliseconds; DoneAt = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds(); Note = $note
    }
}
$fireSeconds = $sw.Elapsed.TotalSeconds

# 分类：202 受理；409+库存不足码 = 售罄；其它 409 = 冲突（重复秒杀等）；429 限流；503 繁忙；其它 = 错误
$soldOutCode = 10002 # apperr.OutOfStock（设计文档 §4 的示例：{"code":10002,"message":"库存不足"}）
$accepted = @($fire | Where-Object { $_.Status -eq 202 })
$soldOut = @($fire | Where-Object { $_.Status -eq 409 -and $_.Code -eq $soldOutCode })
$conflict = @($fire | Where-Object { $_.Status -eq 409 -and $_.Code -ne $soldOutCode })
$limited = @($fire | Where-Object { $_.Status -eq 429 })
$busy = @($fire | Where-Object { $_.Status -eq 503 })
$errors = @($fire | Where-Object { $_.Status -notin 202, 409, 429, 503 })

# ---- 3. 轮询落单结果 ----------------------------------------------------------------
$results = @()
if ($accepted.Count -gt 0) {
    Write-Step "轮询 $($accepted.Count) 个受理请求的结果（最长 ${PollTimeoutSec}s）"
    $results = $accepted | ForEach-Object -ThrottleLimit $Concurrency -Parallel {
        $a = $_
        $c = $using:client
        $deadline = [DateTimeOffset]::UtcNow.AddSeconds($using:PollTimeoutSec)
        $final = $null; $reason = $null; $last = "NONE"
        while ([DateTimeOffset]::UtcNow -lt $deadline) {
            $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get,
                "$($using:BaseUrl)/api/v1/order/seckill/result?requestId=$([Uri]::EscapeDataString($a.RequestId))")
            $req.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $a.Token)
            try {
                $r = $c.SendAsync($req).GetAwaiter().GetResult()
                $text = $r.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                if ([int]$r.StatusCode -eq 200) {
                    $d = ($text | ConvertFrom-Json).data
                    $last = [string]$d.status
                    if ($last -in "SUCCESS", "FAILED") { $final = $last; $reason = [string]$d.reason; break }
                }
                else { $last = "HTTP $([int]$r.StatusCode)" }
            }
            catch { $last = "ERR" }
            Start-Sleep -Milliseconds $using:PollIntervalMs
        }
        [pscustomobject]@{
            Final = if ($final) { $final } else { "PENDING($last)" }; Reason = $reason
            EndToEndMs = ([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() - $a.DoneAt) + $a.Ms
        }
    }
}
$success = @($results | Where-Object { $_.Final -eq "SUCCESS" })
$failed = @($results | Where-Object { $_.Final -eq "FAILED" })
$pending = @($results | Where-Object { $_.Final -like "PENDING*" })

# ---- 4. 报告 ----------------------------------------------------------------------
Write-Host ""
Write-Host "==================== 压测结果 ====================" -ForegroundColor Green
Write-Host ("目标            {0}   券 {1}（库存 {2}）" -f $BaseUrl, $CouponId, $(if ($null -ne $stock) { $stock } else { "未知" }))
Write-Host ("请求            {0}，用时 {1:N2}s，吞吐 {2:N1} req/s（并发 {3}）" -f $fire.Count, $fireSeconds, ($fire.Count / [Math]::Max($fireSeconds, 0.001)), $Concurrency)
Write-Host ("  accepted      {0}   (202 已入队)" -f $accepted.Count)
Write-Host ("  sold-out      {0}   (409 库存不足)" -f $soldOut.Count)
Write-Host ("  conflict      {0}   (409 其它：重复秒杀 / 未开始 / 已结束 ...)" -f $conflict.Count)
Write-Host ("  rate-limited  {0}   (429)" -f $limited.Count)
Write-Host ("  busy          {0}   (503)" -f $busy.Count)
Write-Host ("  error         {0}   (其它状态码 / 网络错误 / 超时)" -f $errors.Count)
if ($errors.Count -gt 0) {
    $errors | Group-Object { "HTTP $($_.Status) $($_.Note)" } | Sort-Object Count -Descending | Select-Object -First 5 |
        ForEach-Object { Write-Host ("      {0} × {1}" -f $_.Count, $_.Name) -ForegroundColor Yellow }
}
if ($conflict.Count -gt 0) {
    $conflict | Group-Object Code | ForEach-Object { Write-Host ("      conflict code {0} × {1}" -f $_.Name, $_.Count) }
}
Write-Host ("入队延迟        {0}" -f (Format-Latency @($accepted | ForEach-Object { $_.Ms })))
Write-Host ("全部请求延迟    {0}" -f (Format-Latency @($fire | ForEach-Object { $_.Ms })))
if ($accepted.Count -gt 0) {
    Write-Host ("落单结果        SUCCESS {0}   FAILED {1}   未出终态 {2}" -f $success.Count, $failed.Count, $pending.Count)
    if ($failed.Count -gt 0) {
        $failed | Group-Object Reason | ForEach-Object { Write-Host ("      FAILED reason {0} × {1}" -f $_.Name, $_.Count) }
    }
    Write-Host ("端到端延迟      {0}   (受理→终态，含轮询间隔 {1}ms)" -f (Format-Latency @($results | Where-Object { $_.Final -in "SUCCESS", "FAILED" } | ForEach-Object { $_.EndToEndMs })), $PollIntervalMs)
}
Write-Host "==================================================" -ForegroundColor Green

$exit = 0
if ($errors.Count -gt 0) { Write-Host "存在其它错误，见上" -ForegroundColor Red; $exit = 1 }
if ($setupFailed -gt ($Users * 0.1)) { Write-Host "超过 10% 的用户注册/登录失败" -ForegroundColor Red; $exit = 1 }
if ($null -ne $stock -and $success.Count -gt [int]$stock) {
    Write-Host ("超卖！成功落单 {0} > 库存 {1}" -f $success.Count, $stock) -ForegroundColor Red
    $exit = 1
}
if ($pending.Count -gt 0) { Write-Host "有 $($pending.Count) 个请求在 ${PollTimeoutSec}s 内没有出终态（消费者积压或 sweeper 尚未处理）" -ForegroundColor Yellow }
$client.Dispose()
exit $exit
