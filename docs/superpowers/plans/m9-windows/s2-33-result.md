# S2-33 区切り2の実機の確認結果（設定の画面と保存・反映）

- 実施日: 2026-11-24 22:25〜23:38（T-1〜T-11）、2026-11-25 19:19〜19:35（T-12、T-13。前日に中断して再開した）
- 本体: `irl-live-keeper.exe`（2026-11-24 22:11:47、15,732,224 バイト）。ユーザーが `windows-test2` の exe と入れ替えた（`s2-33` のものとハッシュが同じ）
- `occupy-ports.ps1` は前回と同じ中身（ハッシュが同じ）
- 始める前:
  - ユーザーがフォルダを整理し、使わないものを `trash` に移していた（`logs` も `trash\logs` へ）
  - 本体のフォルダに `config.yaml` がなく、`config.yaml.before-s2-33`（11-23 22:28:06）だけがあった。ユーザーに聞いて、名前を `config.yaml` に戻してもらった
  - そのあと私が `config.yaml` を `config.yaml.before-s2-33` に写した（中身は表示していない。ハッシュが同じことを確かめた）
  - 直前の S2-19c の exe は、本体のフォルダには残っていない（`irl-live-keeper_old.exe` は 11-23 19:45 の版）
- config.yaml は表示していない。変わった行は、スクリプトでキーの名前と行番号だけを出して確かめた（値は出していない）。パスワードがログに出ていないかは、値を表示せずに比べた
- config.yaml のつくり（キーの名前だけ）: `admin`（port, lan, password）、`mediamtx`（binary, rtmp_port, srt_port, rtsp_port, api_port, rtmp_enabled, srt_enabled）、`cameras`（id, name, on_bad ×2）、`stream`（app, publish_user, publish_pass, read_user, read_pass, obs_read_host, publish_rtmp_port, publish_srt_port, publish_host）、`obs`（host, port, password, standby_scene）、`on_stream_start`（enabled, scene）、`switch`（auto_enabled, fail_kbps, low_sec, recover_kbps, recover_sec, down_sec）、`log`（dir, keep_days）、`gui`（close_action）。61行、コメント4行
- 本体: 22:25:02 起動（PID 39420）。OBS は PID 62128（22:25:22）、22:25:27 `connected to OBS`
- Windows はダーク

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| T-1 設定の画面 | 合格（気になる点が多い） | 区分と項目、反映の印、パスワードの伏せ字と「表示」、スマホで1列、ライトとダーク。見た目・並び・文言の意見は下の一覧 |
| T-2 すぐ反映 | 合格（気になる点あり） | 確認に「すぐ反映」、再起動しない。config.yaml は値の行だけ変わり、コメントと並びが残った。「読み直す」が効いたかわからない。保存がログに出ない |
| T-3 再起動して反映 | 合格（気になる点あり） | 確認に「再起動して反映」。`restart requested` → `generation stopped` → `restart complete`、MediaMTX が 19350 番で起動し直した。窓に「再起動中」の文は見えなかった |
| T-4 ポートの確かめ | 不合格 | 保存は断られていた（config.yaml は変わらず、再起動もない）が、画面には理由もエラーも出ず、確認の窓が閉じるだけ。欄は 19351 のまま残った |
| T-5 直前の保存を取り消す | 合格（気になる点あり） | 1回目で 1935 番に戻り、2回目で 19350 番に戻った（本体とファイル）。ただ、画面の欄は 19350 のまま変わらない。ログに `restart complete` が出ない |
| T-6 本体を再起動する | 合格 | `restart requested` → `generation stopped` → `restart complete` |
| T-7 元に戻して再起動 | 合格（気になる点あり） | 再起動に失敗し、窓に理由と「元に戻して再起動」が出た。押すと `restore backup requested` で `config.yaml.bak` の中身に戻り、動き出した。ログに `restart complete` が出ない。`restart failed` の文が日本語。管理ページは読み込み直すまで古い値 |
| T-8 LAN から開く | 合格（気になる点あり） | スマホから開いて、`binary` と `dir` は表示だけで「同じ PC からだけ変えられる」の文が出た。スマホから閾値を変えて保存できた。オンにするとき、下の注記のほかに注意は出ない |
| T-9 管理ページのポート | 合格 | 18787 へ移り、ログインし直しになった。窓のボタンも新しいポートで開いた。8787 に戻した |
| T-10 パスワードの作り直し（管理） | 合格（気になる点あり） | PC はログインしたまま、スマホはログアウトになった。「表示」で見えた。作り直しがログに出ない |
| T-11 パスワードの作り直し（読み取り）と OBS | 合格（気になる点あり） | 「再起動して反映」。OBS のメディアソースの URL が新しくなった。ログと警告に URL もパスワードも出ない。書き換えたことがログに出ない |
| T-12 配信中の確認（任意） | 合格（文言に気になる点あり） | 配信中に「再起動して反映」の項目を変えると、配信が切れる注意が出た。「キャンセル」で config.yaml も配信も変わらない。注意の文がわかりにくい |
| T-13 初回のパスワードを決める | 合格（気になる点あり） | 窓に「管理ページを開いてパスワードを決めてください」。「パスワードを決める」画面で決めると、そのまま入れた。`first-run\config.yaml` の `admin.password` が埋まり、コメントが残った。スマホからは開けない。画面のレイアウトが少し崩れる。`mediamtx.exe` がないと WARN が出続ける |

