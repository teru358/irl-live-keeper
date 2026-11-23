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

- 実施日: 2026-11-23 09:23〜10:02
- 本体: `irl-live-keeper.exe`（2026-11-23 00:06:26、14,259,200 バイト）、09:17:04 起動（PID 20328、本体の MediaMTX は PID 2384）。OBS は PID 2416
- `ctl.exe`: 11,416,576 バイト（00:06:27）。`watch-status.ps1`: 4,217 バイト（11-22 21:36:58）。`walk-log.ps1`: 1,701 バイト（09:21:05、実行前に中身を読んだ）
- ユーザーの確認: 新しい exe で起動し直した（`config.yaml` は1日目のまま）、OBS は配信していない、Studio Mode は off、iPhone の送信先は1日目の `100.…` の形のまま、スマホは2台とも送っていない
- 回線:
  - iPhone（sub）はキャリア回線から Tailscale（`100.118.173.77`）
  - Android（main）は LAN（`192.168.1.11`）。IRL Pro、W-0 のあとは 720p・3,000 kbps 固定
  - PC（`192.168.1.16`）は有線（イーサネット 1Gbps）
- シーン: W-0 は `T-B`、歩く間は `T-A`（main と sub）
- 見張りのファイル:
  - W-0: `status-t15-slow.txt`（09:26:33〜09:34:49）
  - 歩く間: `status-t15-walk.txt` / `t15-walk.csv`（09:34:49〜10:02:03、付け替えなし）
  - OBS のイベント: `obs-t15-walk.txt`
  - 2分ごとの記録: `inputs-t15-walk.txt`（09:35:26〜10:01:28）
- 閾値は既定（`fail_kbps` 800 が5秒、`recover_kbps` 1500 が10秒）

### まとめ

| # | 結果 |
|---|---|
| W-0 | 1080p・自動で `reader is too slow` が出ると、最初の行から2.2秒後に `obs reader too slow camera=main count=3` が出て、`warnings` に「OBS が読み切れていません」の1行が出た。main は OK のまま（健康に入らない）。最後の行から60.2秒後に `obs reader caught up camera=main count=0` が出て、警告が消えた。720p・3,000 kbps では2分間0行 |
| W-1 | 前半（Larix の SRT、09:37:25〜09:49:24）は、sub はずっと OK。LOW・DOWN・隠す動きは0回。kbps は平均 2,063、800 未満が続いたのは2秒が2回だけ。RTT は平均 109ms（最大 1,306ms）、ロス率は平均 0.7%（最大 35.7%） |
| W-2 | 後半（It's My Live の RTMP、09:50:16〜10:00:17）は、sub を隠したのが2回。1回は 09:52:09〜09:53:05 に約 600 kbps が56秒続いたため（理由はわからない）、もう1回は停止ボタンに触れたため。レンジ・冷蔵庫に入れた2回（3秒・4秒の落ち込み）は、5秒に届かず LOW にならなかった |
| W-3 | 待機への切替は3回で、どれも家の中の main（Android、LAN）の送信が止まったため（正しい切替）。sub だけ悪いのに待機へ送ったことはない。sub を隠したのは、替える間の1回を入れて3回。まちがった切替と言えるものは0回 |
| W-4 | MediaMTX の WARN は100行（`reader is too slow` 99、`decode error` 1）、ERR は0、本体の WARN は0。`media source refreshed` は6回（main の再接続のとき）。OBS のメディアソースは、送っている間は止まっていない。メモリは増え続けていないが、OBS は 1日目より高めで上下が大きい（最大 1,066MB） |

### 準備

- 09:26:13 試験だけの MediaMTX（`dest.yml`）を起動: PID 24788、`127.0.0.1:21935` で待ち受けを確かめた
- `ctl.exe scenes` の1行目は `test用`。`studio status` は `studioMode false`。`profile` は `main`
- 09:26:22 `test setup`: 元のプロファイル `main`、シーンコレクション `test用`、シーン `Camera2`。`T-A`、`T-B`、`T-Sub` ができた
- `watch-status.ps1 -Once` の `warnings` は空
- 09:26:33 W-0 の見張り（PID 26460）、`scene T-B`
- 09:34:49 W-0 の見張りを止め、`scene T-A`。歩く間の見張りを起動: `watch-status.ps1`（PID 36528）、`ctl.exe watch 3600 .\obs-t15-walk.txt`（PID 38692）
- 09:34:49 `walk-log.ps1` を Windows PowerShell 5.1 で起動したが、すぐ構文エラーで終わった（下の「気になった動き」）。09:35:26 に PowerShell 7（`pwsh`、PID 34288）で起動し直した。ファイルは書き換えていない
- 09:37:24 main と sub がどちらも OK。`ctl.exe inputs` を2回読み、4つのソースが `PLAYING` で cursor が進むことを確かめた。`items T-A` は `cam-main`、`cam-sub` とも `enabled=true`
- `profile` が `IRLKeeper-Test` であることを確かめて、09:37:33 `stream start`。`streaming=True scene=T-A health=OK scene_cams=[main,sub] owner=none`
- 09:37 ごろ、ユーザーに外に出てよいことを伝えた

メモリ（WorkingSet。09:35 以降は `inputs-t15-walk.txt` から）:

