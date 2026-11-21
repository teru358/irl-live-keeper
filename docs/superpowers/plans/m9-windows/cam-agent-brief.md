# C-1〜C-7 の代理の確認（スマホ2台。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（OBS の待機シーンへの自動切替の道具。以下「本体」）の、カメラ2台の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。** スマホの操作（送信の開始と停止、回線の切替）はユーザーに頼み、それ以外（OBS の操作、記録、判定）はあなたが行う。

## 前提

- カメラは2台。`main` = Android（RTMP Streamer）、`sub` = iOS（It's My Live）
- 本体は、配信中だけシーンを切り替える。今のシーンに映るカメラが1台でも悪くなると待機シーン（`config.yaml` の `obs.standby_scene`。以下「待機」）へ送り、全部良くなると元のシーンへ戻す。人が切り替えたシーンは「持ち主 none」で、本体は戻さない
- 配信は、試験だけのプロファイルで、PC の中の試験だけの MediaMTX（`127.0.0.1:21935`）へ流す。本物の配信先（Twitch など）には流れない
- シーンは、`ctl.exe test setup` が**今のシーンコレクション（`test用`。ユーザーの手での確認用で、使ってよいと言われている）に足す**: `T-A`（`cam-main` と `cam-sub`）、`T-B`（`cam-main`）、`T-Sub`（`cam-sub`）。待機シーンは、もとからある `⚠回線待機中`（`config.yaml` の `obs.standby_scene`）。`test restore` は、ctl が足したシーンとソース（`test-original.json` に UUID がある）だけを消す
- **シーンコレクションを切り替えない。** OBS のメニューでも切り替えない（前回、WebSocket でシーンコレクションを作ったあと OBS が NotReady のまま戻らず、手で切り替えると OBS が落ちた）
- `test用` には、もとから `Camera`（`IRL`）と `Camera2`（`メディア`）があり、どちらも main を読む。これらには触れない

## 守ること

- **パスワードを、画面への出力、結果のファイル、ユーザーへの文に書かない。** `config.yaml` と `ws-password.txt` にはパスワードが入っている。`config.yaml` を表示しない、書き換えない（`ctl.exe` とスクリプトが読むのはよい）。status は `watch-status.ps1`（1行の形か `-Once`）で読み、`Invoke-RestMethod` の結果をそのまま表示しない（`urls` に資格情報が入る）
- **OBS の設定は、`ctl.exe` の命令でだけ変える**（`test setup|restore`、`scene`、`item`、`hwdecode`、`stream start|stop`）。`item` は試験のシーン（`T-` で始まる）にだけ使う。ユーザーのプロファイルには書かない
- **配信を始める前に、毎回 `.\ctl.exe profile` が `IRLKeeper-Test` を出すことを確かめる。** 違えば配信を始めない（本物の配信先へ公開で流れる）
- **途中で何が起きても、終わるときは必ず「片づけ」（下）をする**
- **止めてよいプロセスは、自分で起動したものだけ（PID で止める）。** `mediamtx.exe` は2つ動く。本体が起動した方（8554 などを開く）は止めない。`Stop-Process -Name` は使わない
- 本体（`irl-live-keeper.exe`）と OBS は、ユーザーが起動する。あなたは起動も停止もしない
- 迷ったら、進めずにユーザーに聞く

## 道具

- `ctl.exe`（OBS の操作）: `scene <名前>`、`current`、`scenes`、`stream start|stop`、`streamstatus`、`profile`、`inputs`（メディアソースのハードウェアデコードと再生の状態）、`item <シーン> <ソース> on|off`、`hwdecode on|off`（ctl が作った `cam-*` だけ）、`test setup|restore`、`watch <秒>`（OBS のイベントを1行ずつ。止めても、それまでの行は残る）。接続先は `OBS_WS_ADDR`、パスワードは `OBS_WS_PASSWORD_FILE` のファイル
- `watch-status.ps1`: 本体の状態を0.5秒ごとに1行で出す（`-Log <ファイル>` でファイルにも）。`-Once` で status の JSON を1回（`urls` を除く）
- `mediamtx.exe` と `dest.yml`: 試験だけの配信先

1行の形の読み方: `cams[main=OK(3,500) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=auto return=T-A warnings=0`。`cams` はカメラごとの状態（OK / LOW / DOWN）と kbps、`scene_cams` は今のシーンに映るカメラ、`health` は今のシーンの健康、`owner` と `return` は待機の持ち主と戻り先。

## 準備

PowerShell で、試験のフォルダから進める。

