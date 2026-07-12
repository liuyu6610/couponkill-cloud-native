# 本地短压测（Windows PowerShell 5.1+ / 无需 JMeter）
# 用法：powershell -File scripts/local-loadtest.ps1
param(
  [int]$Users = 30,
  [int]$Concurrency = 15,
  [string]$Gateway = "http://127.0.0.1:8088",
  [long]$CouponId = 1001
)

$ErrorActionPreference = "Continue"
Write-Host "== reset redis stock =="
docker exec couponkill-redis redis-cli SET "coupon:stock:$CouponId" 1600 | Out-Null

Write-Host "== register/login $Users users =="
$accounts = @()
for ($i = 1; $i -le $Users; $i++) {
  $u = "ltuser$i"
  $p = "pass1234"
  $phone = "1390000{0:d4}" -f $i
  curl.exe -s -X POST "$Gateway/api/v1/user/register?username=$u&password=$p&phone=$phone" | Out-Null
  $login = curl.exe -s -X POST "$Gateway/api/v1/user/login?username=$u&password=$p"
  try {
    $obj = $login | ConvertFrom-Json
    if ($obj.data.token) {
      $accounts += [pscustomobject]@{ userId = [string]$obj.data.userId; token = [string]$obj.data.token }
    }
  }
  catch {}
}
Write-Host "ready tokens: $($accounts.Count)"
if ($accounts.Count -lt 1) { throw "no tokens; check gateway/user service" }

$ok = 0; $fail = 0; $queued = 0
$lock = New-Object object
$sw = [Diagnostics.Stopwatch]::StartNew()
$jobs = @()
foreach ($acc in $accounts) {
  while ((@(Get-Job -State Running).Count) -ge $Concurrency) {
    Start-Sleep -Milliseconds 50
  }
  $jobs += Start-Job -ScriptBlock {
    param($gw, $cid, $token, $userId)
    curl.exe -s -m 12 -X POST "$gw/order/seckill?couponId=$cid" -H "Authorization: Bearer $token" -H "X-User-Id: $userId"
  } -ArgumentList $Gateway, $CouponId, $acc.token, $acc.userId
}
$results = $jobs | Wait-Job | Receive-Job
$jobs | Remove-Job -Force
$sw.Stop()

foreach ($body in $results) {
  if ($body -match '"status":"QUEUED"') { $ok++; $queued++ }
  elseif ($body -match '"success":true') { $ok++ }
  else { $fail++ }
}

Start-Sleep -Seconds 3
$stock = docker exec couponkill-redis redis-cli GET "coupon:stock:$CouponId"
Write-Host ""
Write-Host "==== LOAD RESULT ===="
Write-Host "users=$($accounts.Count) concurrency=$Concurrency elapsedMs=$($sw.ElapsedMilliseconds)"
Write-Host "ok=$ok fail=$fail queued=$queued"
Write-Host "redisStock=$stock (start 1600, delta=$((1600 - [int]$stock)))"