| 時刻 | 本体 20328 | 本体の MediaMTX 2384 | 試験の MediaMTX 24788 | OBS 2416 |
|---|---|---|---|---|
| 09:26 準備のあと | 66.4MB | 39.7MB | 32.1MB | 282.9MB |
| 09:35:26 歩く前 | 66.7MB | 48.9MB | 34.2MB | 596.0MB |
| 09:37:26 前半の始まり | 66.6MB | 52.8MB | 35.1MB | 866.0MB |
| 09:43:27 | 34.6MB | 52.3MB | 44.0MB | 802.5MB |
| 09:49:27 前半の終わり | 34.7MB | 52.0MB | 45.1MB | 397.8MB |
| 09:53:27 | 35.1MB | 50.8MB | 44.2MB | 1,052.9MB |
| 09:55:27 | 35.2MB | 51.1MB | 44.4MB | 1,066.1MB |
| 09:57:27 | 34.6MB | 50.3MB | 44.6MB | 654.8MB |
| 10:01:48 後半の終わり | 34.8MB | 52.0MB | 45.2MB | 968.0MB |

### 区切りの時刻

| 区切り | 時刻 | 決めたログの行 |
|---|---|---|
| 前半の始まり | 09:37:02.636（集計は sub が OK になった 09:37:25 から） | `[SRT] [conn 100.118.173.77:54424] is publishing to path 'live/sub'`、09:37:24.381 `camera changed camera=sub from=LOW to=OK` |
| 前半の終わり | 09:49:25.285 | `[SRT] [conn 100.118.173.77:54424] closed: EOF` |
| 替える間 | 09:49:25〜09:50:15 | 09:49:56.991 `[RTMP] [conn 100.118.173.77:61905] is publishing to path 'live/sub'`、09:50:15.380 `camera changed camera=sub from=LOW to=OK` |
| 後半の始まり | 09:50:16 | 09:50:16.382 `camera shown camera=sub items=3 reason=ok` |
| 後半の途中の切れ（停止ボタン） | 09:57:19〜09:58:26 | 09:57:19.978 `[RTMP] [conn 100.118.173.77:61905] closed: stream unpublished` → 09:58:09.634 `[RTMP] [conn 100.118.173.77:55934] is publishing to path 'live/sub'` → 09:58:26 OK |
| 後半の終わり | 10:00:17 | ユーザーが帰ってきて話しかけた時刻 |

- CSV の `protocol`: `srt` は 09:37:04〜09:49:25（733 行）、`rtmp` は 09:49:58〜10:02:03（667 行）
- ユーザーから聞いたこと:
  - 家を出た時刻: 聞けなかった。そのため、前半は最初の2分を除いた数字（09:39:03〜）も並べた
  - 道の様子: 「この辺りは電波が悪いところが少ない」。そのため、後半（RTMP）で、iPhone を電子レンジや冷蔵庫の中に少し入れて、電波を遮れるか試した。その2回は、09:53:31〜33 と 09:56:50〜53 の落ち込みだと、ユーザーが確かめた
  - 停止ボタン: 後半の途中で、停止ボタンに触れてしまい、送り直した（ログの 09:57:19）
  - 09:52:09〜09:53:05 の約 600 kbps の56秒が何だったかは、ユーザーにもわからない
  - 家の中の Android（main）について、09:48〜09:53 に思い当たることは特にない
- 後半のうち、どこまでが外で、いつ家に戻ったかは聞けていない（レンジ・冷蔵庫を試したのは家の中）

### W-0 新しい警告（IRL Pro、Android、LAN、配信はまだ始めていない、`T-B`）

1080p・ビットレート自動:

```
09:28:34.412 [RTMP] [conn 192.168.1.11:41858] opened
09:28:37.212 [RTMP] [conn 192.168.1.11:41858] is publishing to path 'live/main'
09:28:38.381 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=12868.1
09:28:42.154 WAR [RTSP] [session c884a8ab] reader is too slow, discarding 127 frames   ← 最初の行
09:28:43.154 WAR [RTSP] [session c884a8ab] reader is too slow, discarding 241 frames
09:28:44.154 WAR [RTSP] [session c884a8ab] reader is too slow, discarding 83 frames
09:28:44.380 level=INFO msg="obs reader too slow" camera=main count=3                   ← 最初の行から2.2秒後
09:28:48.381 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=8728.1
```

- `reader is too slow` は、09:28:42〜09:31:04 に140行、33,182 フレーム（OK になってからの2分〔09:30:50 まで〕では127行、29,714 フレーム）。読んでいた OBS のセッションは1つ（`c884a8ab`）
- `warnings` に出た文（そのまま）: 「メインカメラ の映像を OBS が読み切れていません（MediaMTX が映像を捨てています）。送るビットレートや解像度を下げるか、OBS のメディアソースの「ハードウェアデコードを使用」を入れてください」
- 1行の形では `warnings=1`。main は 09:28:48 から OK のまま（09:28:49〜09:30:50 の121行はすべて OK、kbps は約 6,000〜8,700）。健康には入っていない
- ユーザーの報告: 「止まっているときは大丈夫だが、カメラを動かすと必ず乱れた」

720p・3,000 kbps 固定に直した:

```
09:31:04.154 WAR [RTSP] [session c884a8ab] reader is too slow, discarding 336 frames   ← 最後の行
09:31:04.267 [RTMP] [conn 192.168.1.11:41858] closed: EOF
09:31:06.380 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
09:32:04.380 level=INFO msg="obs reader caught up" camera=main count=0                  ← 最後の行から60.2秒後
09:32:28.231 [RTMP] [conn 192.168.1.11:41862] opened
09:32:29.681 [RTMP] [conn 192.168.1.11:41862] is publishing to path 'live/main'
09:32:31.381 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=4443.8
09:32:40.380 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=4866.3
```