- あなたのシェルが bash（Git Bash）なら、PowerShell のコマンドは `powershell -NoProfile -Command "..."` で実行する。`ctl.exe` は bash からそのまま実行してよい
- `ctl.exe` の出力をファイルに書くときは、先に `[Console]::OutputEncoding = [Text.Encoding]::UTF8`（日本語と記号の名前が化けないように）

1. ユーザーに確かめてもらう: (a) `config.yaml` の `cameras:` に2台目 `sub` を足し、本体を起動し直した（起動の表示に `sub` の URL が出る）。(b) It's My Live の送信先に、起動の表示の `sub` の送信用 URL を入れた。(c) OBS は起動していて、配信していない。(d) スマホは2台とも送っていない
2. `ws-password.txt` があるか確かめる（中身は表示しない）。なければユーザーに `powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 1` を一度動かしてもらう
3. 環境変数: `$env:OBS_WS_ADDR = "127.0.0.1:4455"`、`$env:OBS_WS_PASSWORD_FILE = (Resolve-Path .\ws-password.txt).Path`（コマンドごとに新しいシェルになるなら、毎回つける）
4. `test-original.json` がすでにあれば、前の試験が戻っていない。進めずにユーザーに知らせる
5. 試験だけの配信先を裏で起動し（`.\mediamtx.exe .\dest.yml`）、PID を控える。`Get-NetTCPConnection -LocalPort 21935 -State Listen` で確かめる
6. `.\ctl.exe scenes` の1行目が `test用` であることを確かめる（違えば進めずにユーザーに知らせる）。`.\ctl.exe test setup`。出力の「元のプロファイル」「シーンコレクション」「シーン」を控える。`.\ctl.exe scenes` に `T-A`、`T-B`、`T-Sub` が出ること。失敗したら、出力をそのままユーザーに見せ、`test-original.json` があれば `test restore` を試す
7. 見張りを裏で始める: `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-cam.txt` と `.\ctl.exe watch 3600 .\obs-cam.txt`（ファイルを引数で渡す。`>` で書くと、止めたときに最後が欠ける）
8. ユーザーに、2台とも送信を始めてもらう。`status-cam.txt` で `main` と `sub` がどちらも OK になるのを待つ（最大60秒）。`.\ctl.exe inputs` で2つのソースが `PLAYING` で cursor が進むこと（2回読んで比べる）。ならなければ、ユーザーに OBS の画面を見てもらう
9. `.\ctl.exe profile` が `IRLKeeper-Test` を出すことを確かめてから `.\ctl.exe scene T-B` → `.\ctl.exe stream start`。`streaming=True scene=T-B health=OK owner=none` になること

各項目の前に、2台が OK で、今のシーンと owner が項目の始まりの形になっていることを確かめる。「待つ」は、`status-cam.txt` の行を見て待つ（最大の秒数を過ぎたら、その時点の様子を記録して次へ）。

## 確かめること

