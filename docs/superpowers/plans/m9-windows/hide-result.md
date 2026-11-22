# H-1〜H-8 結果（Windows、隠す・戻す、スマホ2台、2026-11-22）

## 判定のまとめ

回線: 全項目を通して、sub（iOS）は **WAN（モバイル回線）から Tailscale 経由**（`100.118.173.77`）、main（Android）は LAN（`192.168.1.11`）で送った。

- H-1 隠す・戻す: 合格。sub が切れて約2秒で `camera hidden camera=sub items=2 reason=down`。T-A のままで、`T-A` と `T-Sub` の `cam-sub` が `enabled=false`、`hidden=2`、`hidden.json` 2件。sub が OK になって1秒で `camera shown reason=ok`、0件に戻った。ユーザーが OBS の画面で、消えて戻ったことを確かめた
- H-2 全部隠れるシーン: 合格。T-Sub では隠さずに待機へ（owner `auto`、return `T-Sub`）、そのあと隠れた。戻るときは `camera shown` → `scene switch ⚠回線待機中→T-Sub` の順だった
- H-3 人が戻す: 合格。人の代わりに `item T-A cam-sub on` で戻した T-A の項目を、20秒以上隠し直さなかった（`user_shown=True`、`hidden=1`）。sub が OK になると `camera shown items=1 reason=ok`、`user_shown=False`
- H-4 すべて表示に戻す: 合格。`show-all.ps1` の応答は `200 {"ok":true}`、`camera shown items=2 reason=manual_all`。sub が悪いままなので、2.55秒後にまた隠した（指示書の「5秒ほど」より早い）
- H-5 配信の停止: 合格。`stream stop` で `camera shown items=2 reason=stream_stop`。配信を始め直すと、すぐ隠し直した
- H-6 Ctrl+C: 合格。Ctrl+C の直後（status が読めた最後 13:45:02.473 の 0.3秒以内）に `camera shown items=2 reason=shutdown`、`hidden.json` 0件で止まった
- H-7 本体が落ちたとき: 合格。タスクの終了のあと、項目は隠れたまま、`hidden.json` は2件で残った。起動し直した本体は記録を引き継ぎ、sub が OK になって1秒で `camera shown items=2 reason=ok`。落ちた本体の MediaMTX（PID 48220）は残っていなかった
- H-8 Studio Mode: 合格。`studio on` で warnings に「Studio Mode のため、隠す動きの代わりに待機シーンへ送っています」。sub を止めると、隠さずに待機へ（return `T-A`）、OK で T-A へ戻った。`studio off` のあと warnings は空になった

## 準備

- 本体: `irl-live-keeper.exe`（2026-11-21 22:38 版）、PID 38028、2026-11-22 13:32:54 に起動。status の `cameras`: `main on_bad=standby`、`sub on_bad=hide`。`config.yaml` に `studio_mode_hide` の行はない（行数だけ数えた。中身は表示していない）
- OBS: `obs64` PID 36200。配信なし、Studio Mode オフ、プロファイル `main`、コレクション `test用`
- 13:34:21.8 試験用の MediaMTX（`dest.yml`）を起動した（PID 36380）。`127.0.0.1:21935` の待ち受けを確かめた
- 13:34:24.5 `ctl.exe test setup`: 成功
  ```
  元のプロファイル: main、シーンコレクション: test用、シーン: Camera（test-original.json に残した）
  試験のプロファイル IRLKeeper-Test（配信先 rtmp://127.0.0.1:21935/test）に切り替え、シーンコレクション test用 に試験のシーンを足した
  シーン: T-A（cam-main と cam-sub）、T-B（cam-main）、T-Sub（cam-sub）。待機シーンは ⚠回線待機中。今のシーンは T-B
  ```
- 見張り: `watch-status.ps1 -Log .\status-hide.txt` と `ctl.exe watch 3600 .\obs-hide.txt`（ファイルを引数で渡した。今回は最後まで残った）
- 13:36:09.9 sub が Tailscale から接続（`100.118.173.77`）、13:36:16.8 main が LAN から接続（`192.168.1.11`）。13:36:30 に2台とも OK。`ctl.exe inputs` を2回読み、4つのソースが `PLAYING` で cursor が約3秒進んでいた
- 準備 1 (b) は、sub が `path 'sub'` に届いたことで確かめた
- 13:36:40.9 `profile` が `IRLKeeper-Test` であることを確かめてから、`scene T-B` → `stream start`。13:36:44.3 `scene T-A`（H-1 へ）