## T-1 設定の画面

- 22:25:35 `admin login source=local auth=passphrase`（窓のボタンから）、22:27:30 `admin login source=lan auth=password`（スマホ）
- スクリーンショット: `T-1-223126.png`（リストと下の注記がくっついている所。ユーザーが撮った）
- ユーザーの言葉（そのまま）:
  - 各項目の下にオレンジ色のアクセントとバーがあるが冗長
  - 項目の設定内容が少ない場合、リストと下部の注記がくっついている
  - カメラのビットレートしきい値が config から取り込まれていない
  - カメラの項目が増えるとスクロールが長くなるので、カメラごとにたためると良い
  - 窓、言語、ログは一つにまとめて良い。言語は右上の設定で切り替えられるので冗長かもしれない
  - 設定反映に再起動が必要なものは＊を付けて、下部注記で良いと思う
  - OBS の待機シーンはシーン一覧を取得して表示できると良い。今は入力欄になっている。待機シーンを削除した時をどうするか
  - 送信の設定項目が分かりづらい
  - カメラと切り替えの設定項目は並んでいるほうがわかりやすい。カメラを設定した後に、待機の設定を行える。設定の並びは後々でも良い
  - MediaMTX の実行ファイルとは？
  - 管理 API / MediaMTX / OBS は並べて良さそう
  - 細かい文言は後で修整で良い
  - VS Code の設定画面のように、設定の一覧が画面のスクロールに追従し、選ぶとそこに飛び、今見ている項目の色が変わる形も良いと思ったが、そこまで項目が多くないのでふさわしくないかもしれない。スクロール追従だけはあっても良いかもしれない
  - カラーモードは問題ない
  - スマホも今のところ崩れてはいない
- 「カメラのしきい値が取り込まれていない」について: config.yaml のキーを見ると、閾値（`fail_kbps` など）は `switch:` の下にだけあり、`cameras:` の下にはない。設定の画面のカメラの閾値の欄は「カメラごとに上書きする値」で、書いていないので空と見られる。空のときは「全体の値（`switch`）を使う」と、その値を示すとよい
- 判断: **合格（気になる点が多い）**

## T-2 すぐ反映