- 09:32:44 の `watch-status.ps1 -Once` で、`warnings` は空に戻っていた（`warnings=0`）
- 720p で送り直してから2分（09:32:05〜09:34:40）、`reader is too slow` は0行。1日目にあった送り始めの3行も出ず、警告は出直さなかった
- main の kbps（09:32:41〜09:34:40、119 行）: 最小 2,468 / 平均 4,078 / 最大 7,161。設定の 3,000 より高い（1日目も平均 3,798）
- 歩く間も、IRL Pro は 720p・3,000 kbps のまま送り続けてもらった

### W-1 前半（Larix の SRT、外を歩く）

流れ:

```
09:37:00.546 [SRT] [conn 100.118.173.77:54424] opened
09:37:02.636 [SRT] [conn 100.118.173.77:54424] is publishing to path 'live/sub'
09:37:04.381 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2137.6
09:37:24.381 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2552.8
09:37:33.589 level=INFO msg="stream state" streaming=true
09:44:04.637 WAR [SRT] [conn 100.118.173.77:54424] decode error: astits: building new data failed: astits: parsing PES data failed: astits: fetching next bytes failed: astikit: slice length is 4040, offset 6820 is invalid
09:48:25.381 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
09:48:25.381 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
09:48:27.381 level=INFO msg="camera changed" camera=main from=DOWN to=LOW kbps=2395.3
09:48:36.381 level=INFO msg="camera changed" camera=main from=LOW to=OK kbps=2731.1
09:48:37.382 level=INFO msg="scene switch" from=⚠回線待機中 to=T-A reason=ok camera=main kbps=1623.6 purpose=AutoRecover
09:48:55.381 level=INFO msg="camera changed" camera=main from=OK to=DOWN kbps=0
09:48:55.382 level=INFO msg="scene switch" from=T-A to=⚠回線待機中 reason=down camera=main kbps=0 purpose=AutoFail
09:49:02.049 [RTMP] [conn 192.168.1.11:41862] closed: read tcp 192.168.1.16:1935->192.168.1.11:41862: i/o timeout
09:49:13.453 [RTMP] [conn 192.168.1.11:41984] opened
09:49:23.453 [RTMP] [conn 192.168.1.11:41984] closed: read tcp 192.168.1.16:1935->192.168.1.11:41984: i/o timeout
09:49:25.285 [SRT] [conn 100.118.173.77:54424] closed: EOF                ← 前半の終わり（RTMP に替えるため）
```

- 前半の SRT は、途中で一度も切れていない（`closed` → `opened` はない）
- sub は 09:37:25〜09:49:24 ずっと OK。`camera hidden` / `camera shown` はなかった
- 待機への切替（09:48:25、09:48:55）は、2回とも家の中の main が止まったため（W-3）
- `decode error` が1回（09:44:04）。その秒の前後の kbps は平常（1日目と同じ種類の行）

### W-2 後半（It's My Live の RTMP）

流れ（main の再接続の行は W-3 に書く）:

```
09:49:27.381 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
09:49:27.403 level=INFO msg="camera hidden" camera=sub items=3 reason=down            ← 替える間（数えない）
09:49:54.240 [RTMP] [conn 100.118.173.77:61905] opened
09:49:56.991 [RTMP] [conn 100.118.173.77:61905] is publishing to path 'live/sub'
09:49:58.381 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=1686.2
09:50:15.380 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=1943.3
09:50:16.382 level=INFO msg="camera shown" camera=sub items=3 reason=ok               ← 後半の始まり
09:52:13.381 level=INFO msg="camera changed" camera=sub from=OK to=LOW kbps=558.9
09:52:13.383 level=INFO msg="camera hidden" camera=sub items=3 reason=low
09:53:14.381 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2639.9
09:53:15.383 level=INFO msg="camera shown" camera=sub items=3 reason=ok
（09:53:31〜33、09:56:50〜53 レンジ・冷蔵庫。状態は OK のまま）
09:57:19.978 [RTMP] [conn 100.118.173.77:61905] closed: stream unpublished          ← 停止ボタン
09:57:22.380 level=INFO msg="camera changed" camera=sub from=OK to=DOWN kbps=0
09:57:22.383 level=INFO msg="camera hidden" camera=sub items=3 reason=down
09:58:06.962 [RTMP] [conn 100.118.173.77:55934] opened
09:58:09.634 [RTMP] [conn 100.118.173.77:55934] is publishing to path 'live/sub'
09:58:11.381 level=INFO msg="camera changed" camera=sub from=DOWN to=LOW kbps=2513.1
09:58:26.381 level=INFO msg="camera changed" camera=sub from=LOW to=OK kbps=2084.2
09:58:27.382 level=INFO msg="camera shown" camera=sub items=3 reason=ok
10:00:17 ユーザーが帰ってきて話しかけた（後半の終わり）
```

- 09:52:09〜09:53:05 の落ち込み（56秒）: 721 から下がり、そのあと約 550〜610 kbps でほぼ平らに続いた（いちばん低いのは 156）。09:53:06 から 2,158、3,325 と戻った
  - MediaMTX の `closed` も `decode error` もない
  - OBS の `Sub` の cursor は、09:51:27 の 88秒から 09:53:27 の 209秒へ、2分で121秒進んでいて、メディアソースは止まっていない
  - 平らな約 600 kbps は、送る側（It's My Live）がビットレートを下げたように見える。原因はわからない（ユーザーにもわからない）
  - このとき OBS は待機シーンにいた（main が止まっていたため）。sub の項目は、待機の間に裏で隠れた
