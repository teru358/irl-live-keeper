# T-15 の確認結果（キャリア回線からの揺れと長い時間）— 1日目（置いたまま）

- 実施日: 2026-11-22 22:46〜23:37
- 本体: `irl-live-keeper.exe`（2026-11-22 21:55:58、14,236,672 バイト）、22:45:41 起動（PID 79552、本体の MediaMTX は PID 100512）
- `ctl.exe`: 11,417,088 バイト（21:55:58）。`watch-status.ps1`: 4,217 バイト（21:36:58、`-Csv` あり）
- ユーザーの確認:
  - 新しい exe で起動した
  - `sub` の `on_bad` は `hide`
  - OBS のメディアソースの URL は `…/live/main`、`…/live/sub` に直した
  - OBS は配信していない
- 準備のあとの `warnings` は空。「URL が古い形」の警告は出ていない
  - 前回まで出ていた「サブカメラ のメディアソースが OBS に見つかりません」も消えた
  - ユーザーのメディアソースの名前は `Main` と `Sub` になっていた
- 回線:
  - iPhone（sub）は全部、キャリア回線から Tailscale（`100.118.173.77`）
  - T15-2c の Android（main）だけ LAN（`192.168.1.11`）
- 場所: 家の中、スマホは置いたまま（外を歩く分は2日目）
- 見張りのファイル:
  - `status-t15-srt.txt` / `t15-srt.csv`（22:48:18〜23:01:23）
  - `status-t15-rtmp.txt` / `t15-rtmp.csv`（23:01:25〜23:37）
  - `obs-t15.txt`
- 閾値は既定（`fail_kbps` 800 が5秒、`recover_kbps` 1500 が10秒）

## まとめ

| # | 結果 |
|---|---|
| T15-1 | 判定区間（22:50:42〜23:00:46）は SRT で10分ずっと OK。kbps は平均 2,068。RTT は平均 78ms。ロス率は1秒ごとの値で平均 2.5%、最大 33%。待機への切替は、手が触れて切れた1回（区間の外、正しい切替）だけ |
| T15-2 | It's My Live の RTMP で10分ずっと OK、kbps は平均 2,095、800 を下回ったことはない。待機への切替は0回 |
| T15-2b | Larix の RTMP は弾かれずにつながった（`[RTMP] … is publishing to path 'live/sub'`、2 tracks）。約7分ずっと OK。OBS で映ったかは、ユーザーから答えをもらっていない |
| T15-2c | IRL Pro の RTMP（`live/main`）は、つながって OK になった。最初の設定（1080p、ビットレート自動、約 7,200 kbps）では、`reader is too slow` が2分で 168 行出て、映像がぼやけ、ノイズが出た。720p・3,000 kbps 固定に直すと、送り始めの1秒だけ（3行）になった。映像は「一瞬崩れた以外は問題ない」 |
| T15-3 | 待機への切替は6回で、どれも送信が止まったため（正しい切替）。まちがった切替は0回 |
| T15-4 | MediaMTX の WARN は172行で、171行が T15-2c の `reader is too slow`、1行が SRT の `decode error`。ERR は0。本体の WARN は0。`media source refreshed` は2回（接続のたびに1回）。OBS のメディアソースは止まらず、メモリも増え続けていない |

## 準備

- 22:47 試験だけの MediaMTX（`dest.yml`）を起動: PID 35344、`127.0.0.1:21935` で待ち受けを確かめた
- `ctl.exe scenes` の1行目は `test用`
- 22:48:03 `test setup`: 元のプロファイル `main`、シーンコレクション `test用`、シーン `Camera2`
- 22:48:07 `scene T-Sub`
- 見張りを起動（`watch-status.ps1 -Log .\status-t15-srt.txt -Csv .\t15-srt.csv -IntervalMs 1000`、`ctl.exe watch 3600 .\obs-t15.txt`）
- 22:49:13 sub が OK になったあと、`profile` が `IRLKeeper-Test` であることを確かめて、22:49:40 `stream start`

メモリ（`Get-Process mediamtx, obs64, irl-live-keeper`、WorkingSet）:

| 時刻 | 本体 79552 | 本体の MediaMTX 100512 | 試験の MediaMTX 35344 | OBS 102060 |
|---|---|---|---|---|
| 22:49 前半の始まり | 24.6MB | 47.7MB | 34.0MB | 582.4MB |
| 22:54:58 | 25.1MB | 48.3MB | 44.2MB | 680.5MB |
| 23:00:46 前半の終わり | 25.2MB | 48.8MB | 43.7MB | 587.6MB |
| 23:07:57 | 25.2MB | 48.5MB | 43.8MB | 693.0MB |
| 23:12:43 後半の終わり | 25.2MB | 48.7MB | 43.5MB | 692.5MB |
| 23:37:05 片づけの前 | 25.4MB | 51.0MB | 44.4MB | 554.4MB |

## T15-1 前半（Larix の SRT、置いたまま）

### 流れ

- 22:49:01 `[SRT] [conn 100.118.173.77:58395] opened`
- 22:49:03 `[path live/sub] stream is available and online, 2 tracks (H264, MPEG-4 Audio)`、`is publishing to path 'live/sub'`
- 22:49:04.785 `camera changed camera=sub from=DOWN to=LOW kbps=2164.8`
- 22:49:09.786 `media source refreshed camera=sub source=Sub`
- 22:49:13.785 `camera changed camera=sub from=LOW to=OK kbps=2305.9`
- 22:49:40 配信を始めた（T-Sub、`owner=none`）
- **22:50:18 ユーザーの手が iPhone に触れて、送信が切れた**

```
22:50:18.517 [SRT] [conn 100.118.173.77:58395] closed: EOF
22:50:19.784 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
22:50:19.785 level=INFO msg="scene switch" from=T-Sub to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
22:50:20.115 level=INFO msg="standby changed" owner=auto return_scene=T-Sub
22:50:20.118 level=INFO msg="camera hidden" camera=sub items=3 reason=down
22:50:28.458 [SRT] [conn 100.118.173.77:52947] opened
22:50:30.549 [SRT] [conn 100.118.173.77:52947] is publishing to path 'live/sub'
22:50:31.784 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2433.6
22:50:40.784 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2450
22:50:41.786 level=INFO msg="camera shown" camera=sub items=3 reason=ok
22:50:41.786 level=INFO msg="scene switch" from=⚠回線待機中 to=T-Sub reason=ok camera=sub kbps=1729.8 purpose=AutoRecover
22:50:42.117 level=INFO msg="standby changed" owner=none return_scene=""
```

- 置いたままの10分をそろえるため、判定区間は、つなぎ直した 22:50:42 から 23:00:46 までにした
- 22:55:23.549 MediaMTX の WARN（1回だけ）。同じ秒の kbps は 177。ユーザーは画面を見ていなかった:

```
WAR [SRT] [conn 100.118.173.77:52947] decode error: astits: building new data failed: astits: parsing PES data failed: astits: fetching next bytes failed: astikit: slice length is 544, offset 9193 is invalid
```

- 23:02:06 SRT を止めた（後半へ）。`closed: EOF` → 23:02:07 `AutoFail`（送り方を替えたため）

### 数字（CSV の `id=sub`）

| 区間 | 行数 | kbps 最小 / 平均 / 最大 | 5%点 | <800 の行 | <1500 の行 | LOW | DOWN | 待機への切替 |
|---|---|---|---|---|---|---|---|---|
| 判定区間 22:50:42〜23:00:46 | 597 | 177 / 2,068 / 2,551 | 1,713 | 2 | 11 | 0 回 | 0 回 | 0 回 |
| 全体 22:49:44〜23:00:46 | 655 | 0 / 2,029 / 2,551 | 1,694 | 15 | 25 | 1 回（9秒） | 1 回（12秒） | 1 回（手が触れた） |

- 判定区間の SRT の値（597 件）:
  - `rtt_ms`: 最小 65.9 / 平均 78.4 / 最大 141.0
  - `loss_rate`（%）: 最小 0.0 / 平均 2.5 / 最大 33.3。0 より大きい行は 240、5% 以上の行は 73
  - `recv_mbps`: 最小 0.0 / 平均 2.2 / 最大 4.7
- 1500 を下回った行は、すべて1〜2秒で戻った
  - いちばん深い落ち込み: 22:55:17 に 186、22:55:23 に 177（`decode error` の秒）
  - そのほかは 838〜1,410

```
22:55:22.012 OK kbps=1892 rtt=98.8 loss=5.085
22:55:23.028 OK kbps=177  rtt=81.2 loss=30.682
22:55:24.039 OK kbps=1018 rtt=75.4 loss=8.922
22:55:25.052 OK kbps=2316 rtt=77.1 loss=0.000
```

