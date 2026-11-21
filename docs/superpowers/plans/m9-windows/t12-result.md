# T-12 結果（Windows、2026-11-21）

## 判定: 合格

OBS が配信先との接続を失って再接続していた約35秒（17:56:39.7〜17:57:14.7）のあいだ、status は 0.5 秒ごとの全行で `streaming=True`、owner `auto`、return `Camera` のままだった。再接続後にカメラが送信を始めると `Camera` に戻り、owner は `none` になった。

## 時刻つきの流れ

- 17:56:05.1 試験用の MediaMTX（`dest.yml`）を起動した（PID 66016）。`127.0.0.1:21935` で待ち受けていることを確認
- 17:56:12.8 `ctl.exe t12 setup` を実行。元のプロファイル `main` を控え、`IRLKeeper-T12` に切り替えた
- 17:56:20.3 `ctl.exe scene Camera` に続けて `ctl.exe stream start` を実行（OBS の `OUTPUT_STARTED` は 17:56:20.383）
- 17:56:20.384 本体のログに `scene switch from=Camera to=⚠回線待機中 reason=down`
- 17:56:25.406 OBS の `SceneChanged ⚠回線待機中`。本体のログに `standby changed owner=auto return_scene=Camera`
- 17:56:25.825 status が `streaming=True scene=⚠回線待機中 owner=auto return=Camera` になった
- 17:56:39.742 `Stop-Process -Id 66016` で試験用の配信先を止めた
- 17:56:39.752 OBS の `OUTPUT_RECONNECTING`。`streamstatus` は 17:56:41.756 から `outputReconnecting true`。OBS はこのあと 17:56:43.8 / 48.9 / 55.5 / 17:57:04.4 にも再試行した
- 17:56:39〜17:57:01 status 44 行すべてが `streaming=True ⚠回線待機中 owner=auto return=Camera`
- 17:57:10.661 試験用の MediaMTX を起動し直した（PID 63064）
- 17:57:14.738 OBS の `OUTPUT_RECONNECTED active=true`。`streamstatus` は 17:57:14.790 に `outputReconnecting false`
- 17:57:15〜17:58:44 status は `streaming=True ⚠回線待機中 owner=auto return=Camera` のまま（main が DOWN の行 144、LOW の行 20、OK の行 11）
- 17:58:24.7 スマホが本体の MediaMTX に接続した（`192.168.1.11`、path `main`）
- 17:58:27.938 本体のログに `camera changed main DOWN→LOW kbps=4517`
- 17:58:37.938 本体のログに `camera changed main LOW→OK kbps=3830.2`
- 17:58:38.940 本体のログに `scene switch from=⚠回線待機中 to=Camera reason=ok purpose=AutoRecover`
- 17:58:43.972 本体のログに `standby changed owner=none return_scene=""`
- 17:58:44.017 status が `scene=Camera ... owner=none return=` になった
- 17:58:52.9 `ctl.exe stream stop` を実行（本体のログでは 17:58:53.534 に `stream state streaming=false`）

## status-t12.txt（抜粋）

配信開始と待機への切替:
```
17:56:20.244 cams[main=DOWN(0)] streaming=False scene=Camera class=monitored health=DOWN scene_cams=[main] owner=none return= warnings=0
17:56:20.754 cams[main=DOWN(0)] streaming=True scene=Camera class=monitored health=DOWN scene_cams=[main] owner=none return= warnings=0
17:56:25.316 cams[main=DOWN(0)] streaming=True scene=Camera class=monitored health=DOWN scene_cams=[main] owner=none return= warnings=0
17:56:25.825 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
```

配信先を止める前後（17:56:39.742 に停止）:
```
17:56:39.037 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:56:39.545 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:56:40.054 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:56:40.566 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
```

再接続の前後（17:57:14.738 に再接続）:
```
17:57:14.103 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:57:14.605 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:57:15.111 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:57:15.615 cams[main=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
```

カメラで戻る:
```
17:58:28.288 cams[main=LOW(4,517)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:58:38.431 cams[main=OK(3,830)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:58:43.505 cams[main=OK(3,627)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=Camera warnings=0
17:58:44.017 cams[main=OK(3,554)] streaming=True scene=Camera class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
```

## obs-t12.txt の StreamStateChanged

