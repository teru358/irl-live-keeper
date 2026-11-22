<#
.SYNOPSIS
  irl-live-keeper の手動の待機の ON / OFF（POST /api/standby/on、/api/standby/off）を押す（0c の Windows の確認用）。
.DESCRIPTION
  パスワードは -Password で渡すか、省略すれば同じフォルダの config.yaml の admin.password を読む（表示しない）。
  受け付けなければ 409（理由は応答の本文と status の last_error）。OBS につながっていなければ 503。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\standby.ps1 on
#>
param(
  [Parameter(Mandatory = $true, Position = 0)][ValidateSet("on", "off")][string]$Action,
  [string]$Password = "",
  [string]$Url = "http://127.0.0.1:8787",
  [string]$Config = (Join-Path $PSScriptRoot "config.yaml")
)

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
$now = Get-Date -Format "HH:mm:ss.fff"
try {
  $r = Invoke-WebRequest -UseBasicParsing -Method Post -Uri "$Url/api/standby/$Action" -Headers @{ Authorization = $auth; "X-IRL-Live-Keeper" = "1" } -TimeoutSec 5
  "{0} standby {1}: {2} {3}" -f $now, $Action, $r.StatusCode, $r.Content
} catch {
  $code = ""
  if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
  $body = ""
  if ($_.ErrorDetails) { $body = $_.ErrorDetails.Message }
  "{0} standby {1}: 失敗 {2} {3} {4}" -f $now, $Action, $code, $_.Exception.Message, $body
}
