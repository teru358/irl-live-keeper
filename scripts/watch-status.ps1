<#
.SYNOPSIS
  窓 B: irl-live-keeper の状態（/api/status）を一定の間隔で1行ずつ表示する（M-9 の Windows の確認用）。
.DESCRIPTION
  パスワードは -Password で渡すか、省略すれば同じフォルダの config.yaml の admin.password を読む。
  -Log を付けると、表示と同じ行をファイルにも書く。Ctrl+C で止める。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-c1.txt
  powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Once   # status の JSON を1回だけ出す（資格情報の入る urls を除く）
#>
param(
  [string]$Password = "",
  [string]$Url = "http://127.0.0.1:8787",
  [int]$IntervalMs = 500,
  [string]$Log = "",
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
  Start-Sleep -Milliseconds $IntervalMs
}