- ユーザーが設定の画面で `switch` の `low_sec` と `down_sec` を変えて保存（2〜3回）、最後に元の値に戻して保存した。「キー名では区別がつかなかったので両方変えてみた」
- ユーザーの言葉: 1.（確認に「すぐ反映」が）「出た」 2.「読み直すボタンが反応しているかわからない」 3.「値は変わっていた。コメントは変わっていない」
- 22:56:28 の保存で `config.yaml.bak`（保存の直前の中身）ができた。`.bak` と今を比べると `low_sec` と `down_sec` の2行だけが違い、今の config.yaml は始める前と同じ中身（61行、コメント4行）
- ログ: この間に `restart requested` は出ていない（再起動しない）。**保存したことも、ログに出ていない**
- 判断: **合格（気になる点あり）**

## T-3 再起動して反映

- ユーザーが `mediamtx` の `rtmp_port` を 19350 にして保存（19350 が空いていることは先に確かめた）
- ユーザーの言葉: 1.「出た」（再起動して反映） 2.「再起動中とは出なかった」 3.「戻ったと思う」
- ログ:

```
23:03:36.731 level=INFO msg="restart requested"
23:03:36.736 level=INFO msg="generation stopped" reason=restart
23:03:36.786 level=INFO msg="restart complete"
23:03:36.790 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
23:03:36.855 INF [RTMP] started with listener on :19350 (TCP/RTMP)
```

- MediaMTX は PID 60772 で 19350 番を待ち受けた。config.yaml は `rtmp_port` の1行だけ変わった
- 判断: **合格（気になる点あり）**。再起動は約0.05秒で終わり、窓の「再起動中」の文は見えなかった

## T-4 ポートの確かめ

- 23:05:04 `occupy-ports.ps1 -Target rtmp -Rtmp 19351` を裏で起動（PID 46292）。19351 を PID 46292 が待ち受けていることを確かめた
- ユーザーが `rtmp_port` を 19351 にして「保存」→「反映する」
- ユーザーの言葉: 「保存は通ったような印象です。理由は出ていません」「保存→反映するのあと、窓が閉じただけのような感じです。エラー表示は有りません」「欄は 19351 のままです」
- config.yaml の指紋（ハッシュ）は変わらず（23:03:36 のまま）、本体は再起動せず、MediaMTX は 19350 のまま。ログにも行がない
- 23:08:49 スクリプト（PID 46292）を PID で止めた
- 判断: **不合格**。保存は断られていたが、画面に理由が出ず、欄にも 19351 が残るので、保存できたと思ってしまう

## T-5 直前の保存を取り消す

- ユーザーが「読み直す」で欄を 19350 に戻してから、「直前の保存を取り消す」→「反映する」を2回
- ユーザーの言葉: 1.「19350 になりました」（読み直す） 2.「19350 のままです」 3.「同じく 19350 です」
- ログと本体:

```
23:10:08.180 level=INFO msg="restart requested"
23:10:08.185 level=INFO msg="generation stopped" reason=restart
23:10:08.240 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
23:10:08.301 INF [RTMP] started with listener on :1935 (TCP/RTMP)      ← T-3 の前の値に戻った
23:10:38.800 level=INFO msg="restart requested"
23:10:38.807 level=INFO msg="generation stopped" reason=restart
23:10:38.862 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
23:10:38.917 INF [RTMP] started with listener on :19350 (TCP/RTMP)     ← もう一度で T-3 の値に
```

- 1回目のあと config.yaml は始める前と同じ中身、2回目のあと T-3 のあとと同じ中身（ハッシュで確かめた）
- 判断: **合格（気になる点あり）**
  - 本体とファイルは期待どおりに戻る
  - **画面の欄は、取り消しのあとも 19350 のまま変わらない**（新しい値を読み直していない）
  - 取り消しの再起動では、**ログに `restart complete` が出ない**

## T-6 本体を再起動する

- ユーザーの言葉: 「再起動しました。画面はかわりません」
- ログ: `23:11:40.988 restart requested` → `23:11:40.994 generation stopped reason=restart` → `23:11:41.045 restart complete`。MediaMTX PID 41604、19350 番
- 判断: **合格**

## T-7 元に戻して再起動

