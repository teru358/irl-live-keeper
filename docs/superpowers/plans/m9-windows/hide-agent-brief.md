# H-1〜H-8 の代理の確認（隠す・戻す。スマホ2台。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（OBS の待機シーンへの自動切替の道具。以下「本体」）の、カメラのソースを隠す・戻す動き（0b）の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。** スマホの操作（送信の開始と停止、回線の切替）はユーザーに頼み、それ以外（OBS の操作、記録、判定）はあなたが行う。

## 前提

- カメラは2台。`main` = Android（RTMP Streamer。`on_bad: standby`）、`sub` = iOS（It's My Live。**`on_bad: hide`**。Tailscale でモバイル回線から送ることがある）。Android の無料の版は約5分で送信が止まるので、長い項目は止まったら送り直してもらう
- **隠す・戻す**（`on_bad: hide` のカメラ）: そのカメラが悪くなると、本体は、そのカメラのメディアソースの項目を**全部のシーンで**非表示にする（シーンは変えない）。良くなると表示に戻す。今のシーンのカメラが全部隠れてしまうときは、隠さずに待機へ送る。隠した項目は `state\hidden.json` に記録し、本体を止めるとき（Ctrl+C、コンソールを閉じる）は2秒以内に表示に戻してから止まる。落ちたときは、次の起動で記録から照合して戻す
- 本体は、配信中だけシーンを切り替える。今のシーンに映るカメラが1台でも悪くなると待機シーン（`config.yaml` の `obs.standby_scene`。以下「待機」）へ送り、全部良くなると元のシーンへ戻す。人が切り替えたシーンは「持ち主 none」で、本体は戻さない
- 配信は、試験だけのプロファイルで、PC の中の試験だけの MediaMTX（`127.0.0.1:21935`）へ流す。本物の配信先（Twitch など）には流れない
- シーンは、`ctl.exe test setup` が**今のシーンコレクション（`test用`。ユーザーの手での確認用で、使ってよいと言われている）に足す**: `T-A`（`cam-main` と `cam-sub`）、`T-B`（`cam-main`）、`T-Sub`（`cam-sub`）。待機シーンは、もとからある `⚠回線待機中`（`config.yaml` の `obs.standby_scene`）。`test restore` は、ctl が足したシーンとソース（`test-original.json` に UUID がある）だけを消す
- **シーンコレクションを切り替えない。** OBS のメニューでも切り替えない（前回、WebSocket でシーンコレクションを作ったあと OBS が NotReady のまま戻らず、手で切り替えると OBS が落ちた）
- `test用` には、もとから `Camera`（`IRL`）と `Camera2`（`メディア`）があり、どちらも main を読む。これらには触れない

## 守ること

- **パスワードを、画面への出力、結果のファイル、ユーザーへの文に書かない。** `config.yaml` と `ws-password.txt` にはパスワードが入っている。`config.yaml` を表示しない、書き換えない（`ctl.exe` とスクリプトが読むのはよい）。status は `watch-status.ps1`（1行の形か `-Once`）で読み、`Invoke-RestMethod` の結果をそのまま表示しない（`urls` に資格情報が入る）
- **OBS の設定は、`ctl.exe` の命令でだけ変える**（`test setup|restore`、`scene`、`item`、`hwdecode`、`stream start|stop`、`studio on|off`）。`item` は試験のシーン（`T-` で始まる）にだけ使う。ユーザーのプロファイルには書かない
- **配信を始める前に、毎回 `.\ctl.exe profile` が `IRLKeeper-Test` を出すことを確かめる。** 違えば配信を始めない（本物の配信先へ公開で流れる）
- **途中で何が起きても、終わるときは必ず「片づけ」（下）をする**
- **止めてよいプロセスは、自分で起動したものだけ（PID で止める）。** `mediamtx.exe` は2つ動く。本体が起動した方（8554 などを開く）は止めない。`Stop-Process -Name` は使わない
- 本体（`irl-live-keeper.exe`）と OBS は、ユーザーが起動する。あなたは起動も停止もしない
- 迷ったら、進めずにユーザーに聞く

## 道具

- `ctl.exe`（OBS の操作）: `scene <名前>`、`current`、`scenes`、`stream start|stop`、`streamstatus`、`profile`、`inputs`（メディアソースのハードウェアデコードと再生の状態）、`item <シーン> <ソース> on|off`、`hwdecode on|off`（ctl が作った `cam-*` だけ）、`items <シーン>`（項目の表示の状態）、`studio on|off|status`（Studio Mode）、`test setup|restore`、`watch <秒>`（OBS のイベントを1行ずつ。止めても、それまでの行は残る）。接続先は `OBS_WS_ADDR`、パスワードは `OBS_WS_PASSWORD_FILE` のファイル
- `watch-status.ps1`: 本体の状態を0.5秒ごとに1行で出す（`-Log <ファイル>` でファイルにも）。`-Once` で status の JSON を1回（`urls` を除く）
- `show-all.ps1`: 「カメラのソースをすべて表示に戻す」（`POST /api/cameras/show-all`）を押す。パスワードは `config.yaml` から読み、表示しない
- `mediamtx.exe` と `dest.yml`: 試験だけの配信先
- 本体の状態の JSON（`watch-status.ps1 -Once`）の `cameras[].hidden`（隠している項目の数）、`cameras[].user_shown`（人が戻した印）、`obs.studio_mode`、`warnings`

