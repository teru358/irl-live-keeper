# W0c-0〜W0c-9 の確認結果（シーンごとの逃がし先、0c）

- 実施日: 2026-11-22 20:28〜20:40
- 本体: 新しい `irl-live-keeper.exe`（2026-11-22 20:16:31、14,224,384 バイト）
  - ユーザーが `config.yaml` に `scene_fallbacks`（`{scene: T-A, fallback: T-Sub}`）を書き足し、起動し直した（20:29:28 起動、PID 78268、本体の MediaMTX は PID 82376）
  - 書き足す前の起動（20:25:42、PID 25268）では警告が出なかったので、ユーザーに直してもらった
- `ctl.exe`: 2026-11-22 20:21:32、11,415,040 バイト
- `config.yaml`: ユーザーが書き足した。ファイルは表示も書き換えもしていない
  - `main` は `on_bad: standby`、`sub` は `on_bad: hide`（ユーザーの申告）
- 送り方と回線:
  - 2台とも RTMP で送った
  - main（Android）は LAN（`192.168.1.11`）
  - sub（iOS）は全項目でキャリア回線から Tailscale（`100.118.173.77`）
- 見張りのファイル: `status-fb.txt`、`obs-fb.txt`
- stack は各項目で `watch-status.ps1 -Once` の `obs.standby.stack` を読んだ

## 判定のまとめ

| # | 判定 | stack | 理由 |
|---|---|---|---|
| W0c-0 | 合格 | `[]` | 前は「scene_fallbacks の 1 行目: scene「T-A」が OBS にない…」と「fallback「T-Sub」が OBS にない…」の2行。`test setup` のあとは `warnings` が空 |
| W0c-1 | 合格 | `["T-A"]` | `scene switch from=T-A to=T-Sub reason=down camera=main purpose=AutoFail`、owner `auto`、return `T-A`。sub は隠さない |
| W0c-2 | 合格 | `["T-A","T-Sub"]` | `from=T-Sub to=⚠回線待機中`。待機に着いた（`standby changed` 20:32:44.895）あと 20:32:44.897 に `camera hidden camera=sub items=2` |
| W0c-3 | 合格 | `["T-A"]` | `camera shown camera=sub reason=ok` のあと、`from=⚠回線待機中 to=T-Sub purpose=AutoRecover` |
| W0c-4 | 合格 | `[]` | `from=T-Sub to=T-A purpose=AutoRecover`、owner `none` |
| W0c-5 | 合格 | `["T-B"]` → `[]` | `scene T-B` で `scene changed by user`、`standby changed owner=none`。同じ時刻に T-B の main が悪いので `from=T-B to=⚠回線待機中 AutoFail`（return `T-B`）。main が戻ると T-B へ |
| W0c-6 | 合格 | `[]` → `["T-A"]` | ON で owner `manual`、return `T-A`、stack `[]`。OFF で `StandbyOffMonitored` により T-Sub へ、owner `auto`、stack `["T-A"]`。main が戻ると T-A へ。ただし `standby.ps1` は `-Config` を付けないと動かなかった（下の「気になった動き」1） |
| W0c-7 | 合格 | `["T-A","T-Sub"]` → `[]` | ON の応答 200。`scene switch` なし（切替なし）で owner `manual`、return `T-A`、stack `[]`。2台が OK になっても待機のまま（20秒近く見た）。OFF で `StandbyOffMonitored` により T-A へ |
| W0c-8 | 合格 | `[]` | T-A のまま sub を隠し（`camera hidden items=2 reason=down`）、送り直すと `camera shown reason=ok` |
| W0c-9 | 記録のみ | `["T-A","T-Sub"]` | 2台の DOWN が同じ判定の回（20:38:59.569）に入り、T-Sub を経ずに T-A から待機へ直接移った。stack は `["T-A","T-Sub"]` |

## 準備

- 20:28:29、`config.yaml` を直す前の `-Once`。`warnings` は「サブカメラ のメディアソースが OBS に見つかりません」だけで、`scene_fallbacks` の警告がなかった
  - ユーザーに設定と起動し直しを頼んだ