- レンジ・冷蔵庫の2回:
  - 09:53:31〜33（180、616、63）の3秒
  - 09:56:50〜53（676、133、141、148）の4秒
  - どちらも 800 未満は5秒に届かず、LOW にならなかった（あと1〜2秒で LOW になっていた）

### 集計（CSV の `id=sub`）

| 区間 | 行数 | kbps 最小 / 平均 / 最大 | 5%点 | <800 の行 | <1500 の行 | LOW | DOWN | sub を隠した | 待機への切替 |
|---|---|---|---|---|---|---|---|---|---|
| 前半 SRT 09:37:25〜09:49:24 | 711 | 4 / 2,063 / 3,733 | 1,484 | 7 | 37 | 0 回 | 0 回 | 0 回 | 2 回（どちらも main） |
| 前半 SRT、最初の2分を除く 09:39:03〜09:49:24 | 614 | 4 / 2,050 / 3,733 | 1,475 | 7 | 36 | 0 回 | 0 回 | 0 回 | 2 回（どちらも main） |
| 後半 RTMP 全体 09:50:16〜10:00:17 | 594 | 0 / 1,793 / 7,805 | 0 | 125 | 144 | 2 回（60秒、15秒） | 1 回（49秒、停止ボタン） | 2 回（62秒、65秒） | 1 回（main） |
| 後半 RTMP、停止ボタンの前 09:50:16〜09:57:19 | 418 | 0 / 1,914 / 7,805 | 437 | 71 | 85 | 1 回（60秒） | 0 回 | 1 回（62秒） | 1 回（main） |
| 後半 RTMP、停止ボタンの後 09:58:27〜10:00:17 | 110 | 629 / 2,094 / 3,040 | 1,065 | 2 | 7 | 0 回 | 0 回 | 0 回 | 0 回 |

- 隠した長さは `camera hidden` から `camera shown` まで: 09:52:13〜09:53:15（62秒、`reason=low`）、09:57:22〜09:58:27（65秒、停止ボタン）。替える間の 09:49:27〜09:50:16 は数えていない
- LOW の区間: 09:52:13〜09:53:13（60行）、09:58:11〜09:58:26（15行、停止ボタンのあとのつなぎ直し）。DOWN の区間: 09:57:22〜09:58:10（49行）

揺れの比べ:

| 区間 | 行数 | 平均 | 標準偏差 | 変動係数 | 前秒比 -40% 超の急落 | 5秒平均の最小 |
|---|---|---|---|---|---|---|
| 前半 SRT 09:37:25〜09:49:24 | 711 | 2,063 | 444 | 22% | 14 回 | 932（09:47:59 まで） |
| 前半 SRT、最初の2分を除く | 614 | 2,050 | 453 | 22% | 14 回 | 932 |
| 後半 RTMP 全体 | 594 | 1,793 | 956 | 53% | 36 回 | 0（停止ボタン） |
| 後半 RTMP、停止ボタンの前 | 418 | 1,914 | 902 | 47% | 26 回 | 296（09:52:27 まで） |
| 後半 RTMP、停止ボタンの後 | 110 | 2,094 | 381 | 18% | 8 回 | 1,691 |

800 未満が2秒以上続いた区間:

| 区間 | 時刻 | 長さ | kbps | 分け |
|---|---|---|---|---|
| 前半 SRT | 09:47:08〜09 | 2秒 | 385, 786 | 2秒 |
| 前半 SRT | 09:47:56〜57 | 2秒 | 365, 4 | 2秒 |
| 後半 RTMP | 09:50:52〜53 | 2秒 | 254, 0 | 2秒 |
| 後半 RTMP | 09:52:09〜09:53:05 | 56秒 | 約 550〜610（最小 156） | 5秒以上（LOW、隠した） |
| 後半 RTMP | 09:53:31〜33 | 3秒 | 180, 616, 63 | 3〜4秒（レンジ・冷蔵庫） |
| 後半 RTMP | 09:56:50〜53 | 4秒 | 676, 133, 141, 148 | 3〜4秒（レンジ・冷蔵庫） |
| 後半 RTMP | 09:57:20〜09:58:10 | 51秒 | 0 | 5秒以上（停止ボタン、DOWN） |

- 前半 SRT: 3〜4秒は0、5秒以上は0
- 後半 RTMP: 3〜4秒は2（どちらもレンジ・冷蔵庫）、5秒以上は2（1つは停止ボタン）

前半（SRT）の RTT とロス率:

| 区間 | rtt_ms 最小 / 平均 / 最大 | loss_rate（%） 最小 / 平均 / 最大 | recv_mbps 平均 / 最大 |
|---|---|---|---|
| 1日目 置いたまま 22:50:42〜23:00:46 | 65.9 / 78.4 / 141.0 | 0.0 / 2.5 / 33.3 | 2.2 / 4.7 |
| 2日目 歩いた 09:39:03〜09:49:24（最初の2分を除く） | 63.0 / 115.6 / 1,306.5 | 0.0 / 0.8 / 35.7 | 2.1 / 5.3 |

