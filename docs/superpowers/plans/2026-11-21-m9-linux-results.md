# M-9（Linux の部分）0a の実機の確認 — 結果（2026-11-21）

- 対象: main の 7b6d290（`go build ./cmd/irl-live-keeper`）。0a（複数のカメラ、地図、待機シーンへ送る動き）
- 環境: 画面のない Linux。OBS Studio 33.0.0（obs-websocket 5.7.6）を Xvfb :77 の上で動かし、設定は専用のディレクトリ（M-0 の `obs-env` を `m9/obs-env/` に写したもの）に閉じ込めた。写した後、`global.ini`（4か所）と `basic.ini`（3か所）、`start-sink.sh` に残っていた M-0 のパスを m9 のものに直した。M-0 の環境のファイルは変わっていない（mtime で確認）。MediaMTX v1.21.4。本体の待ち受けは別のポート（RTMP 11935、SRT 18890、RTSP 18554、API 19997、管理 18787、obs-websocket 14455）。配信先は試験だけの MediaMTX（RTMP 21935）。1935 番（システムの nginx）は使っていない
- カメラ: ffmpeg の testsrc2（x264、約 2,500kbps、640x360）。RTMP は、カメラごとの TCP の中継（31935 main、31936 sub、31937 hand → 11935。速さをファイルで変える）を通した。SRT は直接。帯域を絞る試験は「500kbps に絞る」（LOW の判定は 800kbps 未満）
- OBS のシーン（websocket で作成）: `Standby`、`A`（main_src と sub_src）、`B`（main_src）、`Hand`（hand_src）、`Talk`。メディアソースは `ffmpeg_source`、`is_local_file: false`、`rtsp_transport=tcp`、`restart_on_activate: false`。input は本体が出す OBS 用 URL（カメラごとの path）
- 設定: カメラ3台 `main` / `sub` / `hand`（`on_bad: standby`、hand だけ `recover_sec: 3`）。項目1は `main` の1台の設定（`on_stream_start` は Talk）。項目2以降は3台
- 人の操作は、obs-websocket の `SetCurrentProgramScene` を送る試験用のスクリプト（標準ライブラリだけの Python）と、一部 GUI（xdotool）で行った
- 状態の記録: `/api/status` を 0.4 秒ごとに読んで変化だけ書くスクリプトと、本体のログ。ここに書く時刻は 12:33〜12:51（JST）の実測

## 結果