1行の形の読み方: `cams[main=OK(3,500) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=auto return=T-A warnings=0`。`cams` はカメラごとの状態（OK / LOW / DOWN）と kbps、`scene_cams` は今のシーンに映るカメラ、`health` は今のシーンの健康、`owner` と `return` は待機の持ち主と戻り先。

## 準備

PowerShell で、試験のフォルダから進める。

- あなたのシェルが bash（Git Bash）なら、PowerShell のコマンドは `powershell -NoProfile -Command "..."` で実行する。`ctl.exe` は bash からそのまま実行してよい
- `ctl.exe` の出力をファイルに書くときは、先に `[Console]::OutputEncoding = [Text.Encoding]::UTF8`（日本語と記号の名前が化けないように）

1. ユーザーに確かめてもらう: (a) `config.yaml` の `sub` の `on_bad` を **`hide`** にし（`main` は `standby` のまま）、`obs.studio_mode_hide` は書かない（既定の `standby`）で、**新しい `irl-live-keeper.exe`** で本体を起動し直した。(b) It's My Live の送信先に、起動の表示の `sub` の送信用 URL を入れた。(c) OBS は起動していて、配信していない。(d) スマホは2台とも送っていない
2. `ws-password.txt` があるか確かめる（中身は表示しない）。なければユーザーに `powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 1` を一度動かしてもらう
3. 環境変数: `$env:OBS_WS_ADDR = "127.0.0.1:4455"`、`$env:OBS_WS_PASSWORD_FILE = (Resolve-Path .\ws-password.txt).Path`（コマンドごとに新しいシェルになるなら、毎回つける）
4. `test-original.json` がすでにあれば、前の試験が戻っていない。進めずにユーザーに知らせる
5. 試験だけの配信先を裏で起動し（`.\mediamtx.exe .\dest.yml`）、PID を控える。`Get-NetTCPConnection -LocalPort 21935 -State Listen` で確かめる
6. `.\ctl.exe scenes` の1行目が `test用` であることを確かめる（違えば進めずにユーザーに知らせる）。`.\ctl.exe test setup`。出力の「元のプロファイル」「シーンコレクション」「シーン」を控える。`.\ctl.exe scenes` に `T-A`、`T-B`、`T-Sub` が出ること。失敗したら、出力をそのままユーザーに見せ、`test-original.json` があれば `test restore` を試す
7. 見張りを裏で始める: `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-hide.txt` と `.\ctl.exe watch 3600 .\obs-hide.txt`（ファイルを引数で渡す。`>` で書くと、止めたときに最後が欠ける）
8. ユーザーに、2台とも送信を始めてもらう。`status-hide.txt` で `main` と `sub` がどちらも OK になるのを待つ（最大60秒）。`.\ctl.exe inputs` で2つのソースが `PLAYING` で cursor が進むこと（2回読んで比べる）。ならなければ、ユーザーに OBS の画面を見てもらう
9. `.\ctl.exe profile` が `IRLKeeper-Test` を出すことを確かめてから `.\ctl.exe scene T-B` → `.\ctl.exe stream start`。`streaming=True scene=T-B health=OK owner=none` になること

各項目の前に、2台が OK で、今のシーンと owner が項目の始まりの形になっていることを確かめる。「待つ」は、`status-hide.txt` の行を見て待つ（最大の秒数を過ぎたら、その時点の様子を記録して次へ）。

## 確かめること