- ロス率の高い秒（最大 33%）があっても、kbps はたいてい 1,700〜2,500 のままだった
  - SRT の再送で埋まっていると見られる
  - RTT は 141ms を超えなかった

## T15-2 後半（It's My Live の RTMP、置いたまま）

### 流れ

- 23:02:24 `[RTMP] [conn 100.118.173.77:63649] opened`
- 23:02:27 `[path live/sub] stream is available and online, 2 tracks (H264, MPEG-4 Audio)`、`is publishing to path 'live/sub'`
- 23:02:28 `DOWN → LOW kbps=435.3`
- 23:02:38 `LOW → OK kbps=2081.7`
- 23:02:39 `camera shown items=3 reason=ok` → `scene switch from=⚠回線待機中 to=T-Sub reason=ok purpose=AutoRecover`
- 判定区間: 23:02:40〜23:12:41
- 23:13:16 `closed: stream unpublished`（T15-2b へ替えたため）→ 23:13:18 `AutoFail`

### 数字（CSV の `id=sub`）

| 区間 | 行数 | kbps 最小 / 平均 / 最大 | 5%点 | <800 の行 | <1500 の行 | LOW | DOWN | 待機への切替 |
|---|---|---|---|---|---|---|---|---|
| 23:02:40〜23:12:41 | 594 | 807 / 2,095 / 3,777 | 1,610 | 0 | 23 | 0 回 | 0 回 | 0 回 |

- 1500 を下回った行は、1〜3秒で戻った（最小は 23:05:26 の 807）
- RTT とロス率は出ない（RTMP のため空）
- MediaMTX の WARN は0

## T15-2b おまけ（Larix の RTMP）

- 23:13:37 `[RTMP] [conn 100.118.173.77:50189] opened`
- 23:13:39 `[path live/sub] stream is available and online, 2 tracks (H264, MPEG-4 Audio)`、`[RTMP] [conn 100.118.173.77:50189] is publishing to path 'live/sub'`
  - 弾かれていない
  - URL の欄 `rtmp://100.…:1935/live` と Key の欄 `sub?user=…&pass=…` で通った
- 23:13:40 `DOWN → LOW kbps=2499.7`
- 23:13:50 `LOW → OK kbps=2212.5`
- 23:13:51 `camera shown` → `AutoRecover` で T-Sub
- 23:13:52〜23:20:37（約7分）: 400 行、ずっと OK、待機への切替0回、MediaMTX の WARN は0
  - kbps: 最小 340 / 平均 2,091 / 最大 5,611（5%点 1,568）
  - <800 の行 3、<1500 の行 12
- いちばん深い落ち込み。800 未満が3秒続いたが、5秒には届かず、LOW にもならなかった:

```
23:15:02.526 OK kbps=1235
23:15:03.539 OK kbps=340
23:15:04.542 OK kbps=386
23:15:05.560 OK kbps=672
23:15:06.564 OK kbps=923
23:15:07.582 OK kbps=1268
```

- OBS の T-Sub に映ったかは、ユーザーに聞いたが、答えをもらっていない
  - `ctl.exe inputs` では、`Sub` と `cam-sub` が `PLAYING` で cursor が進んでいた（23:20:37）
- 23:21:02 `closed: EOF`（T15-2c のため iPhone を止めた）→ 23:21:04 `AutoFail`

## T15-2c IRL Pro（Android、LAN）の RTMP

### 1回目（1080p、ビットレート自動）

- 23:24:45 `[RTMP] [conn 192.168.1.11:41388] opened`
- 23:24:47 `[path live/main] stream is available and online, 2 tracks (H264, MPEG-4 Audio)`、`is publishing to path 'live/main'`
- 23:24:48 `DOWN → LOW kbps=6918.3`
- 23:24:53 `media source refreshed camera=main source=Main`
- 23:24:57 `LOW → OK kbps=6710.5`
- 23:24:53 から、MediaMTX の `reader is too slow` がほぼ毎秒出た
  - OBS の2つのセッションが `live/main` を読んでいた
  - 片方は1秒に最大 570 フレームを捨てた

```
23:24:53.685 WAR [RTSP] [session 2610e2a2] reader is too slow, discarding 81 frames
23:24:59.846 WAR [RTSP] [session 1534bdbb] reader is too slow, discarding 133 frames
23:25:46.846 WAR [RTSP] [session 1534bdbb] reader is too slow, discarding 570 frames
```