```
17:56:20.307 StreamStateChanged OBS_WEBSOCKET_OUTPUT_STARTING active=false
17:56:20.383 StreamStateChanged OBS_WEBSOCKET_OUTPUT_STARTED active=true
17:56:39.752 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTING active=false
17:56:43.849 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTING active=false
17:56:48.900 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTING active=false
17:56:55.492 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTING active=false
17:57:04.377 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTING active=false
17:57:14.738 StreamStateChanged OBS_WEBSOCKET_OUTPUT_RECONNECTED active=true
```

## 本体のログ（logs\irl-live-keeper-2026-11-21.log、試験中の time= 行。MediaMTX の RTSP/RTMP の接続の行は一部を省いた）

```
time=2026-11-21T17:55:36.950+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-21T17:56:20.383+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-21T17:56:20.384+09:00 level=INFO msg="scene switch" from=Camera to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-21T17:56:25.406+09:00 level=INFO msg="standby changed" owner=auto return_scene=Camera
time=2026-11-21T17:58:24.725+09:00 level=INFO msg="2026/11/21 17:58:24 INF [RTMP] [conn 192.168.1.11:49640] opened" component=mediamtx
time=2026-11-21T17:58:26.801+09:00 level=INFO msg="2026/11/21 17:58:26 INF [RTMP] [conn 192.168.1.11:49640] is publishing to path 'main'" component=mediamtx
time=2026-11-21T17:58:27.938+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=4517
time=2026-11-21T17:58:37.938+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3830.2
time=2026-11-21T17:58:38.940+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=Camera reason=ok camera=main kbps=3506.9 purpose=AutoRecover
time=2026-11-21T17:58:43.972+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T17:58:53.534+09:00 level=INFO msg="stream state" streaming=false
```

再接続のあいだ（17:56:39〜17:57:15）、本体のログには何も出ていない（`stream state` の変化も警告もない）。

## 気になった動き・指示書と違ったこと

- **`scene switch` のログから実際のシーン切替までに、どちらの向きでも約5秒の差がある。** 待機へ: ログ 17:56:20.384 → OBS の `SceneChanged` と `standby changed` は 17:56:25.406（差 5.02秒）。戻り: ログ 17:58:38.940 → `standby changed` 17:58:43.972、status が Camera になったのは 17:58:44.0（差 5.03秒）。差がほぼ同じなので、決まった5秒の待ち（タイムアウト、あるいは意図した遅延）があるように見える。意図した動きかどうか確かめてほしい
- 手順 4 の「5秒待って最後の行」の時点（17:56:25.3）では、まだ `scene=Camera owner=none` だった。待機に入ったのは 17:56:25.8（開始から約5.5秒）。上の5秒の差によるもので、試験の判定には影響しない
- 再接続までの時間は約35秒で、Linux の約11秒より長い。これは配信先を約31秒止めていたためで（手順 5 で20秒見たあと、手順 6 で起動し直した）、起動し直してから約4秒で再接続した
- `obs-t12.txt` は 17:58:07 で終わっている。`ctl.exe watch` を裏で動かしたまま止めたので、出力の最後の部分がファイルに書かれなかったと思われる。そのため配信停止（17:58:52）の `StreamStateChanged` はこのファイルにない（本体のログの `stream state streaming=false` で確かめた）
- カメラの送信開始は 17:58:24。DOWN→LOW→OK と進み、OK になってから約1秒後に `scene switch` が出た

## 片づけの結果

- `ctl.exe stream stop`: 成功（`outputActive false`）
- `ctl.exe t12 restore`: 成功（「元のプロファイル main に戻した」）
- `ctl.exe profile`: `main`（手順 0 で控えた名前と同じ）
- `t12-original-profile.txt`: 消えている
- 止めたプロセス: 試験用の MediaMTX PID 66016（手順 5）と PID 63064（片づけ）。どちらも PID で止めた。`127.0.0.1:21935` の待ち受けはなくなった
- 止めていないプロセス: 本体 `irl-live-keeper` PID 63148、本体の MediaMTX PID 61308、OBS `obs64` PID 59300
- 裏の見張り（`watch-status.ps1` と `ctl.exe watch`）は2つとも止めた
- 試験で作ったファイル: `status-t12.txt`、`obs-t12.txt`、`dest-mtx-1.out.txt`／`dest-mtx-1.err.txt`、`dest-mtx-2.out.txt`／`dest-mtx-2.err.txt`、`t12-result.md`