- 23:12:26 `occupy-ports.ps1 -Target rtmp -Rtmp 19352` を裏で起動（PID 74616）
- このときの `config.yaml.bak` は、T-5 の取り消しのため、始める前の中身（RTMP 1935）だった
- ユーザーがエディタで `rtmp_port` を 19352 に書き換え、管理ページの「本体を再起動する」
- ユーザーの言葉: 「窓に表示されました」「再起動後、設定は反映されておらず、再読み込みで反映されました」
- ログ:

```
23:14:30.176 level=INFO msg="restart requested"
23:14:30.182 level=INFO msg="generation stopped" reason=restart
23:14:30.190 level=ERROR msg="port precheck failed" error="Cannot use mediamtx.rtmp_port port 19352 (tcp4). Check whether another program (such as a previous irl-live-keeper or mediamtx) is using this port: …"
23:14:30.190 level=ERROR msg="restart failed" error="mediamtx.rtmp_port のポート 19352（tcp4）を使えません。ほかのプログラム（前回の irl-live-keeper や mediamtx など）がこのポートを使っていないか確かめてください: …"
23:15:25.139 level=INFO msg="restore backup requested"
23:15:25.200 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
23:15:25.265 INF [RTMP] started with listener on :1935 (TCP/RTMP)
```

- 元に戻したあと、config.yaml は始める前と同じ中身。MediaMTX PID 78124 が 1935 番。`config.yaml.bak` は 19352 の版になった
- 23:16:42 スクリプト（PID 74616）を PID で止めた
- 判断: **合格（気になる点あり）**
  - 元に戻したあと、**ログに `restart complete` が出ない**（指示書の期待と違う）
  - `restart failed` の error だけ日本語（ほかのログは英語）
  - 管理ページは、読み込み直すまで古い値のまま

## T-8 LAN から開く

- 今の config.yaml では `admin.lan` はもともとオン。注意の文を見るため、オフ → オンにして保存してもらった
- ログ: 23:17:28 と 23:17:41 に `restart requested` → `generation stopped` → `restart complete`（オフとオン）。もう一度試した 23:26:40、23:26:52 も同じ
- 23:19:02 `admin login source=lan auth=password`（スマホ）
- ユーザーの言葉:
  - 「同じ PC からだけ変えられるという表示はあります、入力はできないようです」
  - 「秒数を3にして2に戻しました」（スマホから保存。config.yaml は最後に始める前と同じ中身）
  - 「lan をもう一度試しました。下部に注記は出ますが、それ以外は有りません」
- スマホのスクリーンショット: `IRL Live Keeper.jpeg.png`（ユーザーが `s2-33` に置いた。設定の画面のため、私は開いていない）
- 判断: **合格（気になる点あり）**。オンにするとき、下の注記のほかに注意（確認の窓の中の注意など）は出ない

## T-9 管理ページのポート

- ユーザーが `admin.port` を 18787 にして保存、そのあと 8787 に戻した（18787 が空いていることは先に確かめた）
- ログ: 23:28:22 再起動（18787）、23:28:53 `admin login source=local auth=password`、23:29:09 `admin login source=local auth=passphrase`（窓のボタン）、23:29:25 再起動（8787）
- ユーザーの言葉: 1.「ログインし直しになりました」 2.（窓のボタンで）「開きました」 3.「戻しました、ログインし直しにはなりませんでした」
- 今は 8787 番（`::`）で待ち受け、config.yaml は始める前と同じ中身
- 判断: **合格**

## T-10 パスワードの作り直し（管理）

- ユーザーの言葉: 「PC はログインしたままのようです」「スマホはログアウトしました」「PC 側表示されています」
- config.yaml で変わったのは、`admin` の `password`（4行目）の1行だけ（23:32:47）
- 本体は再起動していない。ログに行はない（23:30:38 の `admin login source=local auth=passphrase` だけ）
- config.yaml のパスワードの値（4つ）は、ログに出ていない
- 判断: **合格（気になる点あり）**。作り直したことがログに出ない