- 23:25:13 ユーザーが、接続を確かめるために OBS で `Camera` に手で切り替えた（`scene changed by user from=⚠回線待機中 to=Camera`）
- ユーザーの報告: 映像がぼやけたり、ノイズが走ったりした
- 23:26:46 ユーザーが送信を止めた（`closed: EOF`）→ 23:26:48 `AutoFail`
- 数字（`id=main`、23:24:58〜23:26:45、107 行）:
  - kbps: 最小 5,768 / 平均 7,189 / 最大 9,120
  - `reader is too slow`: 168 行、捨てた合計 32,208 フレーム
    - session 1534bdbb（`Main` の見直しのあとにできた方）: 107 行、28,686 フレーム
    - session 2610e2a2: 61 行、3,522 フレーム
  - この間の CPU 全体は 9.2%。OBS の `Main` と `cam-main` は `PLAYING` で、cursor も進んでいた
- ユーザーが設定を確かめると、解像度 1080p、ビットレートは自動だった。720p、3,000 kbps 固定に直した

### 2回目（720p、3,000 kbps 固定）

- 23:31:20 シーンを `T-B` にした（本体の戻り先を試験のシーンにするため）
- 23:31:22 `opened` → 23:31:23 `is publishing to path 'live/main'`（2 tracks）
- 23:31:24 `DOWN → LOW kbps=3758.5`
- 23:31:33 `LOW → OK kbps=2850.2`
- 23:31:34 `AutoRecover` で T-B
- 数字（`id=main`、23:31:35〜23:34:36、180 行）:
  - kbps: 最小 2,702 / 平均 3,798 / 最大 6,707（5%点 3,105）。設定の 3,000 より高めに出ている
  - ずっと OK、待機への切替0回
  - `reader is too slow`: 3 行、209 フレーム。23:31:43〜44 だけで、そのあと（23:36:52 まで）は0
- 23:31:44 に kbps が 6,707 に跳ね、同じ秒に `reader is too slow` が出た
  - ユーザーの報告: 「少し崩れた時があった。高ビットレートの瞬間かもしれない。その一瞬以外は問題ない」
- 23:36:53 `scene T-Sub` に戻した
  - sub が DOWN なので、本体はすぐ待機へ送った（`AutoFail`、戻り先 `T-Sub`）

## T15-3 まちがった切替の判断

待機への切替は6回あり、どれも送信が本当に止まっていた（MediaMTX の `closed` の行の 1〜2秒後に DOWN）。まちがった切替（映像が乱れていないのに待機へ送った）は0回。

| 時刻 | 直前の kbps | 理由 | 判定 |
|---|---|---|---|
| 22:50:19 | 2,000 前後 → 0（`closed: EOF`） | 手が iPhone に触れた | 正しい |
| 23:02:07 | → 0（`closed: EOF`） | SRT → It's My Live に替えた | 正しい |
| 23:13:18 | → 0（`stream unpublished`） | It's My Live → Larix RTMP に替えた | 正しい |
| 23:21:04 | → 0（`closed: EOF`） | iPhone を止めた | 正しい |
| 23:26:48 | 7,000 前後 → 0（`closed: EOF`） | IRL Pro を止めて設定を直した | 正しい |
| 23:36:53 | sub は DOWN のまま | `scene T-Sub` に戻した（sub は送っていない） | 期待どおり |

逆に、映像が乱れたのに切り替えなかった場面が2つある。
- T15-2c の1回目: kbps は 7,000 前後で OK のままだったが、映像はぼやけ、ノイズが出た。
- 22:55:23 の SRT の `decode error`: 1秒だけで、kbps も1秒で戻った。

kbps だけでは、OBS の側の取りこぼし（`reader is too slow`）は見えない。

## T15-4 長い時間の様子

- 本体のログ（22:48〜23:37）:
  - MediaMTX の WAR は 172 行
    - 171 行が `reader is too slow`（168 行は T15-2c の1回目、3 行は2回目の始まり）
    - 1 行が SRT の `decode error`
  - MediaMTX の ERR は0。本体の WARN は0
  - `media source refreshed`: 2回（22:49:09 `Sub`、23:24:53 `Main`。どちらも、つながった直後に1回）
