<#
.SYNOPSIS
  窓 B: irl-live-keeper の状態（/api/status）を一定の間隔で1行ずつ表示する（M-9 の Windows の確認用）。
.DESCRIPTION
  パスワードは -Password で渡すか、省略すれば同じフォルダの config.yaml の admin.password を読む。
  -Log を付けると、表示と同じ行をファイルにも書く。Ctrl+C で止める。
  -Csv を付けると、カメラごとの値（時刻、id、state、kbps、protocol、SRT の rtt_ms・loss_rate〔%〕・recv_mbps、
  scene、owner、stack）を CSV にも書く（T-15 の揺れの記録用。表計算ソフトで開ける）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-c1.txt
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-t15.txt -Csv .\t15.csv -IntervalMs 1000
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Once   # status の JSON を1回だけ出す（資格情報の入る urls を除く）
#>
param(
  [string]$Password = "",
  [string]$Url = "http://127.0.0.1:8787",
  [int]$IntervalMs = 500,
  [string]$Log = "",
  [string]$Csv = "",
  [string]$Config = "",
  [switch]$Once
)
$here = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
if ($Config -eq "") { $Config = Join-Path $here "config.yaml" }  # param の既定値では $PSScriptRoot が空になることがある（Windows PowerShell 5.1）

function Read-AdminPassword([string]$path) {
  if (-not (Test-Path $path)) { return "" }
  $inAdmin = $false
  foreach ($line in Get-Content -Path $path -Encoding UTF8) {
    if ($line -match '^\S') { $inAdmin = ($line -match '^admin:') ; continue }
    if ($inAdmin -and $line -match '^\s+password:\s*"?([^"#\s]+)"?') { return $Matches[1] }
  }
  return ""
}

if ($Password -eq "") { $Password = Read-AdminPassword $Config }
if ($Password -eq "") {
  $sec = Read-Host -AsSecureString "管理 API のパスワード（config.yaml の admin.password）"
  $Password = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
}
$auth = "Basic " + [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("admin:$Password"))

if ($Once) {
  [Console]::OutputEncoding = [Text.Encoding]::UTF8
  $s = Invoke-RestMethod -Uri "$Url/api/status" -Headers @{ Authorization = $auth } -TimeoutSec 2
  $s.PSObject.Properties.Remove('urls')
  $s | ConvertTo-Json -Depth 8
  exit 0
}

if ($Csv -ne "" -and -not (Test-Path $Csv)) {
  Add-Content -Path $Csv -Value "time,id,state,kbps,protocol,rtt_ms,loss_rate,recv_mbps,scene,owner,stack" -Encoding UTF8
}

while ($true) {
  $now = Get-Date -Format "HH:mm:ss.fff"
  try {
    $s = Invoke-RestMethod -Uri "$Url/api/status" -Headers @{ Authorization = $auth } -TimeoutSec 2
    $cams = ($s.cameras | ForEach-Object { "{0}={1}({2:N0})" -f $_.id, $_.state, $_.kbps }) -join " "
    $health = if ($null -eq $s.obs.scene_health) { "-" } else { $s.obs.scene_health }
    $seen = ($s.obs.scene_cameras) -join ","
    $line = "{0} cams[{1}] streaming={2} scene={3} class={4} health={5} scene_cams=[{6}] owner={7} return={8} warnings={9}" -f `
      $now, $cams, $s.obs.streaming, $s.obs.scene, $s.obs.scene_class, $health, $seen, `
      $s.obs.standby.owner, $s.obs.standby.return_scene, @($s.obs.warnings).Count
  } catch {
    $line = "{0} 状態を読めない: {1}" -f $now, $_.Exception.Message
  }
  Write-Output $line
  if ($Log -ne "") { Add-Content -Path $Log -Value $line -Encoding UTF8 }
  if ($Csv -ne "" -and $null -ne $s -and $null -ne $s.cameras) {
    $stack = (@($s.obs.standby.stack) -join "|")
    $rows = $s.cameras | ForEach-Object {
      $rtt = ""; $loss = ""; $recv = ""
      if ($null -ne $_.srt) { $rtt = "{0:F1}" -f $_.srt.rtt_ms; $loss = "{0:F3}" -f $_.srt.loss_rate; $recv = "{0:F3}" -f $_.srt.recv_mbps }
      "{0},{1},{2},{3:F0},{4},{5},{6},{7},{8},{9},{10}" -f $now, $_.id, $_.state, $_.kbps, $_.protocol, $rtt, $loss, $recv, $s.obs.scene, $s.obs.standby.owner, $stack
    }
    Add-Content -Path $Csv -Value $rows -Encoding UTF8
  }
  $s = $null
  Start-Sleep -Milliseconds $IntervalMs
}
