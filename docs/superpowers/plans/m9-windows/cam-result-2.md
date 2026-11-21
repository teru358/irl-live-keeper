# C-1〜C-7 結果（Windows、スマホ2台、2026-11-21、2回目）

前回（準備 6 で止まった回）の記録は `cam-result-attempt1.md` に名前を変えて残した。

## 判定のまとめ

- C-7: 合格。`cameras` に2台あった。`scene_cameras` は T-B で `[main]`、T-A で `[main, sub]`。`scene_health` はどちらも OK
- C-1: 合格。sub が送信をやめてから約2.5秒で待機へ移った（owner `auto`、return `T-A`）。sub が OK になって約1.0秒で T-A へ戻った（owner `none`）
- C-2: 合格。T-B で sub が DOWN、LOW、OK と変わる間、T-B のままで health は OK のままだった
- C-3: 合格。T-Sub にした直後（0.4秒後）に待機へ移った（return `T-Sub`）。sub が OK になって約1.0秒で T-Sub へ戻った。T-B に戻してから sub を止めても、何も起きなかった
- C-4: 合格（2回目）。1回目は、`cam-sub` を隠している間に main（RTMP Streamer）がアプリの制限で止まったので、やり直した。2回目では、隠している間（約25秒）は T-A のままで `scene_cams=[main]`、表示に戻すと0.3秒で待機へ移った（return `T-A`）。sub が OK になって約1.0秒で T-A へ戻った
- C-5: 記録のみ。(1) 同じ Wi-Fi で60秒送った間、2台とも OK のままだった（数値は下）。(2) モバイル回線の準備がないので、できなかった
- C-6: 記録のみ。ハードウェアデコードを切ると、obs64 の CPU は論理コア1つ分の 25% から 31% に増え、GPU のビデオデコードは 13.3% から 11.7% に減った。`reader is too slow` は on も off も0件

## 準備

- 本体（`irl-live-keeper` PID 69624）の status にカメラが2台あった（`main` と `sub`）。OBS（`obs64` PID 67620）は配信していなかった。スマホは2台とも DOWN だった
- 準備 1 (b): ユーザーが送信を始めて、sub が `path 'sub'` に届いた（18:52:09.474、192.168.1.17）。これで (b) を確かめたとした
- 18:50:01.0 試験用の MediaMTX（`dest.yml`）を起動した（PID 74528）。`127.0.0.1:21935` で待ち受けていることを確認
- 18:50:03.8 `ctl.exe test setup` を実行し、成功した:
  ```
  元のプロファイル: main、シーンコレクション: test用、シーン: Camera（test-original.json に残した）
  試験のプロファイル IRLKeeper-Test（配信先 rtmp://127.0.0.1:21935/test）に切り替え、シーンコレクション test用 に試験のシーンを足した
  シーン: T-A（cam-main と cam-sub）、T-B（cam-main）、T-Sub（cam-sub）。待機シーンは ⚠回線待機中。今のシーンは T-B
  ```
  `ctl.exe scenes` に `T-A`、`T-B`、`T-Sub` が出た。今回は NotReady にならなかった
- 見張りを2つ始めた（`watch-status.ps1` → `status-cam.txt`、`ctl.exe watch 3600` → `obs-cam.txt`）
- 18:52 ごろ、ユーザーが2台とも送信を始めた。sub は 18:52:19.7 に OK、main は 18:52:26.7 に OK
- `ctl.exe inputs` を2回読み、4つのソース（`IRL`、`cam-main`、`cam-sub`、`メディア`）がどれも `hw_decode=true OBS_MEDIA_STATE_PLAYING` で、cursor が約3秒進んでいることを確かめた
- 18:52:42.6 `ctl.exe profile` が `IRLKeeper-Test` を出すのを確かめてから、`ctl.exe scene T-B` に続けて `ctl.exe stream start` を実行した。status は `streaming=True scene=T-B health=OK owner=none` になった

