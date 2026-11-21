# T-12 の代理の確認（Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（OBS の待機シーンへの自動切替の道具。以下「本体」）の Windows での確認を、ユーザーの代わりに進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

## 確かめること（T-12）

配信中に、OBS から配信先への接続が切れて OBS が再接続しても、本体の「待機の持ち主（owner）」と「戻り先（return）」が保たれること。

- 前提: スマホ（カメラ）が送っていないと、配信を始めた直後に本体が待機シーン `⚠回線待機中` へ切り替える（owner `auto`、return `Camera`）
- 配信先の代わりに、この PC の中に試験だけの MediaMTX（`dest.yml`、`127.0.0.1:21935` だけを開く）を立て、それを止めて再接続を起こす
- 期待: 再接続の間も、status の `streaming=True`、owner `auto`、return `Camera` のまま。再接続のあと、カメラが送り始めると `Camera` へ戻り、owner `none` になる
- 参考: Linux では合格している（配信先を止めて約11秒後に OBS が再接続し、その間も持ち主と戻り先が保たれた）

## 守ること

- **パスワードを、画面への出力、結果のファイル、ユーザーへの文に書かない。** `config.yaml` と `ws-password.txt` にはパスワードが入っている。`config.yaml` を表示しない（スクリプトが読むのはよい）。`config.yaml` を書き換えない
- **OBS の設定は、`ctl.exe t12 setup` と `ctl.exe t12 restore` でだけ変える。** setup は、今のプロファイルの名前を `t12-original-profile.txt` に残してから、試験だけのプロファイル `IRLKeeper-T12` を作って切り替え、その中の配信先を `rtmp://127.0.0.1:21935/test` にする（元のプロファイルには書かない）。restore は、元のプロファイルに戻して `IRLKeeper-T12` を消す
- **途中で何が起きても、終わるときは必ず `.\ctl.exe stream stop` → `.\ctl.exe t12 restore` → `.\ctl.exe profile` で元のプロファイルの名前が出ることを確かめる。** restore が失敗したら、その出力をユーザーに見せ、OBS の「プロファイル」メニューで元のプロファイル（`t12-original-profile.txt` の名前）に戻してもらう
- **止めてよいプロセスは、自分で起動したものだけ（PID で止める）。** `mediamtx.exe` は2つ動く。本体が起動した MediaMTX（8554 などを開く）は止めない。止めるのは、自分が `dest.yml` で起動した方だけ。`Stop-Process -Name` は使わない
- 本体（`irl-live-keeper.exe`）と OBS は、ユーザーが起動する。あなたは起動も停止もしない
- ユーザーの手が要るのは、スマホの送信を始めるところだけ（手順 7）。それ以外で迷ったら、進めずにユーザーに聞く

## 道具

- `ctl.exe`: OBS を操作する（`scene <名前>`、`stream start|stop`、`streamstatus`〔`outputActive` と `outputReconnecting`〕、`profile`、`t12 setup|restore`、`watch <秒>`〔OBS のイベント〕）。接続先は環境変数 `OBS_WS_ADDR`、パスワードは `OBS_WS_PASSWORD_FILE` のファイル
- `watch-status.ps1`: 本体の状態（`/api/status`）を0.5秒ごとに1行で出す（`-Log <ファイル>` でファイルにも書く）。パスワードは `config.yaml` から読み、表示しない
- `mediamtx.exe` と `dest.yml`: 試験だけの配信先

## 手順

PowerShell で、試験のフォルダから進める。各手順で、確かめた値と時刻を控える。

- あなたのシェルが bash（Git Bash）なら、PowerShell のコマンドは `powershell -NoProfile -Command "..."` で実行する。`ctl.exe` は bash からそのまま実行してよい（環境変数は `export`。パスワードのファイルは Windows の形のパスでも、bash の形のパスでもよい）
- シーンの名前に日本語と記号（`⚠回線待機中`）がある。PowerShell で `ctl.exe` の出力をファイルに書くときは、先に `[Console]::OutputEncoding = [Text.Encoding]::UTF8` を実行する（しないと文字が化ける）