- OBS のメディアソース（`ctl.exe inputs` を約5分ごと）:
  - 22:49:58、22:54:58、22:59:49、23:00:46、23:02:56、23:07:57、23:12:43、23:20:37、23:26:03、23:34:36 のどれも、送っているソースは `PLAYING` で cursor が進んでいた。止まったことはない
  - 送っていないソースは `ENDED`、または `PLAYING` で cursor=0
- メモリ（上の表）:
  - 本体は 24.6〜25.4MB、本体の MediaMTX は 47.7〜51.0MB、試験の MediaMTX は 43〜44MB でほぼ一定
  - OBS は 554〜693MB の間で上下していて、増え続けてはいない
- 23:26 ごろ、IRL Pro の1回目の間に、`Sub` と `cam-sub` で `MediaInputPlaybackStarted` / `Ended` が約2秒ごとに続いていた（`obs-t15.txt`）
  - iPhone を止めていて、OBS が sub をつなぎ直そうとしている動き
  - 本体のログに WARN はなく、害は見えない

## 集計に使ったコマンド

`t15stat.ps1`（スクラッチパッドのスクリプト）で、CSV の区間ごとに集計した。中身は次と同じ:

```powershell
$rows = Import-Csv .\t15-srt.csv | Where-Object { $_.id -eq 'sub' -and $_.time -ge '22:50:42' -and $_.time -le '23:00:46' }
$rows | ForEach-Object { [double]$_.kbps } | Measure-Object -Minimum -Average -Maximum
$rows | Where-Object { $_.rtt_ms -ne '' } | ForEach-Object { [double]$_.rtt_ms } | Measure-Object -Minimum -Average -Maximum
$rows | Where-Object { $_.loss_rate -ne '' } | ForEach-Object { [double]$_.loss_rate } | Measure-Object -Minimum -Average -Maximum
@($rows | Where-Object { [double]$_.kbps -lt 800 }).Count
# 5%点: kbps を並べて (件数-1)*0.05 番目
# LOW / DOWN の回数と長さ: state が同じ行の続き（区間）を数えた
# 待機への切替: scene 列が ⚠回線待機中 に変わった回数
```

揺れの比べ（同じ区間、`id` ごと）:

```powershell
# 標準偏差、変動係数（標準偏差 / 平均）、前の秒より 40% 超落ちた回数、5秒の移動平均の最小
```

| 区間 | 行数 | 平均 | 標準偏差 | 変動係数 | 前秒比 -40% 超の急落 | 5秒平均の最小 |
|---|---|---|---|---|---|---|
| SRT（Larix） 22:50:42〜23:00:46 | 597 | 2,068 | 334 | 16% | 5 回 | 1,333 |
| RTMP（It's My Live） 23:02:40〜23:12:41 | 594 | 2,095 | 298 | 14% | 6 回 | 1,304 |
| RTMP（Larix） 23:13:52〜23:20:37 | 400 | 2,091 | 327 | 16% | 5 回 | 711 |
| RTMP（IRL Pro 720p、LAN） 23:31:35〜23:34:36 | 180 | 3,798 | 419 | 11% | 0 回 | 3,390 |
| RTMP（IRL Pro 1080p、LAN） 23:24:58〜23:26:45 | 106 | 7,174 | 594 | 8% | 0 回 | 6,606 |

本体のログの数え方:

```powershell
$L = Get-Content .\logs\irl-live-keeper-2026-11-22.log -Encoding UTF8 | Where-Object { <22:48〜23:37 の time= の行> }
@($L | Where-Object { $_ -match 'too slow' }).Count
@($L | Where-Object { $_ -match ' WAR ' }).Count
@($L | Where-Object { $_ -match 'media source refreshed' }).Count
```

## 意見

### (a) 既定の閾値（800 / 1500）は、このキャリア回線に合っているか

置いたままのキャリア回線（iPhone、約 2,000 kbps で送る設定）では、合っていると考える。

- 30分近く送って、まちがった切替は0回だった。800 を下回った秒は、SRT で2回、Larix の RTMP で3回、It's My Live で0回あった。どれも1〜3秒で戻り、「5秒続いたら悪い」の5秒に届かなかった。
- いちばん際どかったのは、Larix の RTMP の 23:15:03〜05（340、386、672）の3秒で、5秒の移動平均は 711 まで下がった。あと2秒続けば LOW になっていた。歩いて基地局が変わると、もっと長く落ちるかもしれない。2日目の数字で確かめたい。
- 戻りの 1500（10秒）について:
  - 5%点は 1,568〜1,713 で、ふだんの揺れは 1500 の上にある。
  - DOWN からつなぎ直したとき、LOW → OK はどれも9〜10秒で済んだ。戻りが遅すぎることはない。
