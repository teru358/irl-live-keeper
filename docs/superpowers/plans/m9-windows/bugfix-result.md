# B-1〜B-6 の確認結果（不具合の直しの確かめ）

- 実施日: 2026-11-22 21:13〜21:18
- 本体: `irl-live-keeper.exe`（2026-11-22 21:10:17、14,223,360 バイト）
- スクリプト: 4つとも 2026-11-22 21:10:17 の版。ユーザーが最新版だと確かめた
  - `show-all.ps1`: 2,097 バイト
  - `standby.ps1`: 2,307 バイト
  - `watch-status.ps1`: 3,074 バイト
  - `watch-obs.ps1`: 2,023 バイト
- 実行前に4つのスクリプトを読んだ
  - パスワードを表示しない
  - `config.yaml` と `ws-password.txt` の既定のパスは、本文で `$PSScriptRoot`（空なら `$MyInvocation.MyCommand.Path` の親）から決めている
- スマホは使わず、配信も `test setup` もしていない
- 本体のログ: `logs\irl-live-keeper-2026-11-22.log`

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| B-1 | 合格 | `-Config` なしで `standby.ps1 on`。Join-Path のエラーが出ず、HTTP の応答（409 配信中でない）が出た |
| B-2 | 合格 | `standby.ps1 off` も同じく 409 の応答 |
| B-3 | 合格 | `show-all: 200 {"ok":true}` |
| B-4 | 合格 | status の JSON が出た（61行）。`urls` はない |
| B-5 | 合格 | エラーなく約3秒動いて終わった（終了コード 0）。`ws-password.txt` はもとからあったので、作られることは確かめていない |
| B-6a | 合格 | Ctrl+C で止めたあと、`level=WARN msg="MediaMTX が終了しました"` は出ていない（INFO の行も出なかった）。本体の MediaMTX は残っていない |
| B-6b | 合格 | 窓の × で閉じたあと、`level=INFO msg="MediaMTX が終了しました（本体の終了と同時）"` が出て、WARN は出ていない。本体の MediaMTX は残っていない |
| B-6c | 合格 | 本体の MediaMTX だけを終了すると、`level=WARN msg="MediaMTX が終了しました" ... error="exit status 1"` が出て、約1秒後に MediaMTX が起動し直した。最後の Ctrl+C でも WARN は出ていない |

## B-1〜B-5（21:15:05〜21:15:09）

本体（PID 90292、21:13:08 起動）が動いていて、OBS につながっている（21:13:15 `OBS に接続した`）状態で行った。

### B-1

```
> powershell -ExecutionPolicy Bypass -File .\standby.ps1 on
21:15:05.060 standby on: 失敗 409 リモート サーバーがエラーを返しました: (409) 競合 {"error":"受け付けられない: 配信中でない"}
```

### B-2

```
> powershell -ExecutionPolicy Bypass -File .\standby.ps1 off
21:15:05.358 standby off: 失敗 409 リモート サーバーがエラーを返しました: (409) 競合 {"error":"受け付けられない: 配信中でない"}
```

### B-3

```
> powershell -ExecutionPolicy Bypass -File .\show-all.ps1
21:15:05.688 show-all: 200 {"ok":true}
```

### B-4

`powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Once` の出力は JSON で61行あり、`"urls"` の文字は入っていなかった。要点:

- 最上位のキー: `cameras`、`obs`、`mediamtx`、`last_error`
- `obs`: `connected=True streaming=False scene=Camera`、`standby={"owner":"none","return_scene":null,"stack":[]}`
- `cameras`: `main=DOWN sub=DOWN`（スマホは送っていない）
- `warnings`: `サブカメラ のメディアソースが OBS に見つかりません`（ユーザーの普段の構成の警告。前回から出ている）

### B-5

```
> powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Seconds 3
（開始 21:15:06.104）
21:15:07.399 MediaInputPlaybackStarted IRL
21:15:07.401 MediaInputPlaybackEnded IRL
21:15:07.466 MediaInputPlaybackStarted メディア
21:15:07.468 MediaInputPlaybackEnded メディア
（終了 21:15:09.425、終了コード 0）
```

- 既定の `-Out .\watch.txt` に書かれた（試験の前はなかった。350 バイト）
- 本体のログには、B-1〜B-5 の時刻の行はなかった（409 と show-all は、ログに出ない）

## B-6a（Ctrl+C）

- ユーザーが本体（PID 90292）の窓で Ctrl+C を押した（21:15:10〜21:15:45 の間）
- 21:15:46 に確かめた:
  - `Get-Process` に `irl-live-keeper` も `mediamtx` もない
  - 8554 で待ち受けているものもない
- 止めたあと、本体のログには行が増えていない
  - ログのファイルの最終更新は 21:13:15
  - 最後の行は `time=2026-11-22T21:13:15.330+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455`
  - WARN も INFO（本体の終了と同時）も出ていない。指示書では INFO は出なくてもよいので、合格とした