- SRT の全体（09:37:03〜09:49:24、732 件）でも、rtt は 60.2 / 109.0 / 1,306.5、loss は平均 0.7 でほぼ同じ
- ロス率が 0 より大きい行は 141、5% 以上は 22。RTT が 200ms 以上の行は 30
- RTT が 200ms を超えた秒: 09:44:15（768）、09:44:29（784）、09:46:31（598）はどれも1秒だけで、kbps は平常（1,958〜2,511）。09:47:54〜09:48:14 は約20秒続いた（最大 1,306）。09:48:34〜40 も 209〜750
- **先に上がるか**（800 未満が2秒以上続いた区間の、始まりの前10秒）:

| 落ち込み | 前10秒の rtt 平均 / 最大 | 前10秒の loss 平均 / 最大 | 前半全体の平均（rtt / loss） | 読み |
|---|---|---|---|---|
| 09:47:08〜09（385, 786） | 81 / 91 | 0.5 / 2.8 | 116 / 0.8 | 上がっていない |
| 09:47:56〜57（365, 4） | 188 / 606 | 0.2 / 1.4 | 116 / 0.8 | RTT が2秒前（09:47:54）に 95 → 606 へ上がった。kbps は 09:47:55 に 939 へ落ち始めた。ロス率は上がっていない（落ちたあとの 09:47:58 に 22.5%） |

```
09:47:53 kbps=2112 rtt=94.7  loss=0.000
09:47:54 kbps=1931 rtt=605.5 loss=0.000
09:47:55 kbps=939  rtt=605.5 loss=0.000
09:47:56 kbps=365  rtt=889.0 loss=0.000
09:47:57 kbps=4    rtt=796.2 loss=7.419
09:47:58 kbps=1609 rtt=939.1 loss=22.527
09:47:59 kbps=1744 rtt=1229.5 loss=18.325
09:48:01 kbps=2278 rtt=1306.5 loss=0.000
```

main（基準、CSV の `id=main`）:

| 区間 | 行数 | kbps 最小 / 平均 / 最大 | LOW | DOWN |
|---|---|---|---|---|
| 前半 09:37:25〜09:49:24 | 711 | 0 / 3,513 / 15,197 | 1 回（09:48:28〜35） | 2 回（09:48:26〜27、09:48:55〜09:49:23） |
| 前半のうち、落ちる前 09:37:25〜09:48:24 | 652 | 0 / 3,732 / 15,197（5%点 2,730） | 0 回 | 0 回 |
| 後半 09:50:16〜10:00:17 | 594 | 0 / 2,771 / 9,529 | 3 回 | 3 回（09:51:10〜09:52:29 の間） |
| 後半のうち、戻ったあと 09:52:55〜10:00:17 | 438 | 0 / 3,134 / 9,529（5%点 2,136） | 0 回 | 0 回 |

- sub と main が近い時刻に悪くなった所が2つある:
  - 09:47:54〜09:48:14 に SRT の RTT が 600〜1,300ms に上がり、09:47:56〜57 に sub が落ちた。その約30秒後の 09:48:25 に main が止まった（この時 sub は OK）
  - 09:51:09〜09:52:55 に main が止まったりつながったりを繰り返した。その間の 09:52:09〜09:53:05 に、sub が約 600 kbps に下がった
- PC は有線。main は家の Wi-Fi、sub はキャリア回線から家のインターネットの回線を通って PC に届く。2台に共通するのは、家のルーターと回線、PC
- ただし、main が止まった 09:48:25〜09:49:24 の大半の間、sub は 1,600〜2,500 kbps で平常だった。main の止まり方は Android 側か家の Wi-Fi の問題に見えるが、原因は決められなかった（ユーザーに思い当たることはない）

集計に使ったコマンド（`walkstat.ps1`、スクラッチパッドのスクリプト。1日目の `t15stat.ps1` と同じ式に、標準偏差などを足した）:

```powershell
$rows = @(Import-Csv .\t15-walk.csv | Where-Object { $_.id -eq 'sub' -and $_.time.Substring(0,8) -ge '09:37:25' -and $_.time.Substring(0,8) -le '09:49:24' })
$k = @($rows | ForEach-Object { [double]$_.kbps })
$m = $k | Measure-Object -Minimum -Average -Maximum
# 5%点: kbps を並べて (件数-1)*0.05 番目
# 標準偏差: 母標準偏差。変動係数 = 標準偏差 / 平均
# 急落: k[i] < k[i-1] * 0.6（前の秒が 0 より大きいとき）
# 5秒の移動平均: k[i-4..i] の平均の最小
# 800 未満が続いた区間: k < 800 の行が続いた長さ（2秒以上）
# 先に上がるか: 区間の始まりの前10行の rtt_ms と loss_rate の平均と最大
# LOW / DOWN: state が同じ行の続き。待機への切替: scene 列が ⚠回線待機中 に変わった回数
```

```powershell
# 閾値の試算: fail 未満が n 秒続いたら悪い、rec 以上が m 秒続いたら戻る、として sub の kbps を順に見る
# 既定（800 / 5秒、1500 / 10秒）で、実際の LOW（09:52:13〜09:53:15）と同じ結果になることを確かめた
```

本体のログの数え方:

```powershell
$L = <t15log.ps1 -Since '09:34:49' -Until '10:01:49'>   # 資格情報を伏せた行
@($L | Where-Object { $_ -match 'level=WARN' -and $_ -match 'reader is too slow' }).Count
@($L | Where-Object { $_ -match 'decode error' }).Count
@($L | Where-Object { $_ -match 'media source refreshed' }).Count
@($L | Where-Object { $_ -match 'obs reader' })
```

### 1日目と並べた表（sub）