- 気をつける点:
  - 800 / 1500 は、送る側が約 2,000 kbps の前提なら、ほどよい。送る側が 3,000〜4,000 kbps（IRL Pro 720p）なら、もっと早く悪くなったことを知りたくなるかもしれない。
  - 逆に、1,000 kbps 前後で送る設定だと、ふだんから 800 に近すぎる。
  - 閾値は、送るビットレートとあわせて決めるのがよい。

### (b) SRT と RTMP で、揺れ方や切替の起き方に差があったか

置いたままでは、大きな差はなかった。

- **揺れ:** 変動係数は SRT 16%、It's My Live の RTMP 14%、Larix の RTMP 16% で、ほぼ同じだった。急に落ちた回数も5〜6回で同じくらい。
- **落ち込みの深さ:** いちばん深いのは、SRT（177、1秒）と Larix の RTMP（340〜672、3秒）。It's My Live の RTMP は最小 807 で、いちばん穏やかだった。アプリの違いかもしれない。
- **切替:** どちらも、送信を止めたとき以外は0回だった。
- **SRT だけの点:**
  - ロス率が1秒単位で最大 33% まで上がった。それでも kbps はほとんど落ちず、再送で埋まっていると見られる。RTT は 66〜141ms で安定していた。
  - MediaMTX の `decode error` が1回出た。RTMP では MediaMTX の WARN は0。
- **SRT の値の使い道:** 置いたままでは、ロス率と RTT は kbps より早く悪くならなかった。ロス率の高い秒の多くは、kbps が平常のまま。歩いたとき（2日目）に、kbps の落ち込みの前にロス率や RTT が上がるかを見れば、SRT の値を判断に使えるかがわかる。

## 気になった動き

1. **映像の乱れと kbps の判定がずれた。** IRL Pro の 1080p・自動（約 7,200 kbps）では、OBS が読み切れず、`reader is too slow` が出続けて、映像が乱れた。本体は kbps だけを見ているので OK のままだった。
   - OBS 側の取りこぼしは、本体の健康の判定に入っていない。
   - 屋外の送る側の設定がまちがっていると、本体は乱れを見逃す。
   - `reader is too slow` の数を status や警告に出すと、気づきやすい。
2. **IRL Pro の実際のビットレートが設定より高い。** 3,000 kbps 固定でも、平均 3,798、最大 6,707 で測れた。
3. **見張りの切り替えのすきま。** SRT 用の見張りは 23:01:23 まで書き、RTMP 用は 23:01:25 から書いた（すきまは約2秒）。SRT は 23:02:06 まで続いていて、その分は `t15-rtmp.csv` の初めにある（`protocol=srt` が 42 行）。
4. **起動の表示と送信用の URL。** URL は指示書の `live/<id>` の形でつながった。`[RTMP] … is publishing to path 'live/sub'` と `'live/main'` が出ている。

## 片づけ

- 23:37:05 `studio off`、`stream stop`
  - 本体のログ: `stream state streaming=false`、`standby changed owner=none`、`camera shown camera=sub items=3 reason=stream_stop`
- 3秒待って `test restore`
  - 出力: 「試験のシーンとソースを消し、元のプロファイル main とシーン Camera2 に戻した」。準備で控えた名前と同じ
- `profile` は `main`。`scenes` はシーンコレクション `test用`、今のシーン `Camera2`、`T-` のシーンなし
- `test-original.json` は消えた。restore のあとの `warnings` は空
- 試験だけの MediaMTX（PID 35344）を PID で止めた。21935 の待ち受けはなくなった
- 見張り2つ（`watch-status.ps1` と `ctl.exe watch`）を止めた
- 止めていないもの:
  - 本体（PID 79552）
  - 本体の MediaMTX（PID 100512）
  - OBS（PID 102060）

## 2日目（外を歩く）

まだ行っていない。指示書のとおり、Larix の SRT で10分、It's My Live の RTMP で10分、スマホを持って外を歩く。見張りのファイルは `status-t15-srt-walk.txt` / `t15-srt-walk.csv`、`status-t15-rtmp-walk.txt` / `t15-rtmp-walk.csv`。ここに足して、上の置いたままの数字と並べる。