## T-11 パスワードの作り直し（読み取り）と OBS のメディアソース

- OBS が動いている状態で、「読み取りパスワードを作り直す」→「OBS のメディアソースも書き換える」にチェック →「反映する」
- ユーザーの言葉: 「再起動して反映が表示された」「OBS 側のソースに反映された」「（警告に URL やパスワードは）出ていない」
- config.yaml で変わったのは `read_pass`（30行目）の1行だけ
- ログ:

```
23:35:55.564 level=INFO msg="restart requested"
23:35:55.570 level=INFO msg="generation stopped" reason=restart
23:35:55.628 level=INFO msg="restart complete"
23:35:55.632 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
23:35:55.693 INF [RTSP] started with listeners on :8554 (TCP/RTSP)
```

- 新旧のパスワードの値（5つ）も、`rtsp://`・`user=` などの URL も、ログに出ていない
- カメラからは送っていなかったので、映像が戻るかは確かめていない
- 判断: **合格（気になる点あり）**。OBS のメディアソースを書き換えたことがログに出ない

## T-12 配信中の確認（任意）

- 2026-11-25 19:19:30 ユーザーが本体（PID 41316）と OBS（PID 41584）を起動。19:19:42 `connected to OBS`
- 準備:
  - `test-original.json` がないことを確かめた
  - 19:19:57 試験だけの MediaMTX（`dest.yml`）を起動（PID 24404）。`127.0.0.1:21935` で待ち受けを確かめた
  - `ctl.exe scenes` の1行目は `test用`、`profile` は `main`、Studio Mode は off、配信はしていない
  - 19:20:07 `ctl.exe test setup`: 元のプロファイル `main`、シーンコレクション `test用`、シーン `Camera2`
  - `profile` が `IRLKeeper-Test` であることを確かめて、19:20:10 `ctl.exe stream start`。`outputActive true`
- ログ（配信の開始。カメラは送っていないので、本体はすぐ待機へ送った）:

```
19:20:10.628 level=INFO msg="stream state" streaming=true
19:20:10.628 level=INFO msg="scene switch" from=Camera2 to=⚠回線待機中 reason=down camera=sub kbps=0 purpose=AutoFail
19:20:10.961 level=INFO msg="standby changed" owner=auto return_scene=Camera2
19:20:10.963 level=INFO msg="camera hidden" camera=sub items=3 reason=down
19:20:44.876 level=INFO msg="admin login" source=local auth=passphrase
```

- ユーザーが設定の画面で「LAN から開く」（再起動して反映）を変えて「保存」。「変更の確認」に出た文（ユーザーが写した）:

```
再起動して反映

配信が切れ、待機の持ち主と戻り先も消えます。記録中の配信は中断で閉じ、再起動後は新しい記録になります。

LAN から開く — 再起動して反映
```

- ユーザーの言葉: 「『設定の反映のため、アプリを再起動します。』『配信を停止します。シーンの切り替え・待機情報はリセットされます。』等の内容でないと、何がいいたいのかわからない。詳細については実際に行われる処理をみて要検討」「キャンセルでも配信は維持されています」
- 「キャンセル」のあと: config.yaml の指紋は変わらず（11-24 23:35:55 のまま）、本体は再起動せず（MediaMTX PID 41432 のまま）、`outputActive true` のまま
- 確認の窓が赤く表示されたかは、ユーザーから答えをもらっていない
- 片づけ: 19:26:34 `studio off`、`stream stop`（`stream state streaming=false`、`standby changed owner=none`、`camera shown camera=sub items=3 reason=stream_stop`）→ 3秒待って `test restore`（「試験のシーンとソースを消し、元のプロファイル main とシーン Camera2 に戻した」）。`profile` は `main`、`T-` のシーンなし、`test-original.json` なし。試験の MediaMTX（PID 24404）を PID で止めた
- 判断: **合格（文言に気になる点あり）**
  - 「配信が切れ、待機の持ち主と戻り先も消えます」は、何が起きるかがわかりにくい（ユーザー）
  - 「記録中の配信は…」は、記録（履歴）がまだない区切り（指示書のとおり、文言はあとで直す）
  - 試験の配信を始めたとき、`ctl.exe scene T-B` のあとでも、本体の戻り先は `Camera2` になっていた（シーンの切り替えの知らせが届く前に配信が始まったと見られる。この項目には関係ない）

