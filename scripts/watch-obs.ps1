<#
.SYNOPSIS
  窓 C: OBS のイベントと状態を ctl.exe watch で見張り、画面とファイルに残す（M-9 の Windows の確認用）。
.DESCRIPTION
  ctl.exe は、接続先を環境変数 OBS_WS_ADDR、パスワードを OBS_WS_PASSWORD_FILE のファイル（1行）で受け取る。
  パスワードのファイルがなければ、ここで入力して作る（試験が終わったら消す）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Out .\watch-c1.txt
  powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 600 -Out .\watch-t12.txt
#>
param(
  [int]$Seconds = 300,
  [string]$Out = ".\watch.txt",
  [string]$Addr = "127.0.0.1:4455",
  [string]$PasswordFile = (Join-Path $PSScriptRoot "ws-password.txt")
)

if (-not (Test-Path $PasswordFile)) {
  $sec = Read-Host -AsSecureString "OBS の WebSocket のパスワード（ツール → WebSocket サーバー設定）"
  $pw = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
  [IO.File]::WriteAllText($PasswordFile, $pw)
  Write-Output "パスワードを $PasswordFile に書きました（試験が終わったら消してください）"
}

# ctl.exe は UTF-8 で書き出すので、画面とファイルを UTF-8 で読む（Windows PowerShell 5.1 の既定は Shift-JIS で、日本語が化ける）
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$OutputEncoding = [Text.Encoding]::UTF8

$env:OBS_WS_ADDR = $Addr
$env:OBS_WS_PASSWORD_FILE = (Resolve-Path $PasswordFile).Path
$ctl = Join-Path $PSScriptRoot "ctl.exe"
& $ctl watch $Seconds | Tee-Object -FilePath $Out -Encoding utf8
