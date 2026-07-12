# 本地启动 JDK25 性能参数（与 Helm JAVA_TOOL_OPTIONS 对齐）
# 用法：. .\scripts\jdk25-local-env.ps1  然后再 java -jar ...
$env:JAVA_TOOL_OPTIONS = "-XX:+UseZGC -XX:+ZGenerational -XX:+UseCompactObjectHeaders -XX:MaxRAMPercentage=75.0 --enable-preview"
Write-Host "JAVA_TOOL_OPTIONS=$env:JAVA_TOOL_OPTIONS"