## T-13 初回のパスワードを決める

- 本体のフォルダの本体はユーザーが終了した
- 私が `s2-33\first-run` を作り、`irl-live-keeper.exe` を写した（空のフォルダに exe だけ）
- 19:27:37 ユーザーが `first-run\irl-live-keeper.exe` を起動（PID 38648）。`first-run` に `config.yaml`（5,174 バイト）、`mediamtx.yml`、`logs\` ができた
- 管理ページは `127.0.0.1:8787` だけで待ち受けた（LAN には開いていない）
- 窓の行: `回線 メインカメラ (main) ● DOWN 0 kbps`（カメラ1台）、`OBS ● 未接続 配信停止中`、`待機 なし`、`MediaMTX ● 停止`。隠れたボタンに「元に戻して再起動」もある
- ユーザーの言葉（窓の案内）: 「管理ページを開いてパスワードを決めてください」
- ユーザーの言葉:
  1. 「画面が出てボタンが表示されている。レイアウトの崩れが少しある。スクリーンショットを取った」
  2. 「そのまま入れた」
  3. 「反映されていた、コメントは維持されているようだ」
  4. スマホ（家の Wi-Fi）から `http://192.168.1.16:8787/` は「開きませんでした」
- スクリーンショット: `T-13-193006.png`（パスワードの画面のため、私は開いていない）
- `first-run\config.yaml`: 62 行、コメント4行、19:30:09 に書き換わった。`admin.password` が空でないことを、値を出さずに確かめた
- ログ（`first-run\logs`）:

```
19:27:38.316 level=WARN msg="cannot start MediaMTX" error="Cannot start mediamtx: fork/exec C:\…\first-run\mediamtx.exe: The system c…"
（同じ WARN が 1, 2, 4, 8, 16 秒あけて、そのあと30秒ごとに続いた）
19:29:11.111 level=INFO msg="admin login" source=local auth=passphrase
```

  `cannot connect to OBS` は 67 行（新しい config には OBS のパスワードがない）
- 判断: **合格（気になる点あり）**
  - パスワードの画面のレイアウトが少し崩れる
  - パスワードを決めたことが、ログに出ない
  - `first-run` には `mediamtx.exe` がないので、MediaMTX が起動せず、WARN が30秒ごとに出続ける。窓は「MediaMTX ● 停止」で動き続け、起動の失敗の扱いにはならない。新しく作る config で、MediaMTX をどこで探すかを確かめたほうがよい
  - 起動したばかりのとき、ログのファイルが0バイトで、窓の案内にあった文はログに出ていない

## 片づけ

- 私が起動したもの: `occupy-ports.ps1`（PID 46292、74616）、試験だけの MediaMTX（PID 24404）。どれも PID で止めた
- T-12 の OBS の試験の設定は、`test restore` で元に戻した（プロファイル `main`、シーン `Camera2`）
- 本体のフォルダの config.yaml: ユーザーの指示で、私が `config.yaml.before-s2-33` の中身に戻した（2026-11-25、ハッシュが同じことを確かめた）。戻す前の中身は `config.yaml.after-s2-33` として残した（T-10・T-11 の新しいパスワードが入っている）
  - **注意:** OBS のメディアソースの URL は、T-11 で作り直した読み取りパスワードのまま。戻した config.yaml は元の読み取りパスワードなので、そのままでは OBS が映像を読めない。設定の画面で「読み取りパスワードを作り直す」を「OBS のメディアソースも書き換える」つきでもう一度行えば、そろう
