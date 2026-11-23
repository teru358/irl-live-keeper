# 歩く間の記録（T-15 の2日目）。人が見ていなくても、決まった間隔で
# OBS のメディアソースの状態（ctl.exe inputs）とメモリをファイルに足していく。
#
#   powershell -ExecutionPolicy Bypass -File .\walk-log.ps1                 # 既定: 45分、2分ごと
#   powershell -ExecutionPolicy Bypass -File .\walk-log.ps1 -Minutes 60 -EverySec 60
#
# パスワードは表示しない（ctl.exe が ws-password.txt を読む）。
param(
  [int]$Minutes = 45,
  [int]$EverySec = 120,
  [string]$Out = ""
)
$ErrorActionPreference = "Continue"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($Out -eq "") { $Out = Join-Path $here "inputs-t15-walk.txt" }
$env:OBS_WS_ADDR = "127.0.0.1:4455"
$env:OBS_WS_PASSWORD_FILE = Join-Path $here "ws-password.txt"
$ctl = Join-Path $here "ctl.exe"
$end = (Get-Date).AddMinutes($Minutes)
while ((Get-Date) -lt $end) {
  $t = Get-Date -Format "HH:mm:ss"
  Add-Content -Encoding UTF8 -Path $Out -Value "== $t inputs"
  & $ctl inputs 2>&1 | ForEach-Object { "$_" } | Add-Content -Encoding UTF8 -Path $Out
  Add-Content -Encoding UTF8 -Path $Out -Value "== $t items T-A"
  & $ctl items T-A 2>&1 | ForEach-Object { "$_" } | Add-Content -Encoding UTF8 -Path $Out
  Add-Content -Encoding UTF8 -Path $Out -Value "== $t memory"
  Get-Process mediamtx, obs64, irl-live-keeper -ErrorAction SilentlyContinue |
    Select-Object Id, ProcessName, WorkingSet64 | Format-Table -AutoSize | Out-String |
    Add-Content -Encoding UTF8 -Path $Out
  Start-Sleep -Seconds $EverySec
}
Add-Content -Encoding UTF8 -Path $Out -Value ("== {0} end" -f (Get-Date -Format "HH:mm:ss"))