## C-7

```
== T-B 18:52:54.571
  id=main name=メインカメラ on_bad=standby state=OK kbps=4679.1 online=True protocol=rtmp
  id=sub name=サブカメラ on_bad=standby state=OK kbps=1800.1 online=True protocol=rtmp
  obs.scene=T-B scene_class=monitored scene_cameras=[main] scene_health=OK owner=none warnings=0
（18:52:54.594 ctl.exe scene T-A）
== T-A 18:52:56.947
  id=main name=メインカメラ on_bad=standby state=OK kbps=3374.4 online=True protocol=rtmp
  id=sub name=サブカメラ on_bad=standby state=OK kbps=2117.5 online=True protocol=rtmp
  obs.scene=T-A scene_class=monitored scene_cameras=[main, sub] scene_health=OK owner=none warnings=0
```

## C-1（T-A で sub を止める、送り直す）

- 18:53 ごろ ユーザーに sub を止めるよう頼んだ
- 18:53:04.492 MediaMTX に `[conn 192.168.1.17] closed: stream unpublished`（sub が送信をやめた）
- 18:53:06.673 本体が `camera changed sub OK→DOWN` を出し、同じ時刻に `scene switch T-A→⚠回線待機中 reason=down camera=sub`
- 18:53:06.995 OBS の `SceneChanged ⚠回線待機中`。本体は `standby changed owner=auto return_scene=T-A`（送信をやめてから約2.5秒）
- 18:53:27.734 sub が送り直した（`is publishing to path 'sub'`）
- 18:53:29.674 sub が DOWN→LOW、18:53:38.674 LOW→OK
- 18:53:39.676 本体が `scene switch ⚠回線待機中→T-A reason=ok`。18:53:39.995 に `SceneChanged T-A` と `standby changed owner=none`（OK から約1.0秒）

