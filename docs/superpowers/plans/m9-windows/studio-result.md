# S-1〜S-5 の確認結果（Studio Mode を切るときの一瞬の切替、F-9）

- 実施日: 2026-11-22 15:07〜15:16
- 本体: 新しい `irl-live-keeper.exe`（2026-11-22 14:35:02、14,135,808 バイト）。ユーザーが入れ替えのあと起動し直した（15:07:42 起動、PID 55660。S-5 で終了・再起動したあとは PID 61340）
- `ctl.exe`: 2026-11-22 14:36:05、11,385,856 バイト（`preview` の命令あり）
- `config.yaml`: 前回のまま（`main` は `on_bad: standby`、`sub` は `on_bad: hide`、`studio_mode_hide` は書かない）とユーザーから聞いた。ファイルは表示していない
- 回線: main（Android）は LAN（`192.168.1.11`）。sub（iOS）は全項目で キャリア回線から Tailscale（`100.118.173.77`）
- 見張りのファイル: `status-studio.txt`（本体の status、0.5秒ごと）、`obs-studio.txt`（OBS のイベント、最後の `CurrentProfileChanged main` まで残っていた）

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| S-1 | 合格 | preview を T-Sub にして `studio off`。`studio mode off bounce ignored scenes="[T-Sub ⚠回線待機中]"`、`scene changed by user` は出ず、`⚠回線待機中 owner=auto return=T-A` のまま。main が OK になると `AutoRecover` で T-A へ戻った。`warnings` は期待どおりの文 |
| S-2 | 合格 | preview を ⚠回線待機中 にして `studio off`。`scenes="[⚠回線待機中 ⚠回線待機中]"`。持ち主と戻り先は消えず、T-A へ戻った |
| S-3 | 合格 | preview を T-B にして `studio off`。`scenes="[T-B ⚠回線待機中]"`。戻り先は T-A のままで、main が戻ると T-B でなく T-A へ戻った |
| S-4 | 合格 | Studio Mode の中の `scene T-B`、`scene T-A` が `scene changed by user` として2回届いた。`studio off` では `bounce ignored scenes="[T-B T-A]"` が出て、user の行は増えなかった |
| S-5 | 合格 | 落ちたあとの起動のログの最初の行が `隠した項目の記録を読んだ（OBS につないだら照合する） items=2`。sub が OK になると `camera shown camera=sub items=2 reason=ok`、`hidden.json` は0件になった |

## 準備

- 試験だけの MediaMTX（`dest.yml`）: PID 54712、`127.0.0.1:21935` で待ち受けを確かめた
- `ctl.exe scenes` の1行目は `test用`。`test setup` の出力: 元のプロファイル `main`、シーンコレクション `test用`、シーン `Camera`。試験のプロファイル `IRLKeeper-Test` に切り替え、`T-A`、`T-B`、`T-Sub` を足した
- 15:10:28 に2台とも OK。`inputs` で `cam-main` と `cam-sub` が `PLAYING`、cursor が進むことを2回読んで確かめた
- `profile` が `IRLKeeper-Test` であることを確かめてから `scene T-B` → `stream start`（15:10:47 `streaming=True scene=T-B health=OK owner=none`）→ `scene T-A`

## S-1（preview T-Sub）

流れ:

- 15:10:55.095 `studio on`（OBS 15:10:55.147 `StudioModeStateChanged enabled=true`）
- `-Once` の `warnings`: 「屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きの代わりに待機シーンへ送っています」
- ユーザーに main を止めてもらう → 15:11:06.892 `AutoFail`、待機へ
- 15:11:29.624 `preview T-Sub`（`ctl.exe preview` で `T-Sub` を確かめた）
- 15:11:29.674 `studio off`
- ユーザーに main を送り直してもらう（15:11:51 接続 `192.168.1.11`）→ 15:12:05.893 `AutoRecover`

`studio off` の前後の status:

```
15:11:29.435 cams[main=DOWN(0) sub=OK(2,080)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=1
（15:11:29.674 studio off）
15:11:29.940 cams[main=DOWN(0) sub=OK(2,092)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
15:11:32.466 cams[main=DOWN(0) sub=OK(2,022)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
...
15:12:06.359 cams[main=OK(3,985) sub=OK(2,089)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

OBS のイベント:

```
15:10:55.147 StudioModeStateChanged enabled=true
15:11:07.201 SceneChanged ⚠回線待機中
15:11:29.624 PreviewSceneChanged T-Sub
15:11:29.715 SceneChanged T-Sub
15:11:29.722 SceneChanged ⚠回線待機中
15:11:29.727 StudioModeStateChanged enabled=false
15:12:06.201 SceneChanged T-A
```

本体のログ:

```
time=2026-11-22T15:11:06.892+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T15:11:06.892+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T15:11:07.451+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T15:11:29.727+09:00 level=INFO msg="studio mode off bounce ignored" scenes="[T-Sub ⚠回線待機中]"
time=2026-11-22T15:11:54.891+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3681.1
time=2026-11-22T15:12:04.892+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3480.8
time=2026-11-22T15:12:05.893+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3984.9 purpose=AutoRecover
time=2026-11-22T15:12:06.201+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

`studio off` のあと `warnings` は空になった（`studio_mode=False`）。

## S-2（preview ⚠回線待機中）

流れ:

- 15:12:13.402 `studio on`。`warnings` は S-1 と同じ文
- ユーザーに main を止めてもらう → 15:12:27.892 `AutoFail`
- 15:12:41.265 `preview ⚠回線待機中`（確かめた）
- 15:12:41.314 `studio off`
- main を送り直してもらう（15:12:57 接続 `192.168.1.11`）→ 15:13:10.893 `AutoRecover`

`studio off` の前後の status:

```
15:12:41.055 cams[main=DOWN(0) sub=OK(2,118)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=1
（15:12:41.314 studio off）
15:12:41.558 cams[main=DOWN(0) sub=OK(2,118)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
15:12:44.085 cams[main=DOWN(0) sub=OK(2,303)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
...
15:13:11.415 cams[main=OK(3,966) sub=OK(2,156)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

OBS のイベント:

```
15:12:13.454 StudioModeStateChanged enabled=true
15:12:28.201 SceneChanged ⚠回線待機中
15:12:41.265 PreviewSceneChanged ⚠回線待機中
15:12:41.354 SceneChanged ⚠回線待機中
15:12:41.357 SceneChanged ⚠回線待機中
15:12:41.366 StudioModeStateChanged enabled=false
15:13:11.201 SceneChanged T-A
```

本体のログ:

```
time=2026-11-22T15:12:27.891+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T15:12:27.892+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T15:12:28.452+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T15:12:41.366+09:00 level=INFO msg="studio mode off bounce ignored" scenes="[⚠回線待機中 ⚠回線待機中]"
time=2026-11-22T15:13:00.892+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=4720.2
time=2026-11-22T15:13:09.892+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3323.7
time=2026-11-22T15:13:10.893+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3966 purpose=AutoRecover
time=2026-11-22T15:13:11.201+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

## S-3（preview T-B）

流れ:

- 15:13:16.613 `studio on`。`warnings` は S-1 と同じ文
- ユーザーに main を止めてもらう → 15:13:28.892 `AutoFail`
- 15:13:39.856 `preview T-B`（確かめた）
- 15:13:39.903 `studio off`
- main を送り直してもらう（15:13:56 接続 `192.168.1.11`）→ 15:14:09.893 `AutoRecover` で **T-A** へ

`studio off` の前後の status:

```
15:13:39.765 cams[main=DOWN(0) sub=OK(2,331)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=1
（15:13:39.903 studio off）
15:13:40.274 cams[main=DOWN(0) sub=OK(2,165)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
15:13:42.811 cams[main=DOWN(0) sub=OK(2,088)] streaming=True scene=⚠回線待機中 class=standby health=- scene_cams=[] owner=auto return=T-A warnings=0
...
15:14:10.621 cams[main=OK(3,405) sub=OK(2,069)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

OBS のイベント:

```
15:13:16.669 StudioModeStateChanged enabled=true
15:13:29.201 SceneChanged ⚠回線待機中
15:13:39.856 PreviewSceneChanged T-B
15:13:39.943 SceneChanged T-B
15:13:39.950 SceneChanged ⚠回線待機中
15:13:39.955 StudioModeStateChanged enabled=false
15:14:10.200 SceneChanged T-A
```

本体のログ:

```
time=2026-11-22T15:13:28.891+09:00 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
time=2026-11-22T15:13:28.892+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T15:13:29.451+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T15:13:39.955+09:00 level=INFO msg="studio mode off bounce ignored" scenes="[T-B ⚠回線待機中]"
time=2026-11-22T15:13:59.891+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=3691.4
time=2026-11-22T15:14:08.892+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3773.6
time=2026-11-22T15:14:09.893+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3404.6 purpose=AutoRecover
time=2026-11-22T15:14:10.200+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

## S-4（Studio Mode の中の人の切替）

流れ:

- 15:14:19 開始時の status は `scene=T-A owner=none`。2台とも OK
- `studio on` → 15:14:20.461 `scene T-B` → 2秒 → 15:14:22.522 `scene T-A` → 15:14:23.623 `studio off`
- `studio off` の直前の preview は `T-B`。OBS が program を入れ替えたときに、前の program を preview に移した

status:

```
15:14:22.261 cams[main=OK(3,312) sub=OK(2,150)] streaming=True scene=T-B class=monitored health=OK scene_cams=[main] owner=none return= warnings=1
15:14:23.269 cams[main=OK(3,962) sub=OK(2,116)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=1
（15:14:23.623 studio off）
15:14:23.786 cams[main=OK(3,962) sub=OK(2,116)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
15:14:26.314 cams[main=OK(3,266) sub=OK(2,086)] streaming=True scene=T-A class=monitored health=OK scene_cams=[main,sub] owner=none return= warnings=0
```

OBS のイベント:

```
15:14:19.450 StudioModeStateChanged enabled=true
15:14:20.835 SceneChanged T-B
15:14:22.867 PreviewSceneChanged T-B
15:14:22.874 SceneChanged T-A
15:14:23.669 SceneChanged T-B
15:14:23.681 SceneChanged T-A
15:14:23.688 StudioModeStateChanged enabled=false
```

本体のログ:

```
time=2026-11-22T15:14:21.085+09:00 level=INFO msg="scene changed by user" from=T-A to=T-B
time=2026-11-22T15:14:23.124+09:00 level=INFO msg="scene changed by user" from=T-B to=T-A
time=2026-11-22T15:14:23.688+09:00 level=INFO msg="studio mode off bounce ignored" scenes="[T-B T-A]"
```

## S-5（落ちたあとの起動のログ）

流れ:

- 15:14:26 `scene=T-A`、2台とも OK
- ユーザーに sub を止めてもらう → 15:14:35.894 `camera hidden camera=sub items=2 reason=down`
  - T-A のまま
  - `T-A` と `T-Sub` の `cam-sub` が `enabled=false`
  - status の `cameras[sub].hidden=2`
  - `hidden.json` は2件（`camera=sub item=2` と `item=3`、同じ source）
- ユーザーがタスクマネージャーで本体（PID 55660）を終了
  - 15:15:23 に確かめた: 本体が起動した MediaMTX（PID 55944）も残っていない
  - 項目は隠れたまま（`cam-sub enabled=false`）。`hidden.json` は 834 バイトのまま
- ユーザーが本体を起動し直した（15:15:30、PID 61340、MediaMTX は PID 60476）
- ユーザーに sub と main を送り直してもらう
  - 15:15:57 sub 接続（`100.118.173.77`）
  - 15:16:02 main 接続（`192.168.1.11`）
- 15:16:12.440 `camera shown camera=sub items=2 reason=ok`
- 15:16:16.440 `AutoRecover` で T-A へ

本体のログ（起動し直したあと）:

```
time=2026-11-22T15:15:30.437+09:00 level=INFO msg="隠した項目の記録を読んだ（OBS につないだら照合する）" items=2
time=2026-11-22T15:15:30.445+09:00 level=INFO msg="OBS に接続した" addr=127.0.0.1:4455
time=2026-11-22T15:15:30.445+09:00 level=INFO msg="stream state" streaming=true
time=2026-11-22T15:15:30.445+09:00 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
time=2026-11-22T15:15:30.769+09:00 level=INFO msg="standby changed" owner=auto return_scene=T-A
time=2026-11-22T15:16:02.438+09:00 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2085.9
time=2026-11-22T15:16:05.438+09:00 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=4228.9
time=2026-11-22T15:16:07.439+09:00 level=INFO msg="media source refreshed" camera=sub source=cam-sub
time=2026-11-22T15:16:10.441+09:00 level=INFO msg="media source refreshed" camera=main source=メディア
time=2026-11-22T15:16:10.441+09:00 level=INFO msg="media source refreshed" camera=main source=IRL
time=2026-11-22T15:16:10.441+09:00 level=INFO msg="media source refreshed" camera=main source=cam-main
time=2026-11-22T15:16:11.438+09:00 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2028.3
time=2026-11-22T15:16:12.440+09:00 level=INFO msg="camera shown" camera=sub items=2 reason=ok
time=2026-11-22T15:16:15.438+09:00 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=3916
time=2026-11-22T15:16:16.440+09:00 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=3715.8 purpose=AutoRecover
time=2026-11-22T15:16:16.767+09:00 level=INFO msg="standby changed" owner=none return_scene=""
```

項目のあと（15:16:17）:

- `T-A` と `T-Sub` の `cam-sub` が `enabled=true`
- `hidden=0`、`user_shown=False`
- `hidden.json` は0件（84 バイト）

## hidden.json の件数

- S-1〜S-4: 0件（sub は OK のまま）
- S-5 で sub が隠れたあと: 2件
- 本体が落ちたあと: 2件のまま
- sub が OK になったあと: 0件

## 気になった動き、指示書と違ったこと

1. 指示書のとおりに動いた。4回の `studio off` すべてで `studio mode off bounce ignored` が1行出て、`scene changed by user` は出なかった。この試験の間の `scene changed by user` は、次の5回だけだった（15:07:40 以降）。
   - 準備の `scene T-B`（Camera→T-B）
   - 準備の `scene T-A`（T-B→T-A）
   - S-4 の2回
   - 片づけの `test restore`（T-A→Camera）
2. S-4 では、`studio off` の直前の preview が `T-B` になっていた。Studio Mode の中で `scene T-A` を送ると、OBS が前の program（T-B）を preview へ移す（`PreviewSceneChanged T-B`）。そのため、`bounce ignored` の `scenes` は `[T-B T-A]` になった。人の切替ではない一瞬の切替を、正しく捨てている。
3. S-5 で起動し直した直後（15:15:30）、本体は main を DOWN と見て待機へ送った（`AutoFail`、return `T-A`）。本体が落ちると、本体の MediaMTX も一緒に終わり（Job Object）、main と sub の送信も切れる。そのため、送り直すまで main が来ないのは自然。H-7 と同じ動き。2台を送り直すと `camera shown` と `AutoRecover` で元に戻った。
4. S-5 の `hidden.json` の照合で、項目を表示に戻したのは、sub が OK になったとき（`reason=ok`）。起動して OBS につないだ時点では、sub が DOWN のため戻さなかった。期待どおり。
5. 15:07:40 に `MediaMTX が終了しました error="exit status 0xc000013a"` の WARN が1行ある。ユーザーが試験の前に本体を起動し直したときの、前の本体の MediaMTX の終了。この試験の項目には関係しない。

## 片づけ

- 15:16:25 `studio off`、`stream stop`、3秒待って `test restore`
- `test restore` の出力: 「試験のシーンとソースを消し、元のプロファイル main とシーン Camera に戻した」。準備で控えた名前と同じ
- `profile` は `main`。`scenes` はシーンコレクション `test用`、今のシーン `Camera`、`T-` のシーンなし
- `test-original.json` は消えた
- 試験だけの MediaMTX（PID 54712）を PID で止めた。21935 の待ち受けはなくなった
- 見張り2つ（`watch-status.ps1` と `ctl.exe watch`）を止めた
- 止めていないもの:
  - 本体（PID 61340）
  - 本体の MediaMTX（PID 60476）
  - OBS（PID 41960）