| 区間 | 行数 | kbps 最小 / 平均 / 最大 | 5%点 | 変動係数 | 急落 | 5秒平均の最小 | <800 の行 | 800 未満が3秒以上 | LOW | 切替（1日目は待機、2日目は隠す） |
|---|---|---|---|---|---|---|---|---|---|---|
| 置いたまま SRT（Larix） | 597 | 177 / 2,068 / 2,551 | 1,713 | 16% | 5 回 | 1,333 | 2 | 0 | 0 回 | 0 回 |
| 置いたまま RTMP（It's My Live） | 594 | 807 / 2,095 / 3,777 | 1,610 | 14% | 6 回 | 1,304 | 0 | 0 | 0 回 | 0 回 |
| 歩いた SRT（Larix） | 711 | 4 / 2,063 / 3,733 | 1,484 | 22% | 14 回 | 932 | 7 | 0 | 0 回 | 0 回 |
| 歩いた RTMP（It's My Live、停止ボタンの前） | 418 | 0 / 1,914 / 7,805 | 437 | 47% | 26 回 | 296 | 71 | 3（56秒・3秒・4秒） | 1 回 | 1 回 |

- 歩いた RTMP には、レンジ・冷蔵庫を試した2回と、原因のわからない約 600 kbps の56秒が入っている。外を歩いた揺れだけの数字ではない
- 歩いた RTMP の停止ボタンの後（110 行）は、変動係数 18%、5秒平均の最小 1,691 で、置いたままに近い

### W-3 まちがった切替の判断

判断に使ったもの: (1) MediaMTX の `closed` / `decode error`、(2) `inputs-t15-walk.txt` の2分ごとのメディアソースの状態、(3) kbps の落ち込みの深さと長さ。ユーザーは歩いている間 OBS を見ていない。

| 時刻 | 切替 | 直前の kbps | その時の main / sub | 場所 | 判断 | 判断に使ったもの |
|---|---|---|---|---|---|---|
| 09:48:25 | 待機へ（main DOWN、AutoFail）。09:48:37 に T-A へ戻った | main 1,390 → 0 | main は 09:48:25〜27 に約2秒データが止まり、そのあと 2,395、11,425（たまった分がまとめて届いた）。sub は OK 1,911 | main は家の中 | 正しい（main の映像が止まった）。止まりは約2秒だったが、戻りの10秒の規則で待機は12秒 | (3) |
| 09:48:55 | 待機へ（main DOWN） | main 2,297 → 0, 0, 0 | 09:49:02 `closed: … i/o timeout`。09:49:13 の再接続も 09:49:23 に `i/o timeout`、09:49:31 に送り直し。sub は OK 約 2,000 | 家の中 | 正しい（main の送信が止まった） | (1)(3)。09:49:27 の `inputs` で `Main` が `ENDED` |
| 09:49:27 | sub を隠す（`reason=down`） | sub → 0 | SRT `closed: EOF` | 外。RTMP に替えた | 正しい（替える間。数えない） | (1) |
| 09:51:09 | 待機へ（main DOWN） | main 2,000〜3,500 → 0 | 09:51:17 `closed: … i/o timeout`。そのあと 09:51:55、09:52:15 にも `i/o timeout`、09:52:28 に送り直して 09:52:54 に OK。sub は OK 約 2,000 | 家の中 | 正しい（main の送信が止まった） | (1)(3) |
| 09:52:13 | sub を隠す（`reason=low`）。09:53:15 に戻した | sub 2,423 → 721 → 約 550〜610 が56秒 | `closed` も `decode error` もない。`Sub` の cursor は進んでいた。main はこの時 DOWN / LOW（待機の間） | 不明（ユーザーにもわからない） | 規則どおり（800 未満が56秒続いた）。映像が乱れていたかは確かめられないが、「少し下回っただけ」ではない。まちがいの疑いは低い | (2)(3) |
| 09:57:22 | sub を隠す（`reason=down`）。09:58:27 に戻した | sub → 0 | `closed: stream unpublished` | 停止ボタンに触れた | 正しい（送信が止まった） | (1) |

- 待機への切替（3回）は、すべて main の kbps が0になったため。sub だけ悪いのに待機へ送ったことはない
- 「800 を少し下回っただけで、5秒の後すぐ戻った」切替はなかった
- 逆に、送らなかった（隠さなかった）落ち込み: レンジ・冷蔵庫の3秒と4秒。sub の映像は数秒乱れたかもしれないが、5秒の規則どおり
- 09:49:43〜09:51:07 は、main の `reader is too slow` で OBS が映像を捨てていたが、健康には入らないので、それによる切替はない（警告だけ。W-4）

### W-4 長い時間の様子

- 本体のログ（09:34:49〜10:01:49）:
  - MediaMTX の WARN は100行
    - `reader is too slow`: 99行
      - main: 98行（09:49:43〜09:51:07、OBS の2つのセッション `0f11c055` 47行・6,650 フレーム、`bc3d1fb6` 51行・6,553 フレーム）。main が 09:49:31 に送り直した直後で、09:49:43 に 18,675 kbps のかたまりが届いた所から始まった。この間の main の kbps は平均 2,973（最大 8,929）
      - sub: 1行（09:53:07、3 フレーム。sub が 3,325 kbps に戻った秒）。30秒に3回に届かないので、警告は出ていない
    - `decode error`: 1行（SRT、09:44:04）
  - MediaMTX の ERR は0。本体の WARN は0
  - `media source refreshed`: 6回。すべて main で、`Main` と `cam-main` の2つずつが 09:49:38、09:52:10、09:52:21（main がつなぎ直したとき）
  - `obs reader too slow camera=main count=4`（09:49:45.380）→ `obs reader caught up camera=main count=0`（09:52:08.381、最後の行 09:51:07 から61秒後）