```
18:53:06.172 cams[main=OK(3,797) sub=OK(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
18:53:06.680 cams[main=OK(3,510) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=none return= warnings=0
18:53:07.191 cams[main=OK(3,510) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
18:53:30.019 cams[main=OK(3,597) sub=LOW(1,943)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
18:53:39.174 cams[main=OK(3,990) sub=OK(2,019)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
18:53:40.186 cams[main=OK(3,678) sub=OK(2,461)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

## C-2（T-B で sub を止める）

- 18:53:50.5 `ctl.exe scene T-B` を実行した（本体は `scene changed by user T-A→T-B`）
- 18:53:58.772 sub が送信をやめた。18:54:01.674 sub OK→DOWN
- 18:54:31.856 sub が送り直した。18:54:33.674 DOWN→LOW、18:54:42.674 LOW→OK
- この間（18:54:02〜18:54:43）、シーン切替は起きなかった。ずっと `scene=T-B health=OK owner=none`

```
18:53:51.360 cams[main=OK(3,841) sub=OK(1,836)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:54:02.007 cams[main=OK(3,725) sub=DOWN(0)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:54:34.018 cams[main=OK(3,452) sub=LOW(2,411)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:54:43.153 cams[main=OK(4,085) sub=OK(2,239)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
```

## C-3（使わない日のカメラの運用）

- 18:55:14.095 T-B のとき sub が送信をやめた（main は送ったまま）。18:55:15.674 sub OK→DOWN。T-B のまま
- 18:55:24.372 `ctl.exe scene T-Sub` を実行した。18:55:24.762 本体が `scene changed by user T-B→T-Sub`、同じ時刻に `scene switch T-Sub→⚠回線待機中 reason=down camera=sub`
- 18:55:25.095 OBS の `SceneChanged ⚠回線待機中`。`standby changed owner=auto return_scene=T-Sub`
- 18:55:37.567 sub が送り直した。18:55:38.674 DOWN→LOW、18:55:48.674 LOW→OK
- 18:55:49.675 `scene switch ⚠回線待機中→T-Sub reason=ok`。18:55:49.995 に `SceneChanged T-Sub` と `standby changed owner=none`
- 18:55:52.5 `ctl.exe scene T-B` を実行した
- 18:56:02.518 sub が送信をやめた。18:56:03.674 sub OK→DOWN。この間（18:56:03.9〜18:56:27.3、約24秒）何も起きず、`T-B health=OK owner=none` のままだった

```
18:55:24.789 cams[main=OK(3,887) sub=DOWN(0)] streaming=True scene=T-Sub class=monitored health=DOWN scene_cams=[sub] owner=none return= warnings=0
18:55:25.296 cams[main=OK(3,887) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-Sub warnings=0
18:55:49.165 cams[main=OK(3,828) sub=OK(2,311)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-Sub warnings=0
18:55:50.182 cams[main=OK(3,651) sub=OK(2,333)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=none return= warnings=0
18:55:53.231 cams[main=OK(3,955) sub=OK(2,355)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:56:03.895 cams[main=OK(3,448) sub=DOWN(0)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
```

## C-4（T-A で cam-sub を隠す）

### 1回目（main が止まったので、やり直した）

- 18:56:52.2 `ctl.exe scene T-A` を実行した。18:56:54.3 `ctl.exe item T-A cam-sub off` を実行した（OBS の `SceneItemEnableStateChanged T-A item=3 enabled=false`）。`scene_cams=[main]` になった
- 18:57:04.453 sub が送信をやめた。18:57:05.673 sub OK→DOWN。T-A のままだった
- 18:57:14.067 **main が送信をやめた**（ユーザーによれば、RTMP Streamer の制限のため）。18:57:15.674 main OK→DOWN、`scene switch T-A→⚠回線待機中 reason=down camera=main`、`standby changed owner=auto return_scene=T-A`
- 18:57:30.1 `item T-A cam-sub on` を実行した（すでに待機中だった）
- 18:57:28.008 main が送り直した。18:57:39.674 main が OK に戻った。sub が DOWN で、`cam-sub` は表示中なので、待機のままだった（正しい動き）
- 18:58:15.585 sub が送り直した。18:58:26.674 OK。18:58:27.675 `scene switch ⚠回線待機中→T-A reason=ok`

### 2回目

- 18:58:31.0 `ctl.exe item T-A cam-sub off` を実行した。`scene_cams=[main]` になった
- 18:58:39.234 sub が送信をやめた。18:58:41.674 sub OK→DOWN。T-A のままで health は OK だった
- 18:59:07.4 `ctl.exe item T-A cam-sub on` を実行した（隠していた約25秒の間、何も起きなかった）
- 18:59:07.455 本体が `scene switch T-A→⚠回線待機中 reason=down camera=sub`。18:59:07.762 `SceneChanged ⚠回線待機中`、`standby changed owner=auto return_scene=T-A`
- 18:59:25.821 sub が送り直した。18:59:27.674 DOWN→LOW、18:59:36.674 LOW→OK
- 18:59:37.675 `scene switch ⚠回線待機中→T-A reason=ok`。18:59:37.994 `SceneChanged T-A`、`standby changed owner=none`

```
18:58:32.762 cams[main=OK(3,922) sub=OK(2,355)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:58:41.872 cams[main=OK(3,422) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:59:07.248 cams[main=OK(4,158) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main] owner=none return= warnings=0
18:59:07.758 cams[main=OK(3,579) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=DOWN scene_cams=[main,sub] owner=none return= warnings=0
18:59:08.266 cams[main=OK(3,579) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
18:59:36.709 cams[main=OK(3,472) sub=OK(1,975)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
18:59:38.224 cams[main=OK(3,877) sub=OK(2,173)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

## C-5（回線）

(1) 同じ Wi-Fi（main は 192.168.1.11、sub は 192.168.1.17）、T-A、18:59:52〜19:00:52（status 118行）:

- main: 最小 3,279 kbps、最大 4,134 kbps、平均 3,733 kbps。LOW / DOWN に入った回数は 0
- sub: 最小 1,433 kbps、最大 2,667 kbps、平均 2,110 kbps。LOW / DOWN に入った回数は 0
- 2台の kbps の相関係数は 0.14。片方が下がるともう片方が上がる、という様子はなかった
- シーンは60秒間ずっと T-A だった

(2) 別の回線: できなかった（モバイル回線の準備がない）

## C-6（ハードウェアデコード）

T-A、2台とも OK。60秒の間、2秒ごとに CPU 全体と GPU のビデオデコード使用率（`\GPU Engine(*engtype_VideoDecode)\Utilization Percentage` の全インスタンスの合計。Status が正しいサンプルだけ）を同時に読んだ。あわせて obs64 の CPU 時間の増え方を測った。論理コアは16。

- hw_decode=true（2回目、19:03:34〜19:04:35）: CPU 全体の平均 16.4%（最小 9.6、最大 32.1）。obs64 の CPU 時間 15.2秒 / 61秒（論理コア1つ分の 25%）。GPU デコードの平均 13.3%（最小 13.2、最大 13.5）。`reader is too slow` 0件
- hw_decode=false（19:04:45〜19:05:46）: CPU 全体の平均 15.5%（最小 9.1、最大 36.0）。obs64 の CPU 時間 18.8秒 / 61秒（論理コア1つ分の 31%）。GPU デコードの平均 11.7%（最小 9.2、最大 13.5）。`reader is too slow` 0件
- 参考、hw_decode=true の1回目（19:02:14〜19:03:15）: 途中の 19:02:29〜19:03:06 に main が制限で止まったので、使わない。CPU 全体 14.4%、obs64 18%、GPU 10.7%、0件
- 4つのソースの `hw_decode`:
  - on のとき: `IRL`、`cam-main`、`cam-sub`、`メディア` がすべて true
  - off のとき（19:04:35.3 に `ctl.exe hwdecode off`）: `cam-main` と `cam-sub` が false、`IRL` と `メディア` は true のまま
  - 19:05:46.0 に `ctl.exe hwdecode on` で、`cam-main` と `cam-sub` を true に戻した
- 読み方: off にしたのは4つのうち2つ（main と sub を1本ずつ）だけ。`IRL` と `メディア` は main をハードウェアでデコードし続けていたので、GPU デコードの減り方は小さい（13.3%→11.7%）。obs64 の CPU は論理コア1つ分の約6%増えた。CPU 全体の平均は、ばらつきのほうが大きく、差は見えなかった
- 本体のログの `reader is too slow` は、今日の合計が70件で、最後は 17:08:37。今回の試験の間（18:50〜19:06）は0件

## 本体のログ（logs\irl-live-keeper-2026-11-21.log、time= 行。MediaMTX の行は省いた）

```
time=2026-11-21T18:50:07.261+09:00 level=INFO msg="scene changed by user" from=Camera to=T-B
time=2026-11-21T18:52:10.673+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2484.1
time=2026-11-21T18:52:16.674+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3844.1
time=2026-11-21T18:52:19.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2173.8
time=2026-11-21T18:52:26.674+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3470.6
time=2026-11-21T18:52:42.720+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-21T18:52:54.961+09:00 level=INFO msg="scene changed by user" from=T-B to=T-A
time=2026-11-21T18:53:06.673+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:53:06.674+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-21T18:53:06.995+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-21T18:53:29.674+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1943
time=2026-11-21T18:53:38.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2018.8
time=2026-11-21T18:53:39.676+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=sub kbps=2460.6 purpose=AutoRecover
time=2026-11-21T18:53:39.995+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T18:53:50.895+09:00 level=INFO msg="scene changed by user" from=T-A to=T-B
time=2026-11-21T18:54:01.674+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:54:33.674+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2410.7
time=2026-11-21T18:54:42.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2239.2
time=2026-11-21T18:55:15.674+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:55:24.762+09:00 level=INFO msg="scene changed by user" from=T-B to=T-Sub
time=2026-11-21T18:55:24.762+09:00 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-21T18:55:25.095+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-Sub
time=2026-11-21T18:55:38.674+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2369.8
time=2026-11-21T18:55:48.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2311.5
time=2026-11-21T18:55:49.675+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-Sub reason=ok camera=sub kbps=2332.7 purpose=AutoRecover
time=2026-11-21T18:55:49.995+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T18:55:52.861+09:00 level=INFO msg="scene changed by user" from=T-Sub to=T-B
time=2026-11-21T18:56:03.674+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:56:41.674+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2185.6
time=2026-11-21T18:56:50.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2444
time=2026-11-21T18:56:52.595+09:00 level=INFO msg="scene changed by user" from=T-B to=T-A
time=2026-11-21T18:57:05.673+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:57:15.674+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-21T18:57:15.674+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-21T18:57:15.994+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-21T18:57:29.673+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3872.4
time=2026-11-21T18:57:39.674+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3540
time=2026-11-21T18:58:16.673+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1978.7
time=2026-11-21T18:58:26.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2005.5
time=2026-11-21T18:58:27.675+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=sub kbps=2433 purpose=AutoRecover
time=2026-11-21T18:58:27.995+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T18:58:41.674+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-21T18:59:07.455+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-21T18:59:07.762+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-21T18:59:27.674+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1912.4
time=2026-11-21T18:59:36.674+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=1975.3
time=2026-11-21T18:59:37.675+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=sub kbps=2172.7 purpose=AutoRecover
time=2026-11-21T18:59:37.994+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T19:02:29.674+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-21T19:02:29.674+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-21T19:02:29.993+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-21T19:03:06.674+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3950.4
time=2026-11-21T19:03:16.674+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3512
time=2026-11-21T19:03:17.675+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=sub kbps=1904.3 purpose=AutoRecover
time=2026-11-21T19:03:17.995+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-21T19:06:06.515+09:00 level=INFO msg="stream state" streaming=false
time=2026-11-21T19:06:09.028+09:00 level=INFO msg="OBS の program のシーンが削除された。シーンコレクションの切替とみなす"
time=2026-11-21T19:06:11.554+09:00 level=INFO msg="OBS のイベントが途切れた。読み直す"
time=2026-11-21T19:06:11.557+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
```

## obs-cam.txt（シーンと配信の行）

```
18:52:42.642 StreamStateChanged OBS_WEBSOCKET_OUTPUT_STARTING active=false
18:52:42.720 StreamStateChanged OBS_WEBSOCKET_OUTPUT_STARTED active=true
18:52:54.961 SceneChanged T-A
18:53:06.995 SceneChanged ⚠回線待機中
18:53:39.995 SceneChanged T-A
18:53:50.894 SceneChanged T-B
18:55:24.762 SceneChanged T-Sub
18:55:25.095 SceneChanged ⚠回線待機中
18:55:49.994 SceneChanged T-Sub
18:55:52.861 SceneChanged T-B
18:56:52.595 SceneChanged T-A
18:56:54.358 SceneItemEnableStateChanged T-A item=3 enabled=false
18:57:15.994 SceneChanged ⚠回線待機中
18:57:30.144 SceneItemEnableStateChanged T-A item=3 enabled=true
18:58:27.995 SceneChanged T-A
18:58:31.090 SceneItemEnableStateChanged T-A item=3 enabled=false
18:59:07.454 SceneItemEnableStateChanged T-A item=3 enabled=true
18:59:07.762 SceneChanged ⚠回線待機中
18:59:37.994 SceneChanged T-A
19:02:29.993 SceneChanged ⚠回線待機中
```

（ファイルは 19:03:02 で終わっている。下の「気になった動き」を参照）

## 気になった動き、指示書と違ったこと

- **main（Android の RTMP Streamer）は、約5分ごとに送信が止まる。** 送信の始まりと終わり: 18:52:15.5〜18:57:14.1（4分59秒）、18:57:28.0〜19:02:26.9（4分59秒）。ユーザーによれば、アプリの制限による。このため C-4 をやり直し、C-6 の on の測定を一度捨てた。本体は、main が止まるたびに待機へ移り、main が OK に戻ると元のシーンへ戻した（正しい動き）
- **`ctl.exe watch` の出力が、今回も途中で終わっている。** `obs-cam.txt` は 40,960 バイト（4,096 の10倍）で、19:03:02 の行の途中で切れている。指示書には「止めても、それまでの行は残る」とあるが、最後の約3分（19:03:17 の `SceneChanged T-A`、19:06:05 の配信停止の `StreamStateChanged` など）がない。ファイルに書く出力がバッファにたまっていて、止めたときに捨てられたように見える。その間の動きは、本体のログで確かめた
- 回復のログの `camera=` が、回復したカメラを指していないことがある。19:03:17.675 の `scene switch ⚠回線待機中→T-A reason=ok camera=sub` は、main が OK に戻ったこと（19:03:16.674）がきっかけなのに、`camera=sub` と出ている。動きは正しく、ログの表示だけの問題
- カメラが送信をやめてから DOWN と判定されるまでの1〜2秒の間、status は `OK(0)`（状態 OK、kbps 0）を出す。例: 18:53:05.158〜18:53:06.172 の `sub=OK(0)`。DOWN の判定は1秒ごとで、送信をやめてから DOWN までは 1.2〜3.0秒だった
- 本体のログの `scene switch` から OBS の `SceneChanged` までは、どの切替でも約0.3秒だった（T-12 で見た約5秒の差は、今回は出なかった）
- `test restore` で、本体は「OBS の program のシーンが削除された。シーンコレクションの切替とみなす」を出し、OBS を読み直した（約2.5秒）。restore が、program に出ていた `T-A` を、`Camera` に切り替える前に消したように見える。status にも、19:06:09 に `scene=T-A class=unknown` の行があり、19:06:21 に `scene=Camera` になった。害はないが、restore で先にシーンを切り替えてから消すと、本体が読み直さずに済む
- restore のあと、status の warnings に「サブカメラ のメディアソースが OBS に見つかりません」が出ている（`cam-sub` を消したため。試験の前と同じ状態）
- 準備 1 (b) は、ユーザーに聞く代わりに、sub が `path 'sub'` に届いたことで確かめた

## 片づけの結果

- 19:06:05.9 `ctl.exe stream stop`: 成功。3秒後の `streamstatus` は `outputActive false`
- 19:06:09.0 `ctl.exe test restore`: 成功（`試験のシーンとソースを消し、元のプロファイル main とシーン Camera に戻した`）
- `ctl.exe profile`: `main`。準備 6 で控えた名前と同じ
- `ctl.exe scenes`: `シーンコレクション: test用  今のシーン: Camera`。シーンは `Talk`、`Camera2`、`⚠回線待機中`、`Camera`、`Standby` で、`T-` のシーンは消えていた
- `test-original.json`: 消えていた
- 止めたプロセス: 試験用の MediaMTX PID 74528（19:06:18.8、PID で止めた）。`127.0.0.1:21935` の待ち受けはなくなった。裏の見張り2つ（`watch-status.ps1` と `ctl.exe watch`）も止めた
- 止めていないプロセス: 本体 `irl-live-keeper` PID 69624、本体の MediaMTX PID 67672、OBS `obs64` PID 67620
- 試験で作ったファイル: `status-cam.txt`、`obs-cam.txt`、`dest-cam2.out.txt`、`dest-cam2.err.txt`、`cam-result.md`（前回の分として `dest-cam.out.txt`、`dest-cam.err.txt`、`cam-result-attempt1.md`）
