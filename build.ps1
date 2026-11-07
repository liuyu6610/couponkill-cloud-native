# CouponKill 本地构建入口（Windows PowerShell 5.1+ / PowerShell 7）。目标名与根 Makefile 相同。
# 发布职责不在这里：集群 CD 以 Jenkinsfile 为准（docs/CICD-SOURCE-OF-TRUTH.md）。
#
#   .\build.ps1 help
#   .\build.ps1 build                      # 编译五个服务到 .\bin
#   .\build.ps1 test -NoRace               # 没有 gcc 时关闭竞态检测
#   .\build.ps1 images -Tag 123            # 构建 couponkill-<svc>:123
#   .\build.ps1 deploy-chart-canary -StableTag 122 -Tag 123
#
# kubebuilder Operator 的 manifests / generate / install / deploy 等目标依赖 bash 工具链，
# 请在 Git Bash / WSL 里用根 Makefile 执行（make manifests generate；operator 测试用 make operator-test）。

param(
    [Parameter(Position = 0)]
    [string]$Command = "help",
    [string]$Tag = "",
    [string]$StableTag = "",
    [string]$Registry = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker",
    [string]$Release = "couponkill",
    [string]$Namespace = "couponkill",
    [string]$BaseUrl = "http://127.0.0.1:8088",
    [ValidateSet("", "app")]
    [string]$Profile = "",
    [switch]$NoRace,
    [switch]$Volumes
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

# 服务 → 容器端口（EXPOSE 元数据；与设计文档 D3 一致）
$Services = [ordered]@{ gateway = "8088"; user = "8081"; coupon = "8080"; order = "8082"; connector = "8085" }
$Chart = "./charts/couponkill"

function Invoke-Native {
    # 运行外部命令，非 0 退出码即抛错（PowerShell 默认不会因为外部命令失败而停下）
    param([string]$File, [string[]]$Arguments)
    Write-Host ">> $File $($Arguments -join ' ')" -ForegroundColor DarkGray
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File 退出码 $LASTEXITCODE" }
}

function Get-ImageTag {
    if ($Tag) { return $Tag }
    $sha = (git rev-parse --short HEAD 2>$null)
    if ($LASTEXITCODE -eq 0 -and $sha) { return $sha.Trim() }
    return "dev"
}

function Show-Help {
    Write-Host "Usage: .\build.ps1 <target> [-Tag <镜像标签>] [-Registry <仓库>] [-NoRace] ..."
    Write-Host ""
    Write-Host "Go services:"
    Write-Host "  build               Build the five service binaries into .\bin"
    Write-Host "  test                Run Go tests (-NoRace to disable the race detector; set CK_TEST_PG_DSN for PostgreSQL tests)"
    Write-Host "  vet                 go vet"
    Write-Host "  fmt                 gofmt -w"
    Write-Host "  lint                gofmt check + go vet + helm lint"
    Write-Host "  chart-test          helm template render assertions (needs helm on PATH)"
    Write-Host "  images              Build the five service images (couponkill-<svc>:<tag>)"
    Write-Host "  push                Build and push the images to -Registry"
    Write-Host "  up                  docker compose up -d (-Profile app: also the five services)"
    Write-Host "  down                docker compose down (-Volumes drops the data volumes)"
    Write-Host "  smoke               Run the end-to-end test against a running stack (-BaseUrl)"
    Write-Host ""
    Write-Host "Helm:"
    Write-Host "  deploy-chart        helm upgrade --install with dev defaults (chart generates secrets)"
    Write-Host "  deploy-chart-prod   helm upgrade --install with values-prod.yaml (create the Secret first)"
    Write-Host "  deploy-chart-canary Canary: -StableTag stays stable, coupon/order canary pods run -Tag"
}

function Build-Services {
    New-Item -ItemType Directory -Force -Path bin | Out-Null
    $env:CGO_ENABLED = "0"
    foreach ($svc in $Services.Keys) {
        $out = if ($IsLinux -or $IsMacOS) { "bin/$svc" } else { "bin/$svc.exe" }
        Invoke-Native go @("build", "-trimpath", "-ldflags", "-s -w", "-o", $out, "./cmd/$svc")
    }
}

function Test-Services {
    $goArgs = @("test")
    if (-not $NoRace) { $goArgs += "-race" }
    $goArgs += @("-count=1", "./cmd/...", "./internal/...")
    Invoke-Native go $goArgs
}

function Format-Services {
    Invoke-Native gofmt @("-w", "cmd", "internal")
}

function Invoke-Lint {
    Invoke-Native go @("vet", "./cmd/...", "./internal/...")
    $unformatted = & gofmt -l cmd internal
    if ($unformatted) { throw "以下文件未 gofmt:`n$($unformatted -join "`n")" }
    Invoke-Native helm @("lint", $Chart)
    Invoke-Native helm @("lint", $Chart, "-f", "$Chart/values-prod.yaml", "--set-string", "image.tag=lint")
    Invoke-Native helm @("lint", $Chart, "-f", "$Chart/values.canary-keda.yaml")
}

function Test-Chart {
    Push-Location charts/couponkill/tests/render
    try { Invoke-Native go @("test", "-count=1", "./...") } finally { Pop-Location }
}

function Build-Images {
    $imageTag = Get-ImageTag
    $sha = (git rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $sha) { $sha = "unknown" }
    foreach ($svc in $Services.Keys) {
        Invoke-Native docker @("build", "-f", "build/Dockerfile",
            "--build-arg", "SERVICE=$svc", "--build-arg", "PORT=$($Services[$svc])", "--build-arg", "VCS_REF=$($sha.Trim())",
            "-t", "couponkill-${svc}:$imageTag", "-t", "$Registry/couponkill-${svc}:$imageTag", ".")
    }
}

function Push-Images {
    Build-Images
    $imageTag = Get-ImageTag
    foreach ($svc in $Services.Keys) {
        Invoke-Native docker @("push", "$Registry/couponkill-${svc}:$imageTag")
    }
}

function Start-Stack {
    $composeArgs = @("compose")
    if ($Profile) { $composeArgs += @("--profile", $Profile) }
    $composeArgs += @("up", "-d")
    if ($Profile -eq "app") { $composeArgs += "--build" }
    Invoke-Native docker $composeArgs
}

function Stop-Stack {
    $composeArgs = @("compose", "--profile", "app", "down")
    if ($Volumes) { $composeArgs += "-v" }
    Invoke-Native docker $composeArgs
}

function Invoke-Smoke {
    $env:CK_E2E_BASE_URL = $BaseUrl
    Invoke-Native go @("test", "-count=1", "./test/e2e/...")
}

function Deploy-Chart {
    Invoke-Native helm @("upgrade", "--install", $Release, $Chart, "--namespace", $Namespace, "--create-namespace",
        "--set-string", "image.tag=$(Get-ImageTag)")
}

function Deploy-ChartProd {
    Invoke-Native helm @("upgrade", "--install", $Release, $Chart, "--namespace", $Namespace, "--create-namespace",
        "-f", "$Chart/values-prod.yaml", "--set-string", "image.tag=$(Get-ImageTag)")
}

function Deploy-ChartCanary {
    if (-not $StableTag) { throw "deploy-chart-canary 需要 -StableTag <稳定版镜像标签>" }
    $imageTag = Get-ImageTag
    Invoke-Native helm @("upgrade", "--install", $Release, $Chart, "--namespace", $Namespace, "--create-namespace",
        "-f", "$Chart/values-prod.yaml", "-f", "$Chart/values.canary-keda.yaml",
        "--set-string", "image.tag=$StableTag",
        "--set-string", "canary.coupon.tag=$imageTag", "--set-string", "canary.order.tag=$imageTag")
}

switch ($Command) {
    "help" { Show-Help }
    "build" { Build-Services }
    "test" { Test-Services }
    "vet" { Invoke-Native go @("vet", "./cmd/...", "./internal/...") }
    "fmt" { Format-Services }
    "lint" { Invoke-Lint }
    "chart-test" { Test-Chart }
    "images" { Build-Images }
    "push" { Push-Images }
    "up" { Start-Stack }
    "down" { Stop-Stack }
    "smoke" { Invoke-Smoke }
    "deploy-chart" { Deploy-Chart }
    "deploy-chart-prod" { Deploy-ChartProd }
    "deploy-chart-canary" { Deploy-ChartCanary }
    default {
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