| # | 項目 | 結果 | 記録 |
|---|---|---|---|
| 1 | 第1段階のやり直し（カメラ1台）: 絞る→待機、戻す→戻る | 合格 | B で main を 500kbps に絞って 7秒で LOW、同じ瞬間に `reason=low` で待機シーンへ（持ち主 `auto`、戻り先 B）。戻して 10秒で OK、1秒後に B へ（`reason=ok`、持ち主 `none`）。送信を止めると約2秒で DOWN、同時に `reason=down` で待機シーンへ。送信し直して LOW → OK（10秒）→ 1秒後に B へ |
| 1 | 手動の待機 ON / OFF | 合格 | ON で待機シーン（持ち主 `manual`、戻り先 B）、OFF で B へ。待機中の2回目の ON は 409「すでに待機シーン」、手動でないときの OFF は 409「手動の待機中でない」 |
| 1 | `on_stream_start` | 合格（一部） | API から配信を始めると、B から Talk へ（`reason=stream_start purpose=StartScene`）。「回線が悪いまま始める」ほうは、この回は確かめていない（第1段階の T12 で通っている）。配信を止めたままだと、LOW になっても切り替わらない（配信中だけ働く。設計どおり） |
| 1 | ログ（`camera changed`、`scene switch` の camera と kbps、パスワード） | 合格 | `camera changed camera=main from=OK to=LOW kbps=500.3…`、`scene switch from=B to=Standby reason=low camera=main kbps=500.3… purpose=AutoFail`。復帰は `purpose=AutoRecover`。ログファイル（`logs/*.log`）と標準出力の `time=` の行に、送信・読み出し・管理・obs-websocket のどのパスワードも出ていない（grep 0 件）。起動時の表示（バナー）には、送信用 URL と OBS 用 URL にパスワードが入る（設計どおり） |
| 2 | A で sub だけを絞る → 待機へ | 合格 | 7秒で `sub OK→LOW`、`scene switch reason=low camera=sub`、戻り先 A。戻して 10秒後に OK、1秒後に A へ |
| 2 | B で sub だけを絞る → 何も起きない | 合格 | sub は LOW（その後 DOWN）になったが、B は B のまま、待機シーンへ行かない（`scene_cameras=['main']`、`scene_health=OK`） |
| 2 | A で main と sub を絞り、順に戻す | 合格 | 両方 LOW で待機へ（戻り先 A）。main だけ戻して OK になっても待機のまま（15秒見た）。sub も戻して OK になった1秒後に A へ |
| 3 | 使わない日のカメラ: Hand へ（hand は未送信）→ 待機 → 送り始める → `recover_sec` 3秒で Hand へ | 合格 | B から人が Hand へ → 約0秒で待機へ（`reason=down camera=hand`、戻り先 Hand）。hand を送り始めて、DOWN → LOW → OK（3秒）→ 1秒後に Hand へ（`reason=ok camera=hand`）。送り始めから Hand が映るまで約9秒 |
| 3 | 先に B へ切り替えてから hand を止める → 何も起きない | 合格 | hand は DOWN になったが、B のまま、ログに scene switch なし |
| 4 | 人が A の sub の項目を非表示にする → sub を絞っても待機へ行かない | 合格 | `SetSceneItemEnabled` で非表示にすると `scene_cameras=['main']` に変わった。sub が LOW でも A は A のまま。表示に戻すと（`scene_cameras=['main','sub']`、sub は LOW のまま）約1秒で待機へ（`reason=low camera=sub`）。sub を戻して A へ戻った |
| 5 | 新しいシーンにカメラの項目を足す → 監視対象になる | 合格 | 非監視の Talk（`scene_class=other`）に main_src を足すと、すぐ `monitored`、`scene_cameras=['main']`。main を絞ると待機へ、戻すと Talk へ戻った。項目を消した後の Talk は、現在のシーンでなかったため、status では確かめていない |
| 5 | メディアソースの URL を別のカメラの path に変える（`InputSettingsChanged`） | 合格 | sub_src の URL を `/hand` に変えると、約1秒で A の `scene_cameras=['hand','main']`、hand が DOWN で待機へ。status の `warnings` に「サブカメラ のメディアソースが OBS に見つかりません」。URL を `/sub` に戻すと `warnings` は空、約1秒で A へ戻った |
| 5 | 戻り先のシーンを消す → 待機に残る | 合格 | D（main_src）で待機に入った後、D を `RemoveScene` すると、待機シーンのまま持ち主 `none`、戻り先なし（`standby changed owner=none return_scene=""`）。main が回復しても待機シーンのまま |
| 6 | グループの中のカメラで判定が効く | 合格 | `GrpSub`（グループ、中に sub_src）と main_src を持つシーン G（JSON から作った `m9g` コレクション）。`scene_cameras=['main','sub']`。sub を絞ると待機へ（`camera=sub`）、戻して G へ |
| 6 | グループの中に項目を足したとき、30秒ごとの読み直しで地図に入る | 合格 | websocket ではグループの中に項目を足せない（`CreateSceneItem` が 602）。GUI（Studio モード、G をプレビューにして、Sources で hand_src を GrpSub へドラッグ）で足した。グループを入れた別のシーン H（`CreateSceneItem` で `GrpSub` を入れた）を program にして見た: ドラッグの7秒後は `scene_cameras=['sub']`（古い）、9秒後に `['hand','sub']`（読み直しで入った）。hand は DOWN なので待機へ |
| 7 | コレクションの切替（websocket） | 合格 | `SetCurrentSceneCollection` で、本体のログに「OBS のシーンコレクションの切替が始まった」→「…終わった」→「OBS に接続した」。切替の間は status が `connected: false`、`warnings` に「OBS のシーンコレクションを切り替え中」と、切替先にカメラのソースがない間はカメラごとの「…のメディアソースが OBS に見つかりません」。切替の前の待機の持ち主（auto、戻り先 H）は `none` に戻った。元のコレクションに戻すと地図を読み直して `A / monitored / ['main','sub']`。配信中のまま切り替えても配信は続いた |
| 7 | コレクションの切替（GUI の「複製」） | 合格 | `Scene Collection > Duplicate`（m9dup）で、ログに「OBS の program のシーンが削除された。シーンコレクションの切替とみなす」→「切替が終わった」→「OBS に接続した」。状態は `A / monitored` のまま読み直された |
| 8 | 受付の停止（`rtmp_enabled: false`） | 合格 | `ss` で 11935 は待ち受けなし（接続は refused）。18890/UDP と 18554 は待ち受け。起動の表示に RTMP の行なし（SRT の3行と OBS 用 URL）。status の `urls[].publish_rtmp` は空。ffmpeg の SRT（`srt://127.0.0.1:18890?streamid=publish:main:irl:…`）で送ると、約2秒で LOW、約11秒で OK、`protocol: srt`、`srt: {rtt_ms, loss_rate, recv_mbps}` が出た |
| 9 | status の形 | 合格 | `cameras[]`（`id`、`name`、`on_bad`、`state`、`kbps`、`online`、`protocol`、`srt`）、`obs`（`connected`、`streaming`、`scene`、`scene_class`、`scene_cameras`、`scene_health`、`standby{owner,return_scene}`、`auto_enabled`、`warnings`）、`mediamtx`、`urls[]`（`id`、`name`、`publish_srt`、`publish_rtmp`、`obs_read`）、`last_error`。カメラごとに出る |
| 10 | OBS を先に止める | 合格 | 本体は動いたまま。ログは「OBS との接続が切れた」→ 約1、2、4、5、5…秒の間隔で「OBS に接続できない」。切断の間、status は `connected: false`、`standby/on` は 503「OBS に未接続」、MediaMTX は `running` のまま。OBS を起動し直すと、起動の約6秒後に「OBS に接続した」→ `B / monitored / ['main']` を読んだ。配信状態は OBS の再起動で `streaming=false` になった（再取得できている） |
| 11 | Ctrl+C（SIGINT）で止める | 合格 | 終了コード 0。MediaMTX は子プロセスも残らず、ポート（11935、18554、19997、18787、18890）は空いた |