| # | 手順 | 期待 |
|---|---|---|
| C-7 | T-B で2台が OK のとき、`watch-status.ps1 -Once` の JSON から、`cameras`（2台。`id`、`name`、`on_bad`、`state`、`kbps`、`online`、`protocol`）、`obs.scene`、`obs.scene_class`、`obs.scene_cameras`、`obs.scene_health` を記録する。`ctl.exe scene T-A` のあとにもう一度 | `cameras` に2台。T-B では `scene_cameras` が `[main]`、T-A では `[main, sub]`。`scene_health` は OK |
| C-1 | `scene T-A`（2台 OK）。ユーザーに **sub（iOS）の送信を止めて**もらう → 最大15秒待つ → ユーザーに sub を送り直してもらう → 最大40秒待つ | 止めて約2秒で待機へ（owner `auto`、return `T-A`）。sub が OK になって約1秒で T-A へ（owner `none`） |
| C-2 | `scene T-B`（2台 OK）。ユーザーに sub を止めてもらう → 15秒見る → sub を送り直してもらい、OK に戻るのを待つ | T-B のまま、何も起きない（T-B に sub は映らない）。`health` は OK のまま |
| C-3 | 使わない日のカメラの運用。`scene T-B`、ユーザーに sub を止めてもらう（main は送ったまま）→ `scene T-Sub` → sub を送り直してもらう → 最大40秒待つ。続けて `scene T-B` → sub を止めてもらう → 15秒見る | T-Sub にした直後に待機へ（return `T-Sub`）。sub が OK になると T-Sub へ戻る。T-B に戻してから sub を止めても何も起きない |
| C-4 | `scene T-A`（2台 OK）→ `item T-A cam-sub off` → ユーザーに sub を止めてもらう → 15秒見る → `item T-A cam-sub on` → 最大10秒見る → sub を送り直してもらい、戻るのを待つ | 隠している間は T-A のまま（`scene_cams=[main]`）。表示に戻すと、sub が悪いので待機へ（return `T-A`）。sub が OK になると T-A へ |
| C-5 | `scene T-A`。(1) 2台を同じ回線（同じ Wi-Fi）から60秒送り、`status-cam.txt` の2台の kbps の最小・最大・平均と、LOW / DOWN になった回数を記録する。(2) ユーザーに、1台を別の回線（モバイル回線など）から送れるか聞き、送れるなら同じく60秒。送れないなら「できなかった」と書く | 記録だけ（判定はしない）。2台が同じ回線のときに、片方が落ちるともう片方が上がる、などの様子があれば書く |
| C-6 | `scene T-A`（2台 OK）。`ctl.exe inputs` で `hw_decode=true` を確かめ、60秒の CPU と GPU のデコードの使い方を測る（下）→ `ctl.exe hwdecode off` → 10秒待って同じく60秒測る → `ctl.exe hwdecode on`。それぞれの60秒の間に、本体のログ（`logs\` の今日のファイル）に `reader is too slow` が出たかも数える | 記録だけ。ハードウェアデコードの有無で、CPU と GPU の使い方がどう違うか。`reader is too slow` の数。注意: `test用` のもとからある `IRL` と `メディア` も main を読んでデコードしている（`hwdecode` はこの2つを変えない）。`ctl.exe inputs` で4つのソースの `hw_decode` を記録に書く |

C-6 の測り方（PowerShell）:

```powershell
$cpu = Get-Counter '\Processor(_Total)\% Processor Time' -SampleInterval 2 -MaxSamples 30
"CPU 平均 {0:N1}%" -f ($cpu.CounterSamples.CookedValue | Measure-Object -Average).Average
$obs = Get-Process obs64; $t0 = $obs.CPU; Start-Sleep 30; $obs.Refresh(); "obs64 の CPU 時間 30秒あたり {0:N1} 秒" -f ($obs.CPU - $t0)
$gpu = Get-Counter '\GPU Engine(*engtype_VideoDecode)\Utilization Percentage' -SampleInterval 2 -MaxSamples 15 -ErrorAction SilentlyContinue
"GPU デコード 合計の平均 {0:N1}%" -f (($gpu | ForEach-Object { ($_.CounterSamples.CookedValue | Measure-Object -Sum).Sum }) | Measure-Object -Average).Average
```

（カウンタの名前が日本語の Windows で見つからないときは、`Get-Counter -ListSet *` で探すか、タスクマネージャーの値をユーザーに読んでもらう。）

## 片づけ（失敗したときも必ず）

1. `.\ctl.exe stream stop` → 3秒待つ
2. `.\ctl.exe test restore`。「試験のシーンとソースを消し、元のプロファイル … とシーン … に戻した」が出て、準備の 6 で控えた名前と同じこと。`.\ctl.exe profile` と `.\ctl.exe scenes` でも確かめる（`T-` のシーンが消えている）。restore が失敗したら、出力をユーザーに見せ、OBS の「プロファイル」メニューで元のプロファイルに戻し、残った `T-` のシーンと `cam-*`・`label-T-*` のソースを手で消してもらう（シーンコレクションは切り替えない）
3. 試験だけの MediaMTX を PID で止める。裏の見張り2つを止める
4. ユーザーに、スマホ2台の送信を止めてよいこと、`config.yaml` の `sub` はそのままでよいこと（次の試験でも使う）を伝える

## 記録

`cam-result.md` に書く（パスワードを書かない）:

- 項目ごとに、判定（合格 / 不合格 / できなかった / 記録のみ）と1〜2行の理由
- 項目ごとの時刻つきの流れ（ユーザーに頼んだ時刻、状態が変わった時刻、シーンと owner と return）と、`status-cam.txt` の要所の数行
- 本体のログ（`logs\` の今日のファイルの、その時刻の `time=` の行。MediaMTX の接続の行は省いてよい）
- C-5 と C-6 の数値
- 気になった動き、指示書と違ったこと
- 片づけの結果（元のプロファイルとシーンコレクションの名前、止めたプロセスの PID）

最後に、ユーザーに `cam-result.md` ができたことを伝える。
