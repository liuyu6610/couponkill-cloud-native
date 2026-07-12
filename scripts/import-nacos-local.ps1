# 将仓库 nacos/ 配置导入本地 Nacos 3.x，并把 Docker 内网主机名替换为 localhost
# 用法：pwsh -File scripts/import-nacos-local.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Nacos = "http://127.0.0.1:8848"
$TenantDev = "120"
$TenantShard = "998"

function Wait-Nacos {
    for ($i = 0; $i -lt 60; $i++) {
        $code = & curl.exe -s -o NUL -w "%{http_code}" "$Nacos/nacos/"
        if ($code -eq "200") { return }
        Start-Sleep -Seconds 2
    }
    throw "Nacos not ready: $Nacos"
}

function Ensure-Namespace([string]$id, [string]$name) {
    & curl.exe -s -X POST "$Nacos/nacos/v1/console/namespaces" `
        -d "customNamespaceId=$id&namespaceName=$name&namespaceDesc=couponkill-local" | Out-Null
    Write-Host "namespace ensure: $id ($name)"
}

function Localize([string]$text) {
    if ([string]::IsNullOrEmpty($text)) { return $text }
    return $text `
        -replace "jdbc:postgresql://postgres:5432", "jdbc:postgresql://127.0.0.1:5432" `
        -replace "host=postgres port=5432", "host=127.0.0.1 port=5432" `
        -replace "(?m)^(\s*host:\s*)redis-master\s*$", '${1}127.0.0.1' `
        -replace "(?m)^(\s*host:\s*)redis\s*$", '${1}127.0.0.1' `
        -replace "redis-master:6379", "127.0.0.1:6379" `
        -replace "kafka:9092", "127.0.0.1:9092" `
        -replace "kafka:29092", "127.0.0.1:9092" `
        -replace "nacos:8848", "127.0.0.1:8848" `
        -replace "(?m)^(  port:\s*)80(\s*(#.*)?)?$", '${1}8088${2}'
}

function Publish-Config([string]$dataId, [string]$group, [string]$tenant, [string]$content) {
    $tmp = Join-Path $env:TEMP ("nacos-" + [guid]::NewGuid().ToString() + ".txt")
    [System.IO.File]::WriteAllText($tmp, (Localize $content), [System.Text.UTF8Encoding]::new($false))
    $args = @(
        "-s", "-X", "POST", "$Nacos/nacos/v1/cs/configs",
        "-F", "dataId=$dataId",
        "-F", "group=$group",
        "-F", "type=yaml",
        "-F", "content=<$tmp"
    )
    if ($tenant) { $args += @("-F", "tenant=$tenant") }
    $ok = & curl.exe @args
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    if ("$ok" -ne "true") { throw "publish failed: $dataId@$group tenant=$tenant => $ok" }
    Write-Host "OK $tenant/$group/$dataId"
}

Write-Host "== waiting nacos =="
Wait-Nacos
Ensure-Namespace $TenantDev "couponkill-dev"
Ensure-Namespace $TenantShard "couponkill-shard"

$defaultDir = Join-Path $Root "nacos\DEFAULT_GROUP"
Get-ChildItem $defaultDir -Filter *.yaml | ForEach-Object {
    Publish-Config $_.Name "DEFAULT_GROUP" $TenantDev (Get-Content $_.FullName -Raw -Encoding UTF8)
}
Publish-Config "couponkill-gateway.yaml" "DEFAULT_GROUP" $TenantDev (Get-Content (Join-Path $defaultDir "couponkill-gateway-dev.yaml") -Raw -Encoding UTF8)
Publish-Config "couponkill-order-service.yaml" "DEFAULT_GROUP" $TenantDev (Get-Content (Join-Path $defaultDir "couponkill-order-service-dev.yaml") -Raw -Encoding UTF8)
Publish-Config "couponkill-coupon-service.yaml" "DEFAULT_GROUP" $TenantDev (Get-Content (Join-Path $defaultDir "couponkill-coupon-service-dev.yaml") -Raw -Encoding UTF8)
Publish-Config "couponkill-user-service.yaml" "DEFAULT_GROUP" $TenantDev (Get-Content (Join-Path $defaultDir "couponkill-user-service-dev.yaml") -Raw -Encoding UTF8)
Publish-Config "couponkill-connector-service.yaml" "DEFAULT_GROUP" $TenantDev (Get-Content (Join-Path $defaultDir "couponkill-connector-service-dev.yaml") -Raw -Encoding UTF8)

$shardDir = Join-Path $Root "nacos\shard\DEFAULT_GROUP"
Get-ChildItem $shardDir -Filter *.yaml | ForEach-Object {
    Publish-Config $_.Name "DEFAULT_GROUP" $TenantShard (Get-Content $_.FullName -Raw -Encoding UTF8)
}

$sent = Join-Path $Root "nacos\SENTINEL_GROUP\couponkill-gateway-sentinel"
if (Test-Path $sent) {
    $tmp = Join-Path $env:TEMP ("nacos-sent-" + [guid]::NewGuid().ToString() + ".txt")
    [System.IO.File]::WriteAllText($tmp, (Get-Content $sent -Raw -Encoding UTF8), [System.Text.UTF8Encoding]::new($false))
    $ok = & curl.exe -s -X POST "$Nacos/nacos/v1/cs/configs" -F "dataId=couponkill-gateway-sentinel" -F "group=SENTINEL_GROUP" -F "type=json" -F "tenant=$TenantDev" -F "content=<$tmp"
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    Write-Host "OK $TenantDev/SENTINEL_GROUP/couponkill-gateway-sentinel => $ok"
}

Write-Host "== nacos local import done =="
