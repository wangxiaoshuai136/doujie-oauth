[CmdletBinding()]
param(
  [string]$AuthDir = 'D:\WenPaul\Documents\路由网关\EasyCLIProxyAPI\oauth',
  [string]$AppPath = 'D:\Doubao\app\Doubao.exe',
  [string]$LogDir = 'D:\WenPaul\Documents\豆包REA\outputs\daily-credit'
)

$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
$stamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
$lines = [System.Collections.Generic.List[string]]::new()
$lines.Add("[$stamp] daily Doubao credit check")

$running = @(Get-Process -Name Doubao -ErrorAction SilentlyContinue)
if ($running.Count -eq 0) {
  if (-not (Test-Path -LiteralPath $AppPath)) {
    $lines.Add("ERROR desktop app not found: $AppPath")
  } else {
    [System.Diagnostics.Process]::Start($AppPath) | Out-Null
    Start-Sleep -Seconds 5
    $lines.Add('desktop app launched')
  }
} else {
  $lines.Add("desktop app already running (pid=$($running[0].Id))")
}

$accounts = @(Get-ChildItem -LiteralPath $AuthDir -File -Filter 'doubao*.json' | Sort-Object Name)
if ($accounts.Count -eq 0) {
  $lines.Add("ERROR no Doubao credentials found in $AuthDir")
}

$uri = 'https://www.doubao.com/commerce/benefit_supply/credit/get_credit_num_optional_tasks'
$statusNames = @{ 1 = '待领取'; 2 = '已领取'; 3 = '进行中' }
$taskNames = @{ 1 = 'App'; 2 = '电脑端' }

foreach ($file in $accounts) {
  try {
    $auth = Get-Content -LiteralPath $file.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
    $cookies = $auth.cookies
    if (-not $cookies -or -not $cookies.sessionid) {
      $lines.Add("$($file.Name): ERROR missing sessionid")
      continue
    }
    $cookieHeader = ($cookies.PSObject.Properties | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join '; '
    $response = Invoke-RestMethod -Uri $uri -Method Post -Body '{"need_tasks":true}' -ContentType 'application/json' -Headers @{
      Cookie = $cookieHeader
      'Agw-Js-Conv' = 'str'
      Referer = 'https://www.doubao.com/chat/'
      Origin = 'https://www.doubao.com'
      'User-Agent' = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/148.0.0.0 Safari/537.36'
    } -TimeoutSec 20
    if ($response.code -ne 0) {
      $lines.Add("$($file.Name): ERROR code=$($response.code) $($response.msg)")
      continue
    }
    $data = $response.data
    $label = if ($auth.email) { $auth.email } elseif ($auth.label) { $auth.label } else { $file.BaseName }
    $credit = $data.credit_info.total_credit_num
    $part = "[$label] credit=$credit"
    foreach ($task in @($data.optional_tasks)) {
      $kind = $taskNames[[int]$task.task_type]
      if (-not $kind) { $kind = "type$($task.task_type)" }
      $state = $statusNames[[int]$task.button_status]
      if (-not $state) { $state = "status$($task.button_status)" }
      $part += "; $kind=$state"
    }
    $lines.Add($part)
  } catch {
    $lines.Add("$($file.Name): ERROR $($_.Exception.Message)")
  }
}

$logPath = Join-Path $LogDir ("daily-credit-" + (Get-Date -Format 'yyyyMMdd') + ".log")
$lines | ForEach-Object { $_ }
$lines | Add-Content -LiteralPath $logPath -Encoding UTF8
