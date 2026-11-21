<#
.SYNOPSIS
  irl-live-keeper の「カメラのソースをすべて表示に戻す」（POST /api/cameras/show-all）を押す（M-13 の Windows の確認用）。
.DESCRIPTION
  パスワードは -Password で渡すか、省略すれば同じフォルダの config.yaml の admin.password を読む（表示しない）。
  手で隠したカメラのソースも表示に戻る。OBS につながっていなければ 503。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\show-all.ps1
#>
param(
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
  $r = Invoke-WebRequest -UseBasicParsing -Method Post -Uri "$Url/api/cameras/show-all" -Headers @{ Authorization = $auth; "X-IRL-Live-Keeper" = "1" } -TimeoutSec 5
  "{0} show-all: {1} {2}" -f $now, $r.StatusCode, $r.Content
} catch {
  $code = ""
  if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
  "{0} show-all: 失敗 {1} {2}" -f $now, $code, $_.Exception.Message
}