0. **準備**
   - ユーザーに、OBS と本体が起動していること、スマホが**送っていない**こと、OBS が配信して**いない**ことを確かめてもらう
   - `ws-password.txt` があるか確かめる（中身は表示しない）。なければ、ユーザーに `powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 1` を一度動かしてもらう（パスワードを聞かれて、このファイルができる）
   - 環境変数: `$env:OBS_WS_ADDR = "127.0.0.1:4455"`、`$env:OBS_WS_PASSWORD_FILE = (Resolve-Path .\ws-password.txt).Path`（コマンドごとに新しいシェルになるなら、毎回つける）
   - `.\ctl.exe profile` で今のプロファイルの名前を控える。`t12-original-profile.txt` がすでにあれば、前の試験が戻っていない。進めずにユーザーに知らせる
1. **見張りを始める**（どちらも裏で動かす）
   - `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-t12.txt`
   - `.\ctl.exe watch 900 > .\obs-t12.txt`
2. **配信先を起動する**: `.\mediamtx.exe .\dest.yml` を裏で起動し、PID を控える。`Get-NetTCPConnection -LocalPort 21935 -State Listen` で待ち受けを確かめる
3. **試験のプロファイルに切り替える**: `.\ctl.exe t12 setup`。`.\ctl.exe profile` が `IRLKeeper-T12` を出すこと
4. **配信を始める**: **直前にもう一度 `.\ctl.exe profile` を実行し、`IRLKeeper-T12` でなければ配信を始めない**（元のプロファイルのまま始めると、本物の配信先〔Twitch など〕へ公開で流れる）。`IRLKeeper-T12` なら `.\ctl.exe scene Camera` → `.\ctl.exe stream start`。5秒待って、`status-t12.txt` の最後の行が `streaming=True scene=⚠回線待機中 … owner=auto return=Camera` であること
5. **配信先を止める**: 控えた PID で `Stop-Process -Id <PID>`。そのあと20秒、2秒ごとに `.\ctl.exe streamstatus` を実行して、`outputReconnecting true` になることを見る。その間の `status-t12.txt` で、`streaming`、`owner`、`return` が変わらないかを見る
6. **配信先を起動し直す**: もう一度 `.\mediamtx.exe .\dest.yml` を裏で起動し、新しい PID を控える。最大30秒、2秒ごとに `streamstatus` を見て、`outputReconnecting false` に戻ることを確かめる。そのあとも `owner=auto return=Camera` のままであること
7. **カメラで戻す**: ユーザーに「スマホ（main のカメラ）の送信を始めてください」と頼む。最大60秒、`status-t12.txt` で `scene=Camera … owner=none` になるのを待つ
8. **片づける**（失敗したときも必ず）: `.\ctl.exe stream stop` → 3秒待つ → `.\ctl.exe t12 restore` → `.\ctl.exe profile` が手順 0 で控えた名前を出すこと → 試験だけの MediaMTX を PID で止める → 裏の見張り2つを止める。`t12-original-profile.txt` が消えていること。ユーザーに、スマホの送信を止めてよいと伝える

## 記録

`t12-result.md` に書く（パスワードを書かない）:

- 判定（合格 / 不合格 / できなかった）と、その理由を1〜2行
- 時刻つきの流れ: 配信の開始、待機に入った時刻、配信先を止めた時刻、`outputReconnecting` が true になった時刻と false に戻った時刻、その間の `streaming`・`owner`・`return`、カメラで戻った時刻
- `status-t12.txt` の、配信先を止める前後と再接続の前後の数行ずつ。`obs-t12.txt` の `StreamStateChanged` の行
- 本体の窓に出たログの行（ユーザーに貼ってもらうか、`logs\` の今日のファイルの該当の時刻の行。`time=` で始まる行だけ）
- 気になった動き、指示書と違ったこと
- 片づけの結果（元のプロファイルの名前に戻ったこと、止めたプロセスの PID）

最後に、ユーザーに `t12-result.md` の中身を見せる（ユーザーが、それを別のセッションに貼る）。
