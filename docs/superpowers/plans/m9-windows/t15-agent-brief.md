# T-15 の代理の確認（キャリア回線からの揺れと長い時間。iPhone 1台。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（以下「本体」）の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

**前提・守ること・道具・準備・片づけは、同じフォルダの `hide-agent-brief.md` のとおりにする。** 違うのは次の点だけ:

- 使うのは **iPhone 1台（カメラ `sub`）だけ**。Android（`main`）は送らない（`main` は DOWN のままでよい）
- 送り方は2つ。どちらも **キャリアのモバイル回線から Tailscale を通して** `sub` へ送る（Wi-Fi を切り、URL のホストだけを Windows の Tailscale のアドレス `100.…` にする。`config.yaml` の `stream.publish_host` は変えない）
  - **前半: Larix Broadcaster の SRT**（URL の欄に `srt://100.…:8890`、streamid の欄に `publish:sub:<ユーザー>:<パスワード>`。ユーザーが設定済み）
  - **後半: It's My Live の RTMP**
- 本体は今の `irl-live-keeper.exe`（2026-11-22 21:21 の版、14,225,920 バイト）。`watch-status.ps1` は新しい版（21:3x。`-Csv` が増えた）。準備 1 で、ユーザーに入れ替えたことを確かめてもらう
- 記録のファイルは `t15-result.md`、見張りのファイルは `status-t15-srt.txt` / `t15-srt.csv`（前半）と `status-t15-rtmp.txt` / `t15-rtmp.csv`（後半）、OBS のイベントは `obs-t15.txt`
- **見張り**: `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-t15-srt.txt -Csv .\t15-srt.csv -IntervalMs 1000`（後半はファイル名を rtmp に替えて起動し直す）。CSV の列は `time,id,state,kbps,protocol,rtt_ms,loss_rate,recv_mbps,scene,owner,stack`（`loss_rate` は %）

## 目的

屋外に近い回線（キャリアのモバイル回線）で送ったときに、次を確かめる。

- kbps の揺れ方
- 揺れのせいで、まちがった切替（待機へ送る・戻す）が起きないか
- SRT の RTT とロス率が status に出るか
- 長い時間送っても問題が出ないか（MediaMTX の警告、OBS のメディアソースの止まり、メモリ）

既定の閾値は、`fail_kbps` 800（これ未満が5秒で悪い）、`recover_kbps` 1500（これ以上が10秒で戻る）。

## 準備（`hide-agent-brief.md` の準備に加えて）

1. `test setup` のあと、`.\ctl.exe scene T-Sub` で、サブだけのシーンにする（このシーンでは、sub が悪くなると待機へ送る）
2. `.\ctl.exe profile` が `IRLKeeper-Test` であることを確かめてから `.\ctl.exe stream start`（試験の配信先へ。本物の配信先には流れない）
3. 本体の MediaMTX と OBS のメモリを控える: `Get-Process mediamtx, obs64, irl-live-keeper | Select-Object Id, ProcessName, WorkingSet64`（前半の始まりと、前半・後半の終わり）

## 確かめること

| # | 手順 | 記録すること |
|---|---|---|
| T15-1 | 前半（SRT、20分）: ユーザーに Larix で送り始めてもらう。最初の10分はスマホを置いたまま、次の10分はスマホを持って歩いてもらう（建物の中を移動するなど、無理のない範囲で。ユーザーに始まりと終わりの時刻を言ってもらう） | 本体のログの `[SRT] [conn 100.…] opened`、`is publishing to path 'sub'`（tracks の数）、`camera changed`、`scene switch`。CSV から、置いたまま・歩いたときそれぞれの kbps の最小・平均・最大、LOW と DOWN の回数と長さ、待機への切替の回数、RTT とロス率の最小・平均・最大 |
| T15-2 | 後半（RTMP、20分）: ユーザーに Larix を止め、It's My Live の RTMP で送ってもらう。T15-1 と同じく、置いたまま10分、歩いて10分 | T15-1 と同じ（RTT とロス率は出ない） |
| T15-3 | まちがった切替の判断 | 待機への切替が起きたとき、その直前の kbps と、スマホの画面（ユーザーに聞く）や回線の様子を並べ、本当に映像が乱れていたのか（正しい切替）、乱れていないのに切り替えたのか（まちがった切替）を書く |
| T15-4 | 長い時間の様子 | 本体のログの MediaMTX の WARN（`reader is too slow` など）の数、本体の `media source refreshed` の数、OBS のメディアソースが止まったか（`.\ctl.exe inputs` を5分ごと）、メモリの変化 |

CSV の集計は PowerShell で行ってよい（例: `Import-Csv .\t15-srt.csv | Where-Object id -eq 'sub' | Measure-Object kbps -Minimum -Average -Maximum`）。集計に使ったコマンドも記録に書く。

## 片づけ

`hide-agent-brief.md` の片づけのとおり（`stream stop`、`test restore`、試験の MediaMTX を PID で止める、見張りを止める）。ユーザーに、iPhone の送信を止めてよいことを伝える。

## 記録

`t15-result.md` に、項目ごとの数字と、`hide-agent-brief.md` の「記録」と同じ形の流れを書く（パスワードを書かない）。最後に、次の2つについての意見を書く: (a) 既定の閾値（800 / 1500）は、このキャリア回線に合っているか、(b) SRT と RTMP で、揺れ方や切替の起き方に差があったか。ユーザーに `t15-result.md` ができたことを伝える。