- OBS のメディアソース（`inputs-t15-walk.txt`、2分ごとに14回）:
  - 送っている間は、どのソースも `PLAYING` で cursor が進んでいた
  - `ENDED` だったのは、09:49:27（main が止まり、sub は RTMP に替える間）と 09:57:27（`Sub` と `cam-sub`。停止ボタン）だけ
  - `items T-A` の `cam-sub` が `enabled=false` だったのは、09:49:27 と 09:57:27（どちらも sub を隠していた時）
  - `Main` の cursor は 09:53:27 に 56秒で、09:52:31 ごろに読み直していた（`media source refreshed` のあと）
- OBS のイベント（`obs-t15-walk.txt`）: `MediaInputPlaybackStarted` / `Ended` が `Sub`・`cam-sub` で約107回ずつ、`Main`・`cam-main` で約39回ずつ（送っていない間に OBS がつなぎ直そうとする動き。1日目と同じ）。`MediaInputActionTriggered Main …RESTART` が3回
- メモリ（上の表）:
  - 本体: 66.4〜66.9MB だったのが、09:43 ごろに 34.6MB へ下がり、そのあと 34.6〜35.2MB でほぼ一定
  - 本体の MediaMTX は 39.7〜52.8MB、試験の MediaMTX は 32〜45MB で、増え続けてはいない
  - OBS は 283〜1,066MB の間で大きく上下した。ソースが `ENDED` の時は下がり（09:49:27 に 397.8MB）、つなぎ直すと上がる。最後は 968MB。1日目（554〜693MB）より高い。増え続けているかは、この長さではわからない

### 意見

#### (a) 既定の閾値（800 / 1500、5秒 / 10秒）は、歩いたときのキャリア回線に合っているか

この日の回線では、合っていると考える。変えるなら、ほかの値より `fail_kbps` を 800 のままにして、秒数を短くしないことをすすめる。

- 外を歩いた SRT の12分では、800 未満が続いたのは2秒が2回だけで、LOW の手前（3〜4秒）にも届かなかった。ユーザーの話では、この辺りは電波の悪い所が少ない。歩いても、深くて長い落ち込みは出なかった
- RTMP の後半で LOW になったのは、約 600 kbps が56秒続いた所だけ。これは「少し下回った」ではなく、隠すのは規則どおり
- CSV で試算した（sub、隠す回数。替える間と停止ボタンの分を除く）:

| 閾値 | 前半 SRT | 後半 RTMP（停止ボタンの前） | 後半 RTMP（停止ボタンの後） | 変わる所 |
|---|---|---|---|---|
| 800 / 5秒（今） | 0 | 1（09:52:13〜09:53:15） | 0 | 実際と同じ |
| 500 / 5秒 | 0 | 0 | 0 | 約 600 kbps の56秒を隠さなくなる |
| 1000 / 5秒 | 0 | 1 | 0 | 変わらない |
| 800 / 8秒 | 0 | 1（09:52:16〜） | 0 | 隠すのが3秒遅くなるだけ |
| 800 / 3秒 | 0 | 3 | 0 | レンジ・冷蔵庫の3秒・4秒でも隠す |
| 1000 / 3秒 | 1（09:47:57〜09:48:21） | 3 | 0 | SRT の2秒の落ち込みでも隠す |
| 戻り 1000 / 10秒 | 0 | 1 | 0 | 変わらない |

- 3秒にすると、数秒で戻る落ち込みのたびに sub が隠れて戻り、ばたつきが増える。5秒が無難
- 500 に下げると、約 600 kbps で長く続く「映像は動いているが画質が低い」状態を隠さなくなる。600 kbps の映像を見せてよいかで決めることになる（送るビットレートが約 2,000 kbps の前提なら、800 のままがよい）
- 戻りの 1500 / 10秒: つなぎ直したあとの LOW → OK は、sub で 15〜20秒、main で 9〜24秒だった（1日目は 9〜10秒）。途中で 1500 を下回る秒があると数え直しになるため長くなるが、遅すぎて困る長さではなかった
- 待機への切替は main（LAN）が止まったときだけで、閾値を変えても変わらない（kbps が 0 になっていたため）
- この日は、悪い所の少ない道だった。地下やトンネル、混んだ駅などでの数字は、まだない

#### (b) SRT と It's My Live の RTMP で、揺れ方や切替の起き方に差があったか

- SRT（Larix）: 1〜2秒の鋭い落ち込み（最小 4）はあるが、すぐ戻り、ビットレートが下がったまま続くことはなかった。変動係数 22%（置いたままの 16% より大きい）
- RTMP（It's My Live）: レンジ・冷蔵庫を除いても、約 600 kbps に下がって56秒平らに続いた所があった。送る側がビットレートを下げたような形で、SRT（Larix）にはなかった揺れ方。停止ボタンのあとの2分は、置いたままに近い
- 切替は、どちらも閾値を下回り続けたときと、送信が止まったときだけで、起き方の差は見えなかった
- 前半と後半で道や場所が違い（後半はレンジ・冷蔵庫を試した）、同じ条件で比べられていない。差が回線によるものか、アプリによるものかは決められない

#### (c) SRT の RTT とロス率を、悪くなったことの判断に使えそうか

RTT は少しだけ先に上がることがあるが、それだけで判断に使うのは難しい。ロス率は使えない。