- 20:30:13（起動し直したあと）の `-Once` の `warnings`:
  - `scene_fallbacks の 1 行目: scene「T-A」が OBS にないため、この行は使われません`
  - `scene_fallbacks の 1 行目: fallback「T-Sub」が OBS にないため、共通の待機シーンへ逃がします`
  - `サブカメラ のメディアソースが OBS に見つかりません`
- 試験だけの MediaMTX（`dest.yml`）: PID 80680、`127.0.0.1:21935` で待ち受けを確かめた
- `ctl.exe scenes` の1行目は `test用`。`test setup`（20:30:35）の出力: 元のプロファイル `main`、シーンコレクション `test用`、シーン `Camera`。`T-A`、`T-B`、`T-Sub` を足した
- 20:30:42 の `-Once` の `warnings` は空（W0c-0 のあと）
- 送信の開始:
  - 20:31:27 sub 接続（`100.118.173.77`）
  - 20:31:32 main 接続（`192.168.1.11`）
  - 20:31:45 2台とも OK
  - `inputs` で `cam-main` と `cam-sub` が `PLAYING`、cursor が進むことを2回読んで確かめた
- `profile` が `IRLKeeper-Test` であることを確かめてから、`scene T-B` → `stream start`（20:31:55）→ `scene T-A`（20:31:59）。20:32:01 `scene=T-A owner=none`、stack `[]`

## W0c-1（T-A で main を止める）

- 20:32:22 main を止めてもらった
- status:

```
20:32:22.909 cams[main=DOWN(0) sub=OK(2,685)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=none return= warnings=0
20:32:23.415 cams[main=DOWN(0) sub=OK(2,685)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
```

- `-Once`（20:32:34）: `standby={"owner":"auto","return_scene":"T-A","stack":["T-A"]}`、sub `hidden=0`
- `items`: `T-A` と `T-Sub` の `cam-sub` は `enabled=true`
- OBS: `20:32:22.977 SceneChanged T-Sub`
- 本体のログ:

```
time=2026-11-22T20:32:22.569+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T20:32:22.570+09:00 level=INFO msg="scene switch" from=T-A to=T-Sub reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T20:32:22.977+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
```

## W0c-2（続けて sub も止める）

- status:

```
20:32:44.689 cams[main=DOWN(0) sub=DOWN(0)] streaming=True scene=T-Sub class=monitored health=DOWN scene_cams=[sub] owner=auto return=T-A warnings=0
20:32:45.197 cams[main=DOWN(0) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
```

- `-Once`（20:32:55）: `standby={"owner":"auto","return_scene":"T-A","stack":["T-A","T-Sub"]}`、sub `hidden=2`
- `items`: `T-A` と `T-Sub` の `cam-sub` が `enabled=false`
- OBS: `20:32:44.895 SceneChanged ⚠回線待機中`
- 本体のログ:

```
time=2026-11-22T20:32:44.569+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T20:32:44.569+09:00 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-22T20:32:44.895+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T20:32:44.897+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
```

## W0c-3（sub だけ送り直す）

- 20:33:03 sub 接続（`100.118.173.77`、Tailscale）
- status:

```
20:33:07.614 cams[main=DOWN(0) sub=LOW(1,541)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
20:33:16.727 cams[main=DOWN(0) sub=OK(2,211)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
20:33:18.241 cams[main=DOWN(0) sub=OK(1,985)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
```

- `-Once`（20:33:18）: `standby={"owner":"auto","return_scene":"T-A","stack":["T-A"]}`、sub `hidden=0`
- `items`: `cam-sub` が `enabled=true` に戻った
- OBS: `20:33:17.895 SceneChanged T-Sub`
- 本体のログ（表示に戻してから T-Sub へ）:

```
time=2026-11-22T20:33:16.570+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2210.7
time=2026-11-22T20:33:17.580+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T20:33:17.581+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-Sub reason=ok camera=sub kbps=1984.9 purpose=AutoRecover
time=2026-11-22T20:33:17.895+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
```

## W0c-4（main も送り直す）

- 20:33:32 main 接続（`192.168.1.11`）
- status:

```
20:33:46.597 cams[main=OK(3,469) sub=OK(2,305)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
20:33:48.109 cams[main=OK(4,036) sub=OK(1,525)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

- `-Once`（20:33:48）: `standby={"owner":"none","return_scene":null,"stack":[]}`
- OBS: `20:33:47.895 SceneChanged T-A`
- 本体のログ:

```
time=2026-11-22T20:33:46.569+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3469.3
time=2026-11-22T20:33:47.570+09:00 level=INFO msg="scene switch" from=T-Sub to=T-A reason=ok camera=main kbps=4035.6 purpose=AutoRecover
time=2026-11-22T20:33:47.895+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

## W0c-5（人の切替）

- main を止めてもらう → 20:33:59 T-Sub、`-Once` で `stack=["T-A"]` を確かめた
- 20:34:08.514 `ctl.exe scene T-B`
- status:

```
20:34:08.340 cams[main=DOWN(0) sub=OK(2,013)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
20:34:09.354 cams[main=DOWN(0) sub=OK(2,191)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-B warnings=0
```

- `-Once`（20:34:13）: `standby={"owner":"auto","return_scene":"T-B","stack":["T-B"]}`
- OBS: `20:34:08.862 SceneChanged T-B`、`20:34:09.195 SceneChanged ⚠回線待機中`
- 本体のログ:

```
time=2026-11-22T20:34:08.862+09:00 level=INFO msg="scene changed by user" from=T-Sub to=T-B
time=2026-11-22T20:34:08.862+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T20:34:08.862+09:00 level=INFO msg="scene switch" from=T-B to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T20:34:09.195+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-B
```

- main を送り直してもらう（20:34:27 接続 `192.168.1.11`）:

```
time=2026-11-22T20:34:40.569+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3838.6
time=2026-11-22T20:34:41.570+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-B reason=ok camera=main kbps=3621 purpose=AutoRecover
time=2026-11-22T20:34:41.895+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

- `-Once`（20:34:43）: `scene=T-B`、`standby={"owner":"none","return_scene":null,"stack":[]}`
- このあと `ctl.exe scene T-A` で次の項目の始まりの形に戻した

## W0c-6（手動の待機）

- main を止めてもらう → 20:35:01 T-Sub、`stack=["T-A"]`
- 最初の `standby.ps1 on` と `off`（20:35:12〜）は、どちらも次のエラーで止まった。要求は送られず、状態は変わらなかった（本体のログに行なし）:

```
Join-Path : Cannot bind argument to parameter 'Path' because it is an empty string.
At C:\Users\terus\Desktop\windows-test2\standby.ps1:14 char:32
+   [string]$Config = (Join-Path $PSScriptRoot "config.yaml")
```

- `-Config <config.yaml のフルパス>` を付けてやり直した（パスワードはスクリプトが読み、表示していない）
  - 20:35:33.602 `standby on: 200 {"ok":true}`
  - 20:35:35 の `-Once`: `scene=⚠回線待機中 standby={"owner":"manual","return_scene":"T-A","stack":[]}`
  - 20:35:39.221 `standby off: 200 {"ok":true}`
  - 20:35:42 の `-Once`: `scene=T-Sub standby={"owner":"auto","return_scene":"T-A","stack":["T-A"]}`
- status:

```
20:35:33.349 cams[main=DOWN(0) sub=OK(2,162)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
20:35:34.365 cams[main=DOWN(0) sub=OK(2,074)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=manual return=T-A warnings=0
20:35:39.925 cams[main=DOWN(0) sub=OK(1,530)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=auto return=T-A warnings=0
```

- OBS: `20:35:33.929 SceneChanged ⚠回線待機中`、`20:35:39.561 SceneChanged T-Sub`
- 本体のログ:

```
time=2026-11-22T20:35:33.619+09:00 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=command purpose=StandbyOn
time=2026-11-22T20:35:33.929+09:00 level=INFO msg="standby changed" owner=manual return_scene=T-A
time=2026-11-22T20:35:39.240+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-Sub reason=command purpose=StandbyOffMonitored
time=2026-11-22T20:35:39.561+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
```

- main を送り直してもらう（20:35:56 接続 `192.168.1.11`）:

```
time=2026-11-22T20:36:09.569+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3496.5
time=2026-11-22T20:36:10.570+09:00 level=INFO msg="scene switch" from=T-Sub to=T-A reason=ok camera=main kbps=3851.6 purpose=AutoRecover
time=2026-11-22T20:36:10.896+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

## W0c-7（待機の自動の ON の特例）

- main を止めてもらう → 20:36:31 T-Sub
- sub を止めてもらう → 20:36:47 待機
- 20:36:59 の `-Once`: `standby={"owner":"auto","return_scene":"T-A","stack":["T-A","T-Sub"]}`
- 本体のログ（待機へ）:

```
time=2026-11-22T20:36:47.569+09:00 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-22T20:36:47.895+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T20:36:47.907+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
```

- 20:36:59.994 `standby on: 200 {"ok":true}`。`scene switch` は出ず、OBS の `SceneChanged` もない:

```
time=2026-11-22T20:37:00.012+09:00 level=INFO msg="standby changed" owner=manual return_scene=T-A
```

- `-Once`（20:37:02）: `standby={"owner":"manual","return_scene":"T-A","stack":[]}`
- 2台を送り直してもらう:
  - 20:37:16 sub 接続（`100.118.173.77`）
  - 20:37:21 main 接続（`192.168.1.11`）
  - 20:37:31.570 `camera shown camera=sub items=2 reason=ok`
  - 20:37:33.569 main OK
- 20:37:44 の `-Once`（2台 OK から約10秒後）: `scene=⚠回線待機中 standby={"owner":"manual","return_scene":"T-A","stack":[]}`
  - 待機のままで、OBS の `SceneChanged` もない
- status:

```
20:37:34.019 cams[main=OK(3,442) sub=OK(2,100)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=manual return=T-A warnings=0
20:37:50.210 cams[main=OK(3,606) sub=OK(2,001)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=manual return=T-A warnings=0
20:37:52.729 cams[main=OK(3,940) sub=OK(2,232)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

- 20:37:50.909 `standby off: 200 {"ok":true}`:

```
time=2026-11-22T20:37:51.926+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=command purpose=StandbyOffMonitored
time=2026-11-22T20:37:52.228+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

- `-Once`（20:37:54）: `scene=T-A standby={"owner":"none","return_scene":null,"stack":[]}`

## W0c-8（sub だけ悪い）

- sub を止めてもらう。status:

```
20:38:11.976 cams[main=OK(3,499) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

- `-Once`（20:38:22）: `scene=T-A standby={"owner":"none","return_scene":null,"stack":[]}`、sub `hidden=2`
- `items`: `T-A` と `T-Sub` の `cam-sub` が `enabled=false`。OBS の `SceneChanged` はない
- 本体のログ:

```
time=2026-11-22T20:38:11.569+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T20:38:11.572+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
```

- sub を送り直してもらう（20:38:28 接続 `100.118.173.77`）:

```
time=2026-11-22T20:38:41.569+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=1521.6
time=2026-11-22T20:38:42.571+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
```

- `-Once`（20:38:45）: `scene=T-A`、stack `[]`、`cam-sub` は `enabled=true`

## W0c-9（同時に悪くなる、記録のみ）

- 2台をできるだけ同時に止めてもらった。status:

```
20:38:59.615 cams[main=DOWN(0) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=none return= warnings=0
20:39:00.123 cams[main=DOWN(0) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
```

- `-Once`（20:39:24）: `standby={"owner":"auto","return_scene":"T-A","stack":["T-A","T-Sub"]}`、sub `hidden=2`
- `items`: `cam-sub` が `enabled=false`
- OBS: `20:38:59.894 SceneChanged ⚠回線待機中`（T-Sub を経ていない）
- 本体のログ:

```
time=2026-11-22T20:38:59.569+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T20:38:59.569+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T20:38:59.569+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T20:38:59.578+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T20:38:59.894+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
```

- 止めた直後に、Android（main）が1回だけ短くつなぎ直した:
  - 20:39:00.163 `[RTMP] [conn 192.168.1.11:41084] opened`
  - 20:39:01.565 `closed: EOF`
  - 本体の判定は DOWN のままで、項目には響かなかった

## 気になった動き、指示書と違ったこと

1. **`standby.ps1` が既定のままでは動かない。** `powershell -ExecutionPolicy Bypass -File .\standby.ps1 on` を Windows PowerShell 5.1 で動かすと、`param` の `[string]$Config = (Join-Path $PSScriptRoot "config.yaml")` で `Join-Path : Cannot bind argument to parameter 'Path' because it is an empty string.` になって止まる。
   - Windows PowerShell 5.1 では、`param` の既定値を評価する時点で `$PSScriptRoot` が空になるため。
   - `-Config` に `config.yaml` のパスを渡すと動いた。
   - 直し方の案: 既定値を空にして、本文で `if (-not $Config) { $Config = Join-Path $PSScriptRoot "config.yaml" }` とする。`show-all.ps1` では、前回この問題は出ていない。
2. **W0c-9 の切替の順。** `camera hidden`（20:38:59.578）が、OBS の待機への切替の確認（`SceneChanged` と `standby changed`、20:38:59.894）より約0.3秒早く出た。
   - W0c-2 と W0c-7 では、`standby changed` の 2〜12ms あとに `camera hidden` が出ていた。
   - 同じ判定の回で2台が落ちると、切替の要求を出した直後に隠している。
   - 待機の切替は約0.3秒で済むので、その間、T-A の右下（sub の位置）が先に消えた可能性がある（OBS の画面は見ていない）。
3. **W0c-9 の stack。** T-Sub を経ていないのに、stack は `["T-A","T-Sub"]` だった。指示書の期待どおり（逃がし先の並びを積んでから待機へ送る形）。
4. **W0c-7 の OFF の応答の時刻。** `standby.ps1 off` の時刻の表示（20:37:50.909。要求の前に取った時刻）と、本体のログの `scene switch`（20:37:51.926）の間が約1秒あった。W0c-6 では、ON と OFF のどちらも約20ms だった。害はない。
5. **`test restore` のあと。** `warnings` に `scene_fallbacks の 1 行目` の警告が2行戻った（20:40:05）。指示書のとおり、T-A がないと警告が出るだけ。
6. **ユーザーの普段の構成の警告。** 「サブカメラ のメディアソースが OBS に見つかりません」は、試験の前と後に出ていた（試験の間は出ていない）。前回から出ていたもので、この確認の項目には関係しない。

## 片づけ

- 20:39:46 `studio off`、`stream stop`
  - 本体のログ: `stream state streaming=false`、`standby changed owner=none`、`camera shown camera=sub items=2 reason=stream_stop`
- 3秒待って `test restore`
  - 出力: 「試験のシーンとソースを消し、元のプロファイル main とシーン Camera に戻した」。準備で控えた名前と同じ
- `profile` は `main`。`scenes` はシーンコレクション `test用`、今のシーン `Camera`、`T-` のシーンなし
- `test-original.json` は消えた。`state\hidden.json` は 84 バイト（0件）
- 試験だけの MediaMTX（PID 80680）を PID で止めた。21935 の待ち受けはなくなった
- 見張り2つ（`watch-status.ps1` と `ctl.exe watch`）を止めた
- 止めていないもの:
  - 本体（PID 78268）
  - 本体の MediaMTX（PID 82376）
  - OBS（PID 65748）
