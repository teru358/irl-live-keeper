# C-1〜C-7 結果（Windows、スマホ2台、2026-11-21）

## 判定: できなかった（準備 6 で止まった）

`ctl.exe test setup` が途中で失敗した。試験のプロファイルとシーンコレクション `IRLKeeper-Test` は作られたが、そのあと OBS の WebSocket がすべての要求に `NotReady (207): OBS is not ready to perform the request.` を返し、約2分半戻らなかった。そのため C-1〜C-7 はどれも始めていない。配信も一度も始めていない（本物の配信先へは何も流れていない）。

項目ごとの判定:

- C-7: できなかった（準備 6 で止まった）
- C-1: できなかった（同上）
- C-2: できなかった（同上）
- C-3: できなかった（同上）
- C-4: できなかった（同上）
- C-5: できなかった（同上。数値なし）
- C-6: できなかった（同上。数値なし）

## 準備で確かめたこと

- 本体（`irl-live-keeper` PID 69624、18:31:03 起動）の status に、カメラが2台あった: `main`（メインカメラ、`on_bad=standby`）と `sub`（サブカメラ、`on_bad=standby`）。どちらも `DOWN`、`online=false`（スマホは2台とも送っていない）
- OBS（`obs64` PID 39132、18:31:13 起動）は配信していなかった（`outputActive false`）。プロファイルは `main`
- status の warnings に「サブカメラ のメディアソースが OBS に見つかりません」が出ていた（元のシーンコレクションに `cam-sub` がないため。`test setup` が作る前の状態なので想定どおり）
- 準備 1 (b)（It's My Live の送信先）は、スマホの送信が必要になる準備 8 でまとめて確かめる予定だった。準備 6 で止まったため、確かめていない
- `ws-password.txt` はあった（中身は見ていない）。`test-original.json` はなかった

## 時刻つきの流れ

- 18:34:31.7 試験用の MediaMTX（`dest.yml`）を起動した（PID 61828）。`127.0.0.1:21935` で待ち受けていることを確認
- 18:34:34.6 `ctl.exe test setup` を実行した。出力:
  ```
  元のプロファイル: main、元のシーンコレクション: test用（test-original.json に残した）
  FAIL: request GetSceneCollectionList: NotReady (207): OBS is not ready to perform the request.
  ```
- OBS のログ:
  ```
  18:34:34.799: Created profile 'IRLKeeper-Test' (clean, IRLKeeperTest)
  18:34:35.633: No scene file found, creating default scene
  18:34:35.668: Not all sources were cleared when clearing scene data:
  18:34:35.668: - IRL
  18:34:35.668: - テキスト (GDI+) 4
  18:34:35.671: Switched to scene 'シーン'
  18:34:35.671: Created scene collection 'IRLKeeper-Test' (clean, IRLKeeperTest.json)
  ```
- 18:34:46〜18:35:02 3秒ごとに `ctl.exe profile` を6回試した。6回とも `NotReady (207)`。`scenes`、`current`、`streamstatus` も同じ
- この間、OBS は応答していた（`Responding=True`）。プロファイルとシーンコレクションの名前は WebSocket から読めなかった
- 18:35:05〜18:37:07 本体も OBS の状態を読めず、`NotReady (207)` の警告を5秒ごとに出し続けた（下のログ）
- 18:37:12 ユーザーが手で戻した（選んだのは「2: 中止して元に戻す」）。OBS のログでは、このとき OBS が終了している（`==== Shutting down`、終了の途中で `Switched to scene collection 'test用'`）。そのあと起動し直された（新しい `obs64` は PID 67620、18:37:15 起動）。18:37:22 に `Switched to profile 'main'`
- 18:37:17 本体が OBS に接続し直した
- 18:38:10〜18:38:25 片づけ（下）

## 本体のログ（logs\irl-live-keeper-2026-11-21.log、time= 行。MediaMTX の行は省いた。同じ警告が続く部分は略した）