- `s2-33\first-run`（初回の本体と、その config.yaml・ログ）は残してある。最後に見たとき、`first-run` の本体（PID 38648）はまだ動いていた（終了はユーザー）

## 気になった文言と見た目（ここまで）

設定の画面:

1. 各項目の下のオレンジ色のアクセントとバーが冗長
2. 項目が少ない区分で、リストと下の注記がくっつく（`T-1-223126.png`）
3. カメラごとの閾値の欄が空。空なら全体の値（`switch`）を使うことと、その値を示すとよい
4. カメラが増えるとスクロールが長いので、カメラごとにたためるとよい
5. 窓・言語・ログは1つの区分にまとめてよい。言語は右上でも切り替えられるので、設定の画面にあるのは冗長かもしれない
6. 「再起動して反映」の印は、`＊` を付けて下の注記で説明する形でよい
7. OBS の待機シーンは、入力欄ではなくシーンの一覧から選べるとよい。待機シーンを消したときの扱いも決める
8. 送信の設定の項目がわかりにくい
9. カメラと切り替えの項目は並べたほうがわかりやすい（並びはあとでよい）
10. 「MediaMTX の実行ファイル」がわかりにくい
11. 管理 API・MediaMTX・OBS は並べてよい
12. スクロールに追従する設定の目次（VS Code の設定画面のような形）があってもよい
13. `low_sec` と `down_sec` が、名前だけでは区別しにくい
14. 「読み直す」を押しても、効いたかわからない（知らせが出ない）
15. ポートが使えないときの保存で、理由もエラーも出ず、欄に値が残る（T-4）
16. 「直前の保存を取り消す」「元に戻して再起動」のあと、画面の値が古いまま（読み込み直すまで）
17. `lan` をオンにするとき、下の注記のほかに注意が出ない
18. 窓に「再起動中」の文が見えない（再起動が速いため）

配信中の確認（T-12）と初回（T-13）:

22. 配信中の「変更の確認」の文がわかりにくい。「設定の反映のため、アプリを再起動します」「配信を停止します。シーンの切り替え・待機情報はリセットされます」のように、起きることを書く（ユーザーの例。詳しくは実際の処理を見て決める）
23. 「記録中の配信は…」の文（記録はまだない）
24. 「パスワードを決める」画面のレイアウトが少し崩れる（`T-13-193006.png`）
25. `mediamtx.exe` がないとき、WARN が出続けるだけで、窓は「MediaMTX ● 停止」のまま。何をすればよいかの案内がない

ログ:

19. 設定の保存、パスワードの作り直し（管理・読み取り・初回の設定）、OBS のメディアソースの書き換えが、ログに出ない
20. 「直前の保存を取り消す」と「元に戻して再起動」のあと、`restart complete` が出ない
21. `restart failed` の error だけ日本語

## 指示書の誤り・合わない所

1. 「始める前に」の 2 で、私が `config.yaml` を写す手順だったが、始めたとき `config.yaml` がなく、`config.yaml.before-s2-33` だけがあった（ユーザーが名前を変えていた）。名前を戻してもらってから写した
2. 項目の手順がキーの名前（`switch` の `low_sec`、`rtmp_port` など）で書かれていて、設定の画面の表示名と結びつけにくい（ユーザー「キー名の作業指示では分かりづらい」）
3. T-7 の期待「ログに `restore backup requested` と `restart complete`」のうち、`restart complete` は出なかった（本体の側の問題かもしれない）
4. T-8 は `admin.lan` がオフの前提の書き方だが、今の config.yaml ではもともとオンだった。オフ → オンにして試した
5. T-13 は「exe を写す」だけで、`mediamtx.exe` を写す手順がない。そのため MediaMTX が起動せず、WARN が出続けた（指示書は OBS の警告だけを「出てよい」としている）
6. 「片づけ」では config.yaml を戻すのはユーザーとしているが、今回はユーザーの指示で私が戻した