## B-6b（コンソールの窓の ×）

- ユーザーが本体を起動し直し（21:16:29 `OBS に接続した`）、窓の × で閉じた
- 21:16:49 に確かめた: `irl-live-keeper` と `mediamtx` がなく、8554 で待ち受けているものもない
- 本体のログの最後の行:

```
time=2026-11-22T21:16:29.873+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T21:16:29.922+09:00 level=INFO msg="2026/11/22 21:16:29 INF [RTSP] started with listeners on :8554 (TCP/RTSP)" component=mediamtx
（MediaMTX の起動の行、略）
time=2026-11-22T21:16:34.776+09:00 level=INFO msg="MediaMTX が終了しました（本体の終了と同時）" after=4.9080732s error="exit status 0xc000013a"
```

## B-6c（本体の MediaMTX だけを終了。比べるため）

- ユーザーが本体を起動し直した
  - 本体: PID 73468、21:17:04 起動
  - 本体の MediaMTX: PID 76296、8554 で待ち受け
- ユーザーがタスクマネージャーで `mediamtx.exe`（PID 76296）だけを終了した
- 本体のログ:

```
time=2026-11-22T21:17:05.053+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T21:17:05.098+09:00 level=INFO msg="2026/11/22 21:17:05 INF [RTSP] started with listeners on :8554 (TCP/RTSP)" component=mediamtx
time=2026-11-22T21:17:49.303+09:00 level=WARN msg="MediaMTX が終了しました" after=43.2550931s error="exit status 1"
time=2026-11-22T21:17:50.350+09:00 level=INFO msg="2026/11/22 21:17:50 INF MediaMTX v1.21.4, windows, amd64" component=mediamtx
time=2026-11-22T21:17:50.352+09:00 level=INFO msg="2026/11/22 21:17:50 INF [RTSP] started with listeners on :8554 (TCP/RTSP)" component=mediamtx
time=2026-11-22T21:17:50.352+09:00 level=INFO msg="2026/11/22 21:17:50 INF [RTMP] started with listener on :1935 (TCP/RTMP)" component=mediamtx
time=2026-11-22T21:17:50.352+09:00 level=INFO msg="2026/11/22 21:17:50 INF [SRT] started with listener on :8890 (UDP/SRT)" component=mediamtx
time=2026-11-22T21:17:50.353+09:00 level=INFO msg="2026/11/22 21:17:50 INF [API] started with listener on 127.0.0.1:9997 (TCP/HTTP)" component=mediamtx
```

- 21:17:59 に確かめた:
  - 本体 PID 73468 は動いたまま
  - 新しい MediaMTX PID 97584（21:17:50 起動）が 8554 で待ち受けている
- 最後に、ユーザーが本体の窓で Ctrl+C を押した
  - 21:18:27 に確かめた: `irl-live-keeper` と `mediamtx` がなく、8554 で待ち受けているものもない
  - 21:17:51 以降、本体のログに行は増えていない。WARN は出ていない

## 気になった動き

1. **Ctrl+C で止めたとき（B-6a と B-6c の最後）、本体のログに何も出なかった。** 終了の行も、MediaMTX の終了の行もない。窓の × で閉じたとき（B-6b）だけ、INFO の行が出た。
   - 指示書では「出ないこともある」とされているので、不合格にはしていない。
   - ただ、ログだけでは本体が止まった時刻がわからない。止まったことを示す行（例:「止める」）があると、記録に便利。
2. **今日の他の終了のときの行。** 本体の起動し直しのたびに、次の行が出ていた。新しい版の起動（21:13:08）の直前の終了は、INFO の形だった。
   - `20:28:48 level=WARN msg="MediaMTX が終了しました" ... error="exit status 0xc000013a"`（前の版）
   - `21:13:06 level=INFO msg="MediaMTX が終了しました（本体の終了と同時）"`
3. **B-6c の WARN の時刻。**
   - `after=43.2550931s` から数えると、MediaMTX が終わったのは 21:17:48.35 ごろ。WARN が出たのは約1秒後の 21:17:49.303 で、「1秒待って本体の終了を見る」直しのとおり。
   - 起動し直しは、その約1秒後（21:17:50.350）。
4. **B-5 のファイル。** `watch-obs.ps1` を既定のまま動かしたので、試験のフォルダに `watch.txt`（350 バイト）ができた。消していない。

## 片づけ

- `test setup` と配信はしていないので、OBS の設定は変えていない（B-3 の show-all は、隠れた項目を表示に戻す操作。前の試験の終わりには `hidden.json` は0件だった）
- 自分で起動したプロセスはない（スクリプトはすべて終わっている）
- 本体と本体の MediaMTX は、ユーザーの Ctrl+C で止まったまま。OBS は動いている
