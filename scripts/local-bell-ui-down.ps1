# 清理 local-bell-ui-up.ps1 拉起的进程
$ErrorActionPreference = "Continue"
$stateFile = Join-Path $env:TEMP 'couponkill-bell-ui-state.json'
if (Test-Path $stateFile) {
    $s = Get-Content $stateFile -Raw | ConvertFrom-Json
    foreach ($id in @($s.procIds)) {
        Stop-Process -Id $id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item $stateFile -Force -ErrorAction SilentlyContinue
}
Get-CimInstance Win32_Process -Filter "Name='java.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -match 'couponkill-(user|gateway|order)-service|couponkilluserservice|couponkillgateway|couponkillorderservice' } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Get-CimInstance Win32_Process -Filter "Name='node.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -match 'vite' -and $_.CommandLine -match 'couponkill-frontend' } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Write-Host "bell-ui down done"
