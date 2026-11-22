# B-1〜B-6 の代理の確認（不具合の直しの確かめ。スマホは使わない。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（以下「本体」）の不具合の直しを、ユーザーと一緒に確かめる。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

**守ること（パスワード、プロセス、OBS の操作）は、同じフォルダの `hide-agent-brief.md` の「守ること」のとおりにする。** スマホは使わず、配信もしない。`test setup` もしない。

## 前提

- 本体は**新しい `irl-live-keeper.exe`（2026-11-22 21:10 の版、14,223,360 バイト）**。スクリプト4つ（`show-all.ps1`、`standby.ps1`、`watch-status.ps1`、`watch-obs.ps1`）も新しい版（21:10）。最初に、ユーザーにこれらを入れ替えたことを確かめてもらう
- 直したこと:
  - F-10: スクリプトの既定のパス（`config.yaml`、`ws-password.txt`、`ctl.exe`）を、param の既定値ではなく本文で決めるようにした（前の版の `standby.ps1` は `Join-Path : Cannot bind argument to parameter 'Path' because it is an empty string.` で止まった）
  - MediaMTX の終了の WARN: 本体を Ctrl+C で止めたときや、コンソールの窓を閉じたときに、MediaMTX が本体より一瞬早く終わり、本体のログに `level=WARN msg="MediaMTX が終了しました" ... error="exit status 0xc000013a"` が出ていた。新しい版は、1秒待って本体の終了を見て、`level=INFO msg="MediaMTX が終了しました（本体の終了と同時）"` にする（出ないこともある）
- 本体の起動と停止は、ユーザーが行う。あなたは起動も停止もしない
- 本体のログは `logs\irl-live-keeper-<日付>.log`

## 確かめること

| # | 手順 | 期待 |
|---|---|---|
| B-1 | 本体が動いていて、OBS につながっている状態で、`powershell -ExecutionPolicy Bypass -File .\standby.ps1 on`（`-Config` を付けない） | Join-Path のエラーが出ず、HTTP の応答が出る（配信していないので `失敗 409 …` でよい） |
| B-2 | 同じく `.\standby.ps1 off` | B-1 と同じ（409 でよい） |
| B-3 | `powershell -ExecutionPolicy Bypass -File .\show-all.ps1` | `show-all: 200 {"ok":true}` |
| B-4 | `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Once` | status の JSON（`urls` なし）が出る |
| B-5 | `powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 3`（`ws-password.txt` がなければ、作られることも確かめる） | エラーなく3秒動いて終わる |
| B-6a | ユーザーに、本体の窓で **Ctrl+C** を押して止めてもらう。止まったあと、本体のログの最後の20行を読む | `level=WARN msg="MediaMTX が終了しました"` が出ていない（`level=INFO msg="MediaMTX が終了しました（本体の終了と同時）"` は出てよい）。`Get-Process mediamtx` に本体の MediaMTX が残っていない |
| B-6b | ユーザーに本体を起動し直してもらい、OBS につながったら、**コンソールの窓の × で閉じて**もらう。本体のログの最後の20行を読む | B-6a と同じ |
| B-6c | 比べるため: B-6b のあと、ユーザーに本体を起動し直してもらい、**タスクマネージャーで `mediamtx.exe`（本体が起動した方。8554 を開いている方）だけを終了**してもらう | こちらは本体が動いたままなので、今までどおり `level=WARN msg="MediaMTX が終了しました"` が出て、MediaMTX が起動し直す（本体のログの `MediaMTX` の起動の行）。最後に、ユーザーに本体を Ctrl+C で止めてもらう |

各項目で、出力（パスワードを含まないこと）と、本体のログの該当の `time=` の行を記録する。

## 記録

`bugfix-result.md` に、項目ごとの判定（合格 / 不合格 / できなかった）と理由、出力、本体のログの行を書く（パスワードを書かない）。最後に、ユーザーに `bugfix-result.md` ができたことを伝える。