- RTT: 2つの落ち込みのうち、09:47:56 の方は、2秒前に RTT が 95 → 606ms に上がった。09:47:08 の方は、上がっていなかった
- RTT が 600〜800ms に上がっても、kbps が平常のままの秒が3回あった（09:44:15、09:44:29、09:46:31）。RTT だけで悪いと決めると、まちがいが増える
- ロス率: どちらの落ち込みでも、前の10秒は平均 0.2〜0.5% で、全体の平均（0.8%）より低い。ロス率が上がったのは落ちたあと（09:47:58 に 22.5%）
- 使うなら、補助として「RTT が 500ms 以上の状態が数秒続いた」を警告に出すくらい。09:47:54〜09:48:14 は RTT が約20秒高いままで、その間に main も止まった。家の側の何かを示していたかもしれない

#### (d) 新しい警告（W-0）の出方と消え方に、困る所があったか

- 出方: 最初の `reader is too slow` から2.2秒（3行目の直後）で出た。遅くない
- 消え方: 最後の行から60秒で消えた（W-0 で 60.2秒、歩く間は61秒）。指示書のとおり
- 困る所:
  1. **カメラが止まっても、警告が約60秒残る。** W-0 では、main が 09:31:06 に DOWN になってから、警告が消える 09:32:04 まで、止まったカメラについて「読み切れていません」が出ていた。DOWN になったら、すぐ消してもよいと思う
  2. **文の「ハードウェアデコードを使用」を入れてください、がこの日は合っていなかった。** `ctl.exe inputs` では、`Main` と `cam-main` は `hw_decode=true` で、すでに入っていた。すでに入っているときは、その部分を出さない方がよい
  3. **つなぎ直しの直後にも出る。** 歩く間の 09:49:45 の警告は、720p でも、main が送り直した直後のかたまり（18,675 kbps）から約84秒続いた。設定の問題ではなく、つなぎ直しのときに起きる。そのときは文の勧め（ビットレートや解像度を下げる）が当てはまらない
- 文そのものはわかりやすい。健康に入らず、待機へ送らないことも確かめた

### 気になった動き

1. **`walk-log.ps1` が Windows PowerShell 5.1 で動かない。** 起動するとすぐ、`walk-log.ps1:10` の `)` と `walk-log.ps1:7` の `[int]$Minutes = 45,` で構文エラー（`ParserError: UnexpectedToken`）になって終わった。
   - ファイルは BOM なしの UTF-8（先頭が `23 20 E6`）。ほかのスクリプト（`watch-status.ps1` など）は BOM あり（`EF BB BF`）
   - 5.1 は BOM なしのファイルを Shift_JIS として読むので、日本語のコメントが化けて、次の行を巻き込んだと見られる
   - PowerShell 7（`pwsh`）では動いた。ファイルを BOM つきで保存し直せば、5.1 でも動くはず
2. **家の中の main（Android、LAN）が、09:48〜09:53 に何度も止まった。** `i/o timeout` が4回、つなぎ直しが4回。ユーザーに思い当たることはない。そのあと 10:00 までは安定していた。2台が近い時刻に悪くなった所があり（上の main の表のあと）、家のルーターや回線の一時的な問題かもしれないが、決められない。
3. **短い止まりでも、待機が12秒続く。** 09:48:25 の main は約2秒止まっただけだが、戻りの10秒の規則で、待機は12秒（09:48:25〜09:48:37）になった。規則どおり。
4. **`media source refreshed` が2つずつ出る。** 1日目は1回の接続に1つ（`Sub` か `Main`）だったが、2日目は `Main` と `cam-main` の2つずつ出た。どちらも `live/main` を読むソースなので、両方を読み直したと見られる。1日目にどうして1つだったかはわからない。害は見えない。
5. **本体のメモリ。** 起動のあと 66MB 台で、1日目（約 25MB）より大きかった。09:43 ごろに 35MB に下がり、そのあと一定。
6. **OBS のメモリ。** 1日目より高く、上下も大きい（283〜1,066MB）。ソースが止まると下がり、つなぎ直すと上がる。今回の約25分では、増え続けてはいない。
7. **IRL Pro の実際のビットレート。** 720p・3,000 kbps 固定でも、平均は約 3,100〜4,100 kbps で、1日目と同じく設定より高い。

### 片づけ

- 10:01:48 `studio off`、`stream stop`（本体のログ: 10:01:49.354 `stream state streaming=false`）
- 3秒待って 10:01:51 `test restore`
  - 出力: 「試験のシーンとソースを消し、元のプロファイル main とシーン Camera2 に戻した」。準備で控えた名前と同じ
- `profile` は `main`。`scenes` はシーンコレクション `test用`、今のシーン `Camera2`、`T-` のシーンなし
- `test-original.json` は消えた。restore のあとの `warnings` は空
- PID で止めたもの（止める前にコマンドラインを確かめた）:
  - 試験だけの MediaMTX（PID 24788）。21935 の待ち受けはなくなった
  - `watch-status.ps1`（PID 36528）、`ctl.exe watch`（PID 38692）、`walk-log.ps1`（`pwsh`、PID 34288）
  - W-0 の見張り（PID 26460）は 09:34:49 に止めた
- 止めていないもの: 本体（PID 20328）、本体の MediaMTX（PID 2384）、OBS（PID 2416）
- ユーザーに、iPhone と Android の送信を止めてよいこと、iPhone の Wi-Fi を戻してよいことを伝えた
