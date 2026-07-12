# JDK25 Leyden AOT 训练脚本（可选，不改业务代码）
# 用法（管理员 PowerShell）：
#   powershell -File scripts/jdk25-aot-train.ps1
# 产出：order-service AOT 缓存目录，启动时加 -XX:AOTCache=... 可缩短冷启动（JEP 514/515）
param(
  [string]$Jar = "d:\job-project\couponkill-cloud-native\couponkill-order-service\target\couponkill-order-service-1.0-SNAPSHOT.jar",
  [string]$OutDir = "d:\job-project\couponkill-cloud-native\target\aot\order"
)

$ErrorActionPreference = "Stop"
$java = "D:\dev-lanuage\java\jdk-25\bin\java.exe"
if (-not (Test-Path $Jar)) { throw "JAR not found: $Jar — build order-service first" }
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host "Training AOT cache (app exits after short warmup)..."
& $java `
  -XX:+UseZGC -XX:+ZGenerational -XX:+UseCompactObjectHeaders `
  --enable-preview `
  "-XX:AOTMode=record" `
  "-XX:AOTConfiguration=$OutDir\order.aotconf" `
  -jar $Jar --spring.main.web-application-type=none 2>&1 | Select-Object -Last 20

Write-Host "Done. See OpenJDK Leyden docs to create AOTCache from AOTConfiguration."
Write-Host "Local run tip: set JAVA_TOOL_OPTIONS to include ZGC + CompactHeaders + --enable-preview"