## 見つけた不具合・気になる動き（直していない）

1. **本体の切替がまだ確定していない間に、人が別のシーンへ切り替えると、人の切替が無かったことになる（3回中3回）。**
   - 再現: main の送信を止めて DOWN にし、配信を止めた状態で B にいる。配信を始める（`POST /api/stream/start`）。約0.15秒後（本体が B → 待機シーンへ切り替えている最中）に、人が A へ切り替える（`SetCurrentProgramScene A`）。
   - 結果: ログに `scene changed by user` が出ない。最終のシーンは待機シーンで、戻り先は B のまま（A ではない）。人が選んだ A が失われる。回線が回復すると B へ戻る。
   - 配信開始と同時に回線が悪いとき、かつ人が約0.3秒以内に切り替えたときだけ。IRL Link が配信の開始の直後に切り替えると当たりうる。第1段階の項目8（待機中に切り替えると戻り先が変わる）は、待機が確定した後なら正しく動いた（X 試験で A、Hand ともに戻り先が変わった）
2. **OBS と切断している間（本体が OBS の再起動待ち、コレクション切替中）、status の `obs.scene` に、シーンの名前ではなく UUID が入る**（例: `449c7d73-…`）。`scene_class` は `unknown`。T12 では「切断の直前の値のまま」とあった。管理ページに出すなら、名前ではなく UUID が見えてしまう
3. AutoRecover の `scene switch` の `camera=` は、複数のカメラが映るシーンで、最後に回復したカメラではないことがある（A で main、sub が映り、sub が最後に回復したのに `camera=main`。G では `camera=sub`）。どのカメラの値を出すかの規則が読み取れない。ログを読む人が混乱しうる（挙動には影響なし）

OBS 側の動き（本体の不具合ではない）:

- `SetCurrentSceneCollection` は、OBS を起動した後にディスクへ置いたコレクションを知らず、code 600 を返した（OBS を再起動すると一覧に出た）
- GUI で Studio モードを切ると、program が別のシーン（H）に変わった。本体はその切替を人の切替として扱い、待機シーンへ送った
- 中継（TCP の絞り）に溜まったデータは、送信を止めた後も数秒流れるため、DOWN は止めてから約7秒後になることがある（試験の道具の影響）

## 試験の道具（記録用。リポジトリの外）

`/tmp/claude-1000/-home-teru358-project-irl-live-keeper/9ba12fa0-d1e3-40a3-86be-c9d0f6c7da54/scratchpad/m9/`（`bin/ws.py`、`bin/throttle.py`、`bin/watch.py`、`bin/mkscenes.py`、`bin/mkgroup.py`、`bin/env.sh`、`logs/watch.out`、`shots/`）。終了時に、自分で起動したプロセス（本体、MediaMTX 2つ、ffmpeg、中継3つ、監視スクリプト、OBS、Xvfb :77）をすべて PID で止めた。ほかのプロセス（Xvfb :99、システムの nginx 1935）には触れていない。

## まだ確かめていないこと

- 0a の `on_bad: hide`（0b）、Windows 版 OBS のメディアソース、IRL Link との併用
- 回線が悪いまま配信を始める `on_stream_start`（T12 で確認済み）、本体の強制終了（T12 で確認済み）