```
time=2026-11-21T18:31:20.684+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-21T18:34:35.626+09:00 level=INFO msg="OBS のシーンコレクションの切替が始まった"
time=2026-11-21T18:35:05.627+09:00 level=INFO msg="OBS のイベントが途切れた。読み直す"
time=2026-11-21T18:35:05.627+09:00 level=WARN msg="OBS の状態を取得できない" err="GetStreamStatus: request GetStreamStatus: NotReady (207): OBS is not ready to perform the request."
time=2026-11-21T18:35:06.629+09:00 level=WARN msg="OBS の状態を取得できない" err="GetVersion: request GetVersion: NotReady (207): OBS is not ready to perform the request."
（18:35:08〜18:37:07 も同じ GetVersion の NotReady が約5秒ごとに続く）
time=2026-11-21T18:37:07.666+09:00 level=WARN msg="OBS の状態を取得できない" err="GetVersion: request GetVersion: NotReady (207): OBS is not ready to perform the request."
time=2026-11-21T18:37:12.666+09:00 level=WARN msg="OBS に接続できない" addr=127.0.0.1:4455 err="dial tcp 127.0.0.1:4455: connectex: No connection could be made because the target machine actively refused it."
time=2026-11-21T18:37:17.674+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
```

本体は、シーンコレクションの切替の始まり（18:34:35.626）を受け取っていた。切替の終わりの知らせは来ず、30秒後（18:35:05.627）に「イベントが途切れた」として読み直しに入った。

## 気になった動き、指示書と違ったこと

- **`test setup` のあと、OBS の WebSocket が NotReady から戻らなかった。** OBS のログでは、プロファイルの作成（18:34:34.799）とシーンコレクションの作成（18:34:35.671）が約0.9秒の間に続いていた。プロファイルの切替がまだ終わらないうちにシーンコレクションの作成を始めたため、obs-websocket が「準備できた」に戻れなくなった可能性がある（推測。確かめていない）。`ctl.exe test setup` の中で、プロファイルを切り替えたあとに OBS が準備できるのを待ってから（`GetVersion` などが通るまで再試行してから）シーンコレクションへ進むように直すとよいかもしれない
- 本体は NotReady のあいだ落ちず、OBS が起動し直されると5秒で接続し直した
- ユーザーが元に戻したとき、OBS は一度終了して起動し直されていた（PID 39132 → 67620）。メニューで切り替えたのか、終了して起動し直したのかは、ログからは区別できなかった。起動し直した OBS は、はじめ試験のプロファイルで立ち上がったらしく、18:37:22 に `main` へ切り替わった
- 見張り（準備 7 の `watch-status.ps1` と `ctl.exe watch`）は始める前に止まったので、`status-cam.txt` と `obs-cam.txt` はない
- 準備 1 (b) は確かめていない（上に書いた理由による）
- 参考: 前の T-12 では `ctl.exe watch` を裏で動かしたまま止めると、ファイルの最後の部分が欠けた。C-1〜C-7 をやり直すときは、`watch` に試験の長さちょうどの秒数を渡して自然に終わらせるか、ctl 側で1行ごとに書き出すようにするとよい

## 片づけの結果

- 18:38:10.8 `ctl.exe stream stop`: `FAIL: request StopStream: OutputNotRunning (501)`。配信していなかったので想定どおり（直前の `streamstatus` は `outputActive false`）
- 18:38:13.9 `ctl.exe test restore`: 成功（`元のプロファイル main と、元のシーンコレクション test用 に戻した（試験のシーンコレクション IRLKeeper-Test は残る）`）。OBS のログでは 18:38:14.933 に `Removed profile 'IRLKeeper-Test'`
- `ctl.exe profile`: `main`。準備 6 で控えた元のプロファイルと同じ
- `ctl.exe scenes` の1行目: `シーンコレクション: test用  今のシーン: Camera`。準備 6 で控えた元のシーンコレクションと同じ
- `test-original.json`: `test restore` が消した（消えていることを確かめた）
- 止めたプロセス: 試験用の MediaMTX PID 61828（18:38:25.0、PID で止めた）。`127.0.0.1:21935` の待ち受けはなくなった
- 止めていないプロセス: 本体 `irl-live-keeper` PID 69624、本体の MediaMTX PID 67672、OBS `obs64` PID 67620
- 試験のシーンコレクション `IRLKeeper-Test` は OBS に残っている（次の `test setup` で使い直す。消すなら OBS の「シーンコレクション」→「削除」）
- `config.yaml` の `sub` はそのままでよい（次の試験でも使う）
- 試験で作ったファイル: `dest-cam.out.txt`、`dest-cam.err.txt`、`cam-result.md`