項目のあとに毎回、`.\ctl.exe items T-A`、`.\ctl.exe items T-Sub` で項目の表示の状態を記録する（`cam-sub enabled=...`）。本体のログ（`logs\` の今日のファイル）の `camera hidden` / `camera shown` の行も記録する。

| # | 手順 | 期待 |
|---|---|---|
| H-1 | 隠す・戻す: `scene T-A`（2台 OK）。ユーザーに **sub の送信を止めて**もらう → 最大15秒待つ → ユーザーに sub を送り直してもらう → 最大40秒待つ | 止めて数秒で `camera hidden camera=sub`。**T-A のまま**（待機へ行かない）。`T-A` と `T-Sub` の `cam-sub` が `enabled=False`、status の `cameras[sub].hidden` が 2 前後、`state\hidden.json` に記録がある（中身はパスワードを含まないので、件数と項目の名前を記録してよい）。sub が OK になると `camera shown camera=sub reason=ok`、項目が表示に戻り、記録が消える。ユーザーに、OBS の画面で sub の項目が消えて戻ったかを見てもらう |
| H-2 | 全部隠れるシーン: `scene T-Sub`（2台 OK）。ユーザーに sub を止めてもらう → 最大15秒 → 送り直してもらう → 最大40秒 | T-Sub には sub しかいないので、隠さずに**待機へ**（owner `auto`、return `T-Sub`）。そのあと待機の間に sub の項目が隠れる。sub が OK になると、項目が表示に戻ってから T-Sub へ戻る（ログの順で確かめる） |
| H-3 | 人が戻す: `scene T-A`。sub を止めてもらい、隠れたら `item T-A cam-sub on`（人の操作の代わり）→ 20秒見る → sub を送り直してもらう | 人が戻した T-A の項目を、同じ不調の間は隠し直さない（`user_shown=true`）。sub が OK になると印が消える |
| H-4 | すべて表示に戻す: `scene T-A`。sub を止めてもらい、隠れたら `powershell -ExecutionPolicy Bypass -File .\show-all.ps1` | 応答 200。`T-A` と `T-Sub` の `cam-sub` が表示に戻る（`reason=manual_all`）。そのあと sub が悪いままなら、5秒ほどでまた隠す（ふだんの規則に戻る）。最後に sub を送り直してもらう |
| H-5 | 配信の停止: `scene T-A`。sub を止めてもらい、隠れたら `.\ctl.exe stream stop` | `reason=stream_stop` で表示に戻る。確かめたら、`profile` が `IRLKeeper-Test` であることを確かめてから `stream start` し直し、sub を送り直してもらう |
| H-6 | 本体を Ctrl+C で止める: `scene T-A`。sub を止めてもらい、隠れたら、**ユーザーに本体の窓で Ctrl+C を押してもらう** | 2秒以内に `reason=shutdown` で表示に戻ってから止まる。`hidden.json` の記録が空。ユーザーに本体を起動し直してもらい、つながるのを待つ |
| H-7 | 本体が落ちたとき: `scene T-A`。sub を止めてもらい、隠れたら、**ユーザーにタスクマネージャーの「詳細」で `irl-live-keeper.exe` を「タスクの終了」してもらう**（あなたは止めない）→ 項目は隠れたまま残る（`items` で確かめる）→ sub を送り直してもらう → ユーザーに本体を起動し直してもらう | 起動し直した本体が `hidden.json` を照合し、sub が OK になると表示に戻す（`reason=ok`）。`Get-Process mediamtx` で、落ちた本体の MediaMTX が残っていないことも確かめる（Job Object） |
| H-8 | Studio Mode（既定の `standby`）: `scene T-A`。`.\ctl.exe studio on` → status の `warnings` に「Studio Mode のため、隠す動きの代わりに待機シーンへ送っています」→ sub を止めてもらう → 最大15秒 → 送り直してもらう → 最大40秒 → `.\ctl.exe studio off` | Studio Mode の間は、sub を隠さずに**待機へ**送る（return `T-A`）。sub が OK になると T-A へ戻る。`studio off` のあと、`warnings` からその文が消える |

## 片づけ（失敗したときも必ず）

1. `.\ctl.exe studio off`（Studio Mode をオフに戻す）→ `.\ctl.exe stream stop` → 3秒待つ
2. `.\ctl.exe test restore`。「試験のシーンとソースを消し、元のプロファイル … とシーン … に戻した」が出て、準備の 6 で控えた名前と同じこと。`.\ctl.exe profile` と `.\ctl.exe scenes` でも確かめる（`T-` のシーンが消えている）。restore が失敗したら、出力をユーザーに見せ、OBS の「プロファイル」メニューで元のプロファイルに戻し、残った `T-` のシーンと `cam-*`・`label-T-*` のソースを手で消してもらう（シーンコレクションは切り替えない）
3. 試験だけの MediaMTX を PID で止める。裏の見張り2つを止める
4. ユーザーに、スマホ2台の送信を止めてよいこと、`config.yaml` の `sub` の `on_bad` を `standby` に戻すかどうかは、ユーザーが決めることを伝える

## 記録

`hide-result.md` に書く（パスワードを書かない）:

- 項目ごとに、判定（合格 / 不合格 / できなかった / 記録のみ）と1〜2行の理由
- 項目ごとの時刻つきの流れ（ユーザーに頼んだ時刻、状態が変わった時刻、シーンと owner と return）と、`status-hide.txt` の要所の数行
- 本体のログ（`logs\` の今日のファイルの、その時刻の `time=` の行。MediaMTX の接続の行は省いてよい）
- `hidden.json` の件数の変化（各項目のあと）
- 気になった動き、指示書と違ったこと
- 片づけの結果（元のプロファイルとシーンコレクションの名前、止めたプロセスの PID）

最後に、ユーザーに `hide-result.md` ができたことを伝える。