## H-1 隠す・戻す（T-A、sub は Tailscale）

- 13:36:54.793 sub の接続が切れた（`[conn 100.118.173.77] closed`）
- 13:36:56.821 `camera changed sub OK→DOWN`、13:36:56.824 `camera hidden camera=sub items=2 reason=down`（OBS: `T-A item=3` と `T-Sub item=2` が `enabled=false`）
- 隠れたあと: `items T-A` → `cam-sub enabled=false`、`items T-Sub` → `cam-sub enabled=false`。status `cameras[sub].hidden=2`。`hidden.json` 2件（`camera=sub item=3`、`camera=sub item=2`、`source=121c5aa6-…`）。シーンは T-A のまま、owner `none`
- ユーザーが OBS の画面で非表示を確かめた
- 13:37:36.2 sub が送り直した。13:37:50.821 OK、13:37:51.832 `camera shown camera=sub items=2 reason=ok`。ユーザーが表示に戻ったことを確かめた
- 戻ったあと: 両方 `enabled=true`、`hidden=0`、`hidden.json` 0件

```
13:36:56.426 cams[main=OK(4,356) sub=OK(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
13:36:56.936 cams[main=OK(3,442) sub=DOWN(0)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
13:37:41.135 cams[main=OK(3,537) sub=LOW(1,810)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
13:37:51.248 cams[main=OK(3,346) sub=OK(2,349)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

## H-2 全部隠れるシーン（T-Sub、sub は Tailscale）

- 13:38:29.5 `scene T-Sub`（2台 OK）
- 13:38:39.821 sub OK→DOWN。13:38:39.822 `scene switch T-Sub→⚠回線待機中 reason=down camera=sub`。13:38:40.156 `standby changed owner=auto return_scene=T-Sub`。13:38:40.158 `camera hidden items=2 reason=down`（待機に入ってから隠した）
- 止めたあと: 両方 `enabled=false`、`hidden=2`、`hidden.json` 2件
- 13:39:00.4 sub が送り直した。13:39:21.821 OK
- 13:39:22.822 `camera shown items=2 reason=ok`、同じ時刻（同じミリ秒）に `scene switch ⚠回線待機中→T-Sub reason=ok`。OBS のイベントの順も `SceneItemEnableStateChanged … enabled=true`（13:39:22.822）→ `SceneChanged T-Sub`（13:39:23.156）
- 戻ったあと: 両方 `enabled=true`、`hidden.json` 0件

```
13:38:30.182 cams[main=OK(3,872) sub=OK(2,244)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=none return= warnings=0
13:38:40.306 cams[main=OK(4,043) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-Sub warnings=0
13:39:22.131 cams[main=OK(4,079) sub=OK(1,961)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-Sub warnings=0
13:39:23.655 cams[main=OK(3,408) sub=OK(2,080)] streaming=True scene=T-Sub class=monitored health=OK scene_cams=[sub] owner=none return= warnings=0
```

## H-3 人が戻す（T-A、sub は Tailscale）

- 制限の前に main を送り直してもらった（13:39:42 止め → 13:40:12 送り直し。T-Sub の間なので、main が DOWN でもシーンは変わらなかった）
- 13:40:24.7 `scene T-A`（2台 OK）
- 13:40:35.821 sub OK→DOWN、13:40:35.824 `camera hidden items=2 reason=down`
- 13:40:48.9 `ctl.exe item T-A cam-sub on`（人の操作の代わり）。本体のログ 13:40:48.942 `hidden item dropped camera=sub scene=T-A item=3 state=H`
- 戻した直後と20秒後（13:41:09）: `items T-A` → `cam-sub enabled=true`、`items T-Sub` → `cam-sub enabled=false`。status `hidden=1 user_shown=True`。`hidden.json` 1件（`item=2`）。T-A のまま、隠し直さなかった
- 13:41:19.4 sub が送り直した。13:41:45.821 OK、13:41:45.822 `camera shown items=1 reason=ok`
- そのあと: 両方 `enabled=true`、`hidden=0`、`user_shown=False`、`hidden.json` 0件

## H-4 すべて表示に戻す（T-A、sub は Tailscale）

- 13:42:05.821 sub OK→DOWN、13:42:05.824 `camera hidden items=2 reason=down`
- 13:42:16.256 `show-all.ps1` → `show-all: 200 {"ok":true}`
- 13:42:16.273 `camera shown items=2 reason=manual_all`。直後: 両方 `enabled=true`、`hidden=0`、`hidden.json` 0件
- 13:42:18.824 `camera hidden items=2 reason=down`（2.55秒後に隠し直した）。約10秒後: 両方 `enabled=false`、`hidden=2`、`hidden.json` 2件
- このあと、送り直してもらう間に main が止まった（13:42:37.8。制限の前に送り直してもらったため）。13:42:38.822 待機へ（`camera=main`、return `T-A`）
- 13:42:57.821 sub OK、13:42:58.822 `camera shown items=2 reason=ok`（待機の間に表示に戻った）。13:43:02.821 main OK、13:43:03.822 T-A へ

## H-5 配信の停止（T-A、sub は Tailscale）

- 13:43:22.821 sub OK→DOWN、13:43:22.823 `camera hidden items=2 reason=down`
- 13:43:39.8 `ctl.exe stream stop`。13:43:40.638 `stream state streaming=false`、13:43:40.639 `camera shown items=2 reason=stream_stop`。そのあと: 両方 `enabled=true`、`hidden=0`、`hidden.json` 0件
- 13:43:50.6 `profile` が `IRLKeeper-Test` であることを確かめてから `stream start`。13:43:50.717 `streaming=true`、13:43:50.719 `camera hidden items=2 reason=down`（2件）
- 13:44:01.5 sub が送り直した。13:44:22.821 OK、13:44:23.823 `camera shown items=2 reason=ok`、0件

## H-6 Ctrl+C（T-A、sub は Tailscale）

- 13:44:42.821 sub OK→DOWN、13:44:42.823 `camera hidden items=2 reason=down`（`hidden.json` 2件）
- ユーザーが本体の窓で Ctrl+C を押した。status が読めた最後は 13:45:02.473、13:45:02.750 に `camera shown camera=sub items=2 reason=shutdown`（OBS: 13:45:02.749 / 13:45:02.750 に `enabled=true`）
- 止まったあと（13:45:18）: 両方 `enabled=true`。`hidden.json` は 13:45:02 に書かれ、0件。本体（PID 38028）と本体の MediaMTX（PID 38308）は消えていた。8787 と 8554 の待ち受けはなかった
- 13:45:48 ユーザーが本体を起動し直した（PID 48668、MediaMTX PID 48220）。すぐ OBS につながり、2台とも DOWN なので、同じ時刻に待機へ（`camera=main`、return `T-A`）、sub を隠した（`camera hidden items=2 reason=down`）
- 2台が送り直し、13:46:37.431 `camera shown reason=ok`、13:46:42.431 T-A へ

## H-7 本体が落ちたとき（T-A、sub は Tailscale）

- 13:46:56.429 sub OK→DOWN、13:46:56.432 `camera hidden items=2 reason=down`（`hidden.json` 2件）
- ユーザーがタスクマネージャーで `irl-live-keeper.exe`（PID 48668）を終了した。status が読めた最後は 13:47:37.201
- 終了したあと（13:47:53）: `items T-A` → `cam-sub enabled=false`、`items T-Sub` → `cam-sub enabled=false`（隠れたまま残った）。`hidden.json` 2件。本体のログに `camera shown` は出ていない。**本体の MediaMTX（PID 48220）は残っていなかった**（`Get-Process mediamtx` は試験用の PID 36380 だけ）。OBS は配信中、シーンは T-A のまま
- 13:48:17 ユーザーが本体を起動し直した（PID 50184、MediaMTX PID 51068）。13:48:18.195 OBS につながり、待機へ（main が DOWN のため。return `T-A`）。このとき `camera hidden` は出ていない（記録の2件を引き継いだ）
- 13:48:23.6 sub が送り直した。13:48:37.187 OK、13:48:38.189 `camera shown camera=sub items=2 reason=ok`。両方 `enabled=true`、`hidden.json` 0件
- 13:48:58 main が送り直した。13:49:10 OK、13:49:11 T-A へ

## H-8 Studio Mode（T-A、sub は Tailscale）

- 13:49:11.7 `ctl.exe studio on`（`studioMode true`）。status `studio_mode=True`、warnings `[Studio Mode のため、隠す動きの代わりに待機シーンへ送っています]`
- 13:49:22.188 sub OK→DOWN、同じ時刻に `scene switch T-A→⚠回線待機中 reason=down camera=sub`。13:49:22.522 `standby changed owner=auto return_scene=T-A`。`camera hidden` は出ず、両方 `enabled=true`、`hidden=0`、`hidden.json` 0件
- 13:49:43.7 sub が送り直した。13:49:57.187 OK、13:49:58.189 `scene switch ⚠回線待機中→T-A reason=ok`、13:49:58.528 owner `none`
- 13:50:02.0 `ctl.exe studio off`（`studioMode false`）。3秒後の status `studio_mode=False`、warnings 空

```
13:49:14.182 cams[main=OK(3,882) sub=OK(2,354)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=1
13:49:22.654 cams[main=OK(3,619) sub=DOWN(0)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=1
13:49:58.650 cams[main=OK(3,463) sub=OK(2,360)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=1
13:50:05.398 cams[main=OK(3,813) sub=OK(1,941)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

## hidden.json の件数の変化

- 準備: ファイルなし（`state\` もなし）
- H-1: 隠れたあと 2件 → 戻ったあと 0件
- H-2: 待機のあと 2件 → 戻ったあと 0件
- H-3: 隠れたあと 2件 → T-A を人が戻したあと 1件（T-Sub の分）→ sub OK で 0件
- H-4: 隠れたあと 2件 → show-all 直後 0件 → 隠し直して 2件 → sub OK で 0件
- H-5: 隠れたあと 2件 → stream stop で 0件 → stream start で 2件 → sub OK で 0件
- H-6: 隠れたあと 2件 → Ctrl+C で 0件 → 起動し直しで 2件（2台 DOWN のため）→ sub OK で 0件
- H-7: 隠れたあと 2件 → 落ちたあとも 2件 → 起動し直し、sub OK で 0件
- H-8: ずっと 0件

## 本体のログ（logs\irl-live-keeper-2026-11-22.log、time= 行。MediaMTX の行は省いた）

```
time=2026-11-22T13:33:07.834+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T13:34:27.956+09:00 level=INFO msg="scene changed by user" from=Camera to=T-B
time=2026-11-22T13:36:14.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2017.9
time=2026-11-22T13:36:20.821+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3765.2
time=2026-11-22T13:36:23.822+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2261
time=2026-11-22T13:36:29.821+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3932.8
time=2026-11-22T13:36:41.019+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-22T13:36:44.323+09:00 level=INFO msg="scene changed by user" from=T-B to=T-A
time=2026-11-22T13:36:56.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:36:56.824+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:37:40.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1809.9
time=2026-11-22T13:37:50.821+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2349
time=2026-11-22T13:37:51.832+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:38:29.823+09:00 level=INFO msg="scene changed by user" from=T-A to=T-Sub
time=2026-11-22T13:38:39.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:38:39.822+09:00 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-22T13:38:40.156+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-Sub
time=2026-11-22T13:38:40.158+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:39:04.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2518.9
time=2026-11-22T13:39:21.821+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=1960.5
time=2026-11-22T13:39:22.822+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:39:22.822+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-Sub reason=ok camera=sub kbps=2080.5 purpose=AutoRecover
time=2026-11-22T13:39:23.156+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T13:39:43.822+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T13:40:13.821+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3586.4
time=2026-11-22T13:40:23.821+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3343.7
time=2026-11-22T13:40:25.090+09:00 level=INFO msg="scene changed by user" from=T-Sub to=T-A
time=2026-11-22T13:40:35.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:40:35.824+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:40:48.942+09:00 level=INFO msg="hidden item dropped" camera=sub scene=T-A item=3 state=H
time=2026-11-22T13:41:23.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2107
time=2026-11-22T13:41:45.821+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2252.9
time=2026-11-22T13:41:45.822+09:00 level=INFO msg="camera shown" camera=sub items=1 reason=ok
time=2026-11-22T13:42:05.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:42:05.824+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:42:16.273+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=manual_all
time=2026-11-22T13:42:18.824+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:42:38.821+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T13:42:38.822+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T13:42:39.156+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T13:42:48.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1810.5
time=2026-11-22T13:42:53.821+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3210.1
time=2026-11-22T13:42:57.821+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2277.2
time=2026-11-22T13:42:58.822+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:43:02.821+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3950.2
time=2026-11-22T13:43:03.822+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3420 purpose=AutoRecover
time=2026-11-22T13:43:04.157+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T13:43:22.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:43:22.823+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:43:40.638+09:00 level=INFO msg="stream state" streaming=false
time=2026-11-22T13:43:40.639+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=stream_stop
time=2026-11-22T13:43:50.717+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-22T13:43:50.719+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:44:05.821+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2178.1
time=2026-11-22T13:44:22.821+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=1879.8
time=2026-11-22T13:44:23.823+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:44:42.821+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:44:42.823+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:45:02.750+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=shutdown
time=2026-11-22T13:45:48.436+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T13:45:48.436+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-22T13:45:48.436+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T13:45:48.445+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:45:48.756+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T13:46:27.430+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2159.6
time=2026-11-22T13:46:31.430+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3895
time=2026-11-22T13:46:32.431+09:00 level=INFO msg="media source refreshed" camera=sub source=cam-sub
time=2026-11-22T13:46:36.430+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2040.4
time=2026-11-22T13:46:36.431+09:00 level=INFO msg="media source refreshed" camera=main source=IRL
time=2026-11-22T13:46:36.431+09:00 level=INFO msg="media source refreshed" camera=main source=メディア
time=2026-11-22T13:46:36.431+09:00 level=INFO msg="media source refreshed" camera=main source=cam-main
time=2026-11-22T13:46:37.431+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:46:41.430+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3324.5
time=2026-11-22T13:46:42.431+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=4114 purpose=AutoRecover
time=2026-11-22T13:46:42.756+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T13:46:56.429+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:46:56.432+09:00 level=INFO msg="camera hidden" camera=sub items=2 reason=down
time=2026-11-22T13:48:18.195+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T13:48:18.195+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-22T13:48:18.195+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T13:48:18.523+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T13:48:28.188+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2465.7
time=2026-11-22T13:48:33.188+09:00 level=INFO msg="media source refreshed" camera=sub source=cam-sub
time=2026-11-22T13:48:37.187+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2176.9
time=2026-11-22T13:48:38.189+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T13:49:01.187+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=4373.6
time=2026-11-22T13:49:06.188+09:00 level=INFO msg="media source refreshed" camera=main source=cam-main
time=2026-11-22T13:49:06.189+09:00 level=INFO msg="media source refreshed" camera=main source=IRL
time=2026-11-22T13:49:06.189+09:00 level=INFO msg="media source refreshed" camera=main source=メディア
time=2026-11-22T13:49:10.188+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3740.1
time=2026-11-22T13:49:11.189+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3818.2 purpose=AutoRecover
time=2026-11-22T13:49:11.522+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T13:49:22.188+09:00 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
time=2026-11-22T13:49:22.188+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
time=2026-11-22T13:49:22.522+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T13:49:48.188+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2316.1
time=2026-11-22T13:49:57.187+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2285.6
time=2026-11-22T13:49:58.189+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=sub kbps=2359.8 purpose=AutoRecover
time=2026-11-22T13:49:58.528+09:00 level=INFO msg="standby changed" owner=none return_scene=""
time=2026-11-22T13:50:02.091+09:00 level=INFO msg="scene changed by user" from=T-A to=⚠回線待機中
time=2026-11-22T13:50:02.101+09:00 level=INFO msg="scene changed by user" from=⚠回線待機中 to=T-A
time=2026-11-22T13:50:15.491+09:00 level=INFO msg="stream state" streaming=false
time=2026-11-22T13:50:18.090+09:00 level=INFO msg="scene changed by user" from=T-A to=Camera
```

## 気になった動き、指示書と違ったこと

- **`studio off` のときに、本体が「人が切り替えた」と2回受け取った。** 13:50:02.091 `scene changed by user T-A→⚠回線待機中`、13:50:02.101 `⚠回線待機中→T-A`（OBS の `SceneChanged` も同じ2つ）。Studio Mode を切ると、OBS が一瞬 program を別のシーンにしてから戻すように見える（OBS の動き。なぜ `⚠回線待機中` だったのかはわからない）。今回は T-A、owner `none` で何も起きなかった。しかし、待機中（owner `auto`）に `studio off` すると、owner が `none` になり、自動で戻らなくなるかもしれない（確かめていない）
- **人が戻した項目は、sub が悪いまま映っている。** H-3 で T-A の `cam-sub` を表示に戻したあと、sub が DOWN の間、T-A のまま待機へも行かなかった。status の health も OK のまま。「人が戻した＝映してよい」という扱いなら期待どおり。待機へ送るべきかどうかは、作る人の判断
- **隠している間の status。** sub を隠している間、status の `scene_cams` は `[main,sub]`、`health=OK` のまま（例: 13:36:56.936）。C-4 で ctl が手で隠したときは `scene_cams=[main]` だった。本体が自分で隠した項目は数え方が違うように見える。表示の上だけのことかもしれない
- H-4 で、show-all のあと隠し直すまでは 2.55秒だった（指示書の期待は「5秒ほど」）
- H-7 で、起動し直した本体が `hidden.json` を引き継いだときに、それを示すログの行がなかった（`camera hidden` も出ない。表示に戻したときの `camera shown reason=ok` だけ）。「記録から照合した」1行があると、あとで追いやすい
- 本体を起動し直した直後（H-6、H-7）、OBS につながったのと同じ時刻に待機へ移り、sub を隠した（2台とも、本体の MediaMTX が止まったのに合わせて接続が切れていたため）。待つ時間（猶予）はなかった
- 新しい行 `media source refreshed`（LOW→OK の途中や OK になったときに、そのカメラのメディアソースを読み直している）が出た
- main（Android の無料の版）が約5分で止まるので、H-3、H-4 の前に送り直してもらった。H-4 の終わりに main が止まった間は、main の不調として待機へ移った（`on_bad: standby` のとおり）
- `test restore` は、今回は先に `Camera` へ切り替えてから消した（本体のログ 13:50:18.090 `scene changed by user T-A→Camera`）。前回（C-1〜C-7）の「program のシーンが削除された」は出なかった
- `ctl.exe watch 3600 .\obs-hide.txt`（ファイルを引数で渡す形）は、止めたあとも最後の行（13:50:19.600 `CurrentProfileChanged main`）まで残っていた
- 本体を止めていた間（13:45:02〜13:45:48 と 13:47:37〜13:48:17）、`watch-status.ps1` は「状態を読めない: The operation has timed out.」を34行出した

## 片づけの結果

- 13:50:14.5 `ctl.exe studio off`（`studioMode false`）
- 13:50:14.6 `ctl.exe stream stop`: 成功。3秒後 `outputActive false`
- 13:50:17.7 `ctl.exe test restore`: 成功（`試験のシーンとソースを消し、元のプロファイル main とシーン Camera に戻した`）
- `ctl.exe profile`: `main`。準備 6 で控えた名前と同じ
- `ctl.exe scenes`: `シーンコレクション: test用  今のシーン: Camera`。`T-` のシーンは消えていた（`Talk`、`Camera2`、`Camera`、`⚠回線待機中`、`Standby`）
- `test-original.json`: 消えていた
- 止めたプロセス: 試験用の MediaMTX PID 36380（13:50:22.7、PID で止めた）。`127.0.0.1:21935` の待ち受けはなくなった。裏の見張り2つ（`watch-status.ps1` と `ctl.exe watch`）も止めた
- 止めていないプロセス: 本体 `irl-live-keeper` PID 50184、本体の MediaMTX PID 51068、OBS `obs64` PID 36200
- 試験で作ったファイル: `status-hide.txt`、`obs-hide.txt`、`dest-hide.out.txt`、`dest-hide.err.txt`、`hide-result.md`。本体の `state\hidden.json` は0件
