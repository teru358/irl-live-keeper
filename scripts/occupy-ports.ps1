<#
.SYNOPSIS
  試験用: irl-live-keeper が使うポートを一時的に占有する（ポートの事前確認の試験に使う）。

.DESCRIPTION
  本体と同じ形で待ち受ける。RTMP / RTSP は全インターフェースの TCP、SRT は全インターフェースの UDP、
  MediaMTX の API と管理 API は 127.0.0.1 の TCP。Enter を押すと閉じて終わる。
  全インターフェースで待ち受けるので、Windows のファイアウォールの確認が出ることがある。
  この試験では外から接続しないので、「キャンセル」でよい（同じ PC での占有はそのまま効く）。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1
  powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target rtmp
  powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target srt,admin -Srt 18890
#>
param(
  [ValidateSet('rtmp', 'srt', 'rtsp', 'api', 'admin')]
  [string[]]$Target = @('rtmp', 'srt', 'rtsp', 'api', 'admin'),
  [int]$Rtmp = 1935,
  [int]$Srt = 8890,
  [int]$Rtsp = 8554,
  [int]$Api = 9997,
  [int]$Admin = 8787
)

$ErrorActionPreference = 'Stop'
$specs = @{
  rtmp  = @{ Proto = 'tcp'; Addr = [System.Net.IPAddress]::Any;      Port = $Rtmp;  Name = 'mediamtx.rtmp_port' }
  srt   = @{ Proto = 'udp'; Addr = [System.Net.IPAddress]::Any;      Port = $Srt;   Name = 'mediamtx.srt_port' }
  rtsp  = @{ Proto = 'tcp'; Addr = [System.Net.IPAddress]::Any;      Port = $Rtsp;  Name = 'mediamtx.rtsp_port' }
  api   = @{ Proto = 'tcp'; Addr = [System.Net.IPAddress]::Loopback; Port = $Api;   Name = 'mediamtx.api_port' }
  admin = @{ Proto = 'tcp'; Addr = [System.Net.IPAddress]::Loopback; Port = $Admin; Name = 'admin.listen' }
}

$held = @()
try {
  foreach ($t in $Target) {
    $s = $specs[$t]
    $ep = New-Object System.Net.IPEndPoint($s.Addr, $s.Port)
    try {
      if ($s.Proto -eq 'tcp') {
        $l = New-Object System.Net.Sockets.TcpListener($ep)
        $l.ExclusiveAddressUse = $true
        $l.Start()
        $held += $l
      } else {
        $u = New-Object System.Net.Sockets.UdpClient
        $u.ExclusiveAddressUse = $true
        $u.Client.Bind($ep)
        $held += $u
      }
      Write-Host ("占有した: {0,-20} {1}/{2} ({3})" -f $s.Name, $s.Port, $s.Proto, $s.Addr)
    } catch {
      Write-Host ("占有できない（すでに使われている?）: {0} {1}/{2}: {3}" -f $s.Name, $s.Port, $s.Proto, $_.Exception.Message)
    }
  }
  if ($held.Count -eq 0) { Write-Host '占有したポートはない。終わる。'; exit 1 }
  Write-Host ''
  Write-Host 'この状態で irl-live-keeper.exe を起動して、止まり方を確かめる。'
  Read-Host 'Enter で閉じて終わる' | Out-Null
} finally {
  foreach ($h in $held) {
    if ($h -is [System.Net.Sockets.TcpListener]) { $h.Stop() } else { $h.Close() }
  }
  Write-Host '閉じた。'
}
