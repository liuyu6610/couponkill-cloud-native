# 端到端测试：对运行中的栈执行 `go test ./test/e2e/...`（注册 → 登录 → 查券 → 秒杀 → 轮询 → 取消 → 再抢）。
#
#   pwsh -File scripts/dev-up.ps1            # 先把栈起来（或 docker compose --profile app up -d --build）
#   pwsh -File scripts/e2e.ps1               # 默认打网关 http://127.0.0.1:8088
#   pwsh -File scripts/e2e.ps1 -BaseUrl http://staging.example.com -Run TestSeckillFlow -Verbose
#
# 目标地址通过 CK_E2E_BASE_URL 传给测试：-BaseUrl > 当前 shell 的 CK_E2E_BASE_URL > http://127.0.0.1:8088。
# 栈需要以 CK_SEED_DEMO=true 启动（测试用演示账号 demo/admin）。
[CmdletBinding()]
param(
    [string]$BaseUrl = "",
    [string]$Run = "",
    # 先等网关 /readyz 变为 200 的最长秒数；0 = 不等待
    [int]$WaitSec = 60
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location -LiteralPath $Root

if (-not $BaseUrl) { $BaseUrl = $env:CK_E2E_BASE_URL }
if (-not $BaseUrl) { $BaseUrl = "http://127.0.0.1:8088" }
$BaseUrl = $BaseUrl.TrimEnd("/")

if (-not (Test-Path -LiteralPath (Join-Path $Root "test/e2e"))) {
    throw "找不到 test/e2e（端到端测试尚未提交到当前分支）。"
}

if ($WaitSec -gt 0) {
    $deadline = (Get-Date).AddSeconds($WaitSec)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri "$BaseUrl/readyz" -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) { $ready = $true; break }
        }
        catch { Start-Sleep -Milliseconds 500 }
    }
    if (-not $ready) { throw "等待 $BaseUrl/readyz 超时（${WaitSec}s）。栈起来了吗？（scripts/dev-up.ps1）" }
}

$env:CK_E2E_BASE_URL = $BaseUrl
Write-Host "CK_E2E_BASE_URL=$BaseUrl" -ForegroundColor Cyan

$goArgs = @("test", "-count=1")
if ($VerbosePreference -eq "Continue") { $goArgs += "-v" }
if ($Run) { $goArgs += @("-run", $Run) }
$goArgs += "./test/e2e/..."
Write-Host ">> go $($goArgs -join ' ')" -ForegroundColor DarkGray
& go @goArgs
exit $LASTEXITCODE
