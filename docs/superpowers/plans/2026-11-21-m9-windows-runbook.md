# M-9（Windows の部分）: 0a の実機の確認の手順

- 対象: main の `87fca65` 以降のビルド（`tmp/windows-test/irl-live-keeper.exe`。Linux で作った GUI ではない版〔コンソールの版〕。小さな窓は第2段階の区切り1で入る）
- 前回の手順書: `docs/superpowers/plans/2026-11-20-windows-runbook.md`（W1〜W8、T-8〜T-15）。Linux の結果: `docs/superpowers/plans/2026-11-21-m9-linux-results.md`（11項目すべて合格）
- この手順書は、ユーザーと一緒に進める。結果は各表の「結果」の欄に書く

## 0. 準備

1. `tmp/windows-test/` の中身を Windows の PC の試験のフォルダに写す（`irl-live-keeper.exe` を上書きする。`mediamtx.exe` は前と同じ v1.21.4）
2. **`config.yaml` を手で直す**（第2段階と複数のカメラで、古い項目は使えなくなった。残っていると、起動のときに書き直し方を示して止まる）。パスワードは今の値のまま残すので、スマホのアプリと OBS の URL は変わらない

   | 前 | 後 |
   |---|---|
   | `admin:` の `listen: "127.0.0.1:8787"` | `port: 8787` と `lan: false` の2行 |
   | `stream:` の `name: live` | 消す。代わりに、ファイルの最後に下の `cameras:` を足す（`id` は前の `name` と同じ `live` にする。URL が変わらないように） |
   | `obs:` の `media_source: ...` と `monitored_scenes:`（とその下の `- Camera` など） | 消す（どのシーンにどのカメラが映るかは、本体が OBS から調べる） |

   ```yaml
   cameras:
     - id: live
       name: メインカメラ
       on_bad: standby
   ```

   2台目を足すときは、`- id: sub`、`name: サブカメラ`、`on_bad: standby` を続けて書く（0a では `hide` も `standby` と同じ動き）。送信用 URL は、起動の表示にカメラごとに出る
3. 起動して、起動の表示にカメラごとの URL が出ることと、エラーで止まらないことを確かめる
4. OBS（33.0.1。30.1 以降なら対応）: カメラごとにメディアソース（「ローカルファイル」を外し、`input` に起動の表示の OBS 用 URL）を作る。「ハードウェアデコードを使用」を入れる。メディアソースの名前は自由（本体は URL で見分ける）
5. 見張りの窓を2つ開く（どちらも試験のフォルダで。Ctrl+C で止める）
   - **窓 B（本体の状態）**: `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1`（`-Log .\status-c1.txt` でファイルにも残す）。パスワードは同じフォルダの `config.yaml` の `admin.password` を読む。0.5秒ごとに、カメラごとの状態と kbps、配信、シーン、分類、健康、映るカメラ、待機の持ち主と戻り先、警告の数を1行で出す
   - **窓 C（OBS のイベント）**: `powershell -ExecutionPolicy Bypass -File .\watch-obs.ps1 -Out .\watch-c1.txt`（`-Seconds 600` で見張る長さ）。初回は OBS の WebSocket のパスワードを聞かれ、`ws-password.txt` に書く（試験が終わったら消す）。中で `OBS_WS_ADDR` と `OBS_WS_PASSWORD_FILE` を設定して `ctl.exe watch` を動かす
   - 手で一度だけ見るなら: `curl.exe -u admin:<パスワード> http://127.0.0.1:8787/api/status`

## 1. 第1段階の確認のやり直し（カメラ1台）

前回の手順書の W1〜W8 と T-8〜T-11、T-13、T-14 を、新しいビルドとカメラ1台の設定で、ひととおりやり直す（全部でなくてよい。switcher を作り直したので、特に T-8〜T-11 の自動切替と人の切替の見分け、W5・W7・W8 の終了のときに MediaMTX が残らないこと）。

| # | 確かめること | 結果 |
|---|---|---|
| R-1 | W1（ポートの事前確認）、W4（ログ、パスワードが出ない）、W5・W7・W8（終わり方で MediaMTX が残らない） | 2026-11-21: W1 合格（rtmp・srt・rtsp・api・admin の5つとも、理由を出して「Enter キーを押すと閉じます」で止まる）。W5・W7・W8 合格（MediaMTX が残らない）。W4 合格（config.yaml の4つのパスワードを logs\ のすべてのログで探して、見つからない）。気づき: admin のときだけ、止まる前に `OBS に接続できない … operation was canceled` の WARN が1行出る（止めるときの取り消し。害はない）。試験のフォルダの `occupy-ports.ps1` が古く、`admin.listen` と出ていた（scripts の版は直っていた。写し直した） |
| R-2 | T-8〜T-11（IRL Link からの切替、回線が悪いときの戻り先、トランジション） | T-8 2026-11-21: 合格。待機中（戻り先 Camera）に IRL Link で Camera2 → 0.3秒で待機へ戻され、戻り先 Camera2（owner auto）→ スマホを再開、OK の1秒後に Camera2 へ（owner none）。本体を先に起動したこの回は、メディアソースの見張りは入れ直さなかった（OBS が2秒ごとに自分で試し直していた）。T-9 2026-11-21: 合格（一部）。待機中に IRL Link で Talk → owner none、戻り先なし。Talk のままスマホを再開 → LOW まで（OK になる前に止めたので、OK のときに Talk に残ることは今回は見ていない。owner none なので戻す動きはない）。回線が悪いまま Talk から Camera を選ぶ → すぐ待機へ（戻り先 Camera、owner auto）。OBS → 本体の順のこの回も、見張りが入れ直して映った（17:02:52）。T-9 の残り 2026-11-21: 合格。OK のときに Camera から Talk → 12秒 Talk のまま。T-10 2026-11-21: 合格。OK のときに Talk から待機シーン → 10秒そのまま（owner none）。そのあと人が Camera へ。T-11 2026-11-21: 合格。フェード 5000ms で、AutoFail（17:16:04.1）→ 5秒後のトランジションの終わりに owner auto（17:16:09.1）、AutoRecover（17:16:39.1）→ 17:16:44.1 に owner none。本体の切替を人の切替と取り違えない。配信開始時の AutoFail も同じ（17:15:30.6 → 17:15:35.6） |
| R-3 | 回線が悪い → 待機 → 戻る、のログに `camera changed` と `scene switch`（`camera`、`kbps`）が出る | 2026-11-21: 合格。スマホを止める → 2秒で `camera changed OK→DOWN` と `scene switch reason=down camera=main kbps=0`（owner auto、戻り先 Camera）→ 再開 → LOW → OK のあと1秒で `scene switch reason=ok`（owner none） |

## 2. 第1段階の残り

| # | 確かめること | 結果 |
|---|---|---|
| T-12 | 配信中に上り回線を一時的に切って、OBS の出力が再接続しても、待機の持ち主と戻り先が保たれる | 2026-11-21: 合格（Windows の Claude Code のセッションが代理で進めた。指示書と結果は `docs/superpowers/plans/m9-windows/` の `t12-agent-brief.md` と `t12-result.md`。`ctl.exe t12` の命令は、あとで `ctl.exe test`〔tools/obsctl〕に置き換えた）。WAN の線を抜けないので、試験だけのプロファイル（`ctl.exe t12 setup`。配信先は PC の中の試験だけの MediaMTX `127.0.0.1:21935`）で配信し、その MediaMTX を止めた。OBS の `OUTPUT_RECONNECTING`（17:56:39.8）から `OUTPUT_RECONNECTED`（17:57:14.7）までの約35秒、status の全行で `streaming=True`、owner `auto`、return `Camera`。本体のログには何も出ない。そのあとカメラを送ると Camera へ戻った（owner none）。元のプロファイルに戻したことを確かめた。`scene switch` のログから `standby changed` まで約5秒あるのは、T-11 で設定したフェード 5000ms が残っていたため（ログはトランジションの始まり、持ち主の変化は終わり） |
| T-15 | 実際のスマホの配信アプリ（SRT か RTMP）で、屋外に近い回線（モバイル回線など）から送り、kbps の揺れと、誤った切替が起きないか。SRT の RTT とロス率が status に出るか。**回線の分け方: Android（main）は LAN だけ、iOS（sub）を WAN（キャリアのモバイル回線）から Tailscale で送る**。It's My Live の URL のホストだけを Windows の Tailscale のアドレスに替える（`stream.publish_host` は変えない。変えると Android の URL も変わる） || 2026-11-21: 一部。RTMP の接続だけ確かめた: iPhone 15 Pro（It's My Live）をモバイル回線にし、Tailscale（`direct`）で Windows の Tailscale のアドレスへ送ると、つながって約 2 Mbps で OK になった。そのために、今のフォルダの mediamtx.exe の受信を Tailscale（プライベート）で許す規則を足した（もとの規則はパブリックだけだった）。kbps の揺れ、SRT、長い時間はこれから。2026-11-22: SRT は一部。iPhone（LAN、192.168.1.17）から `main` へ。lm-cam: つながる（`is publishing`、2 tracks H264 と音声）が、SRT の段階の受け取りが約 13 kbps しかなく、映像を送り出していない（`media source refreshed` を繰り返し、OBS の読み手は60秒で i/o timeout。CBR にしても同じ。設定に MTU・パケットサイズの項目はなく、PB Key Length は 16/24/32 だけ）。同じアプリの RTMP では映る。It's My Live: つながり、SRT の段階で約 1.2 Mbps 届くが、MediaMTX が `1 track (MPEG-4 Audio)` で受け付け、映像（PID 256）を毎秒約30回 `received data from undeclared track` で捨てる（アプリが始まりに音声だけを宣言する）。本体の kbps は音声の約 67 で LOW のまま。プレビューを待ってから始めても同じ。どちらも本体の側（SRT の受付、認証、status の `srt` の RTT・ロス率・受け取り、kbps）は正しく動いた。アプリと切り離して、Linux で ffmpeg から SRT（mpegts、H264 2.5 Mbps と AAC）で送ると、2 tracks で受け付け、約2秒で LOW、10秒で OK（2669 kbps）、`srt` に RTT・ロス率・受け取り 2.67 Mbps、OBS のメディアソースは PLAYING で cursor が進んだ。Larix Broadcaster（Android、LAN、192.168.1.11）の SRT は合格: 20:05:47 につながり、`2 tracks (H264, MPEG-4 Audio)` で受け付け、約1秒で `DOWN→LOW kbps=2736.1`、10秒で `LOW→OK kbps=2403.2`。status は `protocol=srt`、`kbps` 約 2249、`srt` は RTT 5.3ms、loss_rate 0.76、recv 2.61 Mbps。OBS の画面にも映像が映ることを、ユーザーが目で確かめた。Larix の RTMP は、本体の URL（`/` のあとが1段）を Larix が弾いた（見立て。未確かめ）。屋外の回線での SRT、長い時間はこれから |

## 3. 複数のカメラ（スマホ2台）

`cameras` に2台（`live` と `sub`）を書き、OBS にシーン A（メインとサブ）、B（メインだけ）、Sub（サブだけ）を作る。

2026-11-21 の確認は、Windows の Claude Code のセッションが代理で進めた（指示書と結果は `m9-windows/` の `cam-agent-brief.md`、`cam-result-1.md`〔準備で止まった回〕、`cam-result-2.md`）。カメラは `main`（Android の RTMP Streamer）と `sub`（iOS の It's My Live）。シーンは `ctl.exe test setup` が `test用` に足した `T-A`・`T-B`・`T-Sub`（表の A・B・Sub にあたる）。

| # | 確かめること | 結果 |
|---|---|---|
| C-1 | A にいるとき、サブの送信を止める（またはサブのスマホの回線を悪くする）→ 待機へ。戻すと A へ || 2026-11-21: 合格。sub を止めて約2.5秒で待機へ（return T-A）、sub が OK になって約1秒で T-A へ |
| C-2 | B にいるとき、サブの送信を止める → 何も起きない（B にサブは映らない） || 2026-11-21: 合格。T-B では sub が止まっても何も起きない |
| C-3 | 使わない日のカメラの運用: B にいる → Sub へ切り替える（サブはまだ送っていない）→ 待機へ → サブを送り始める → OK になったら Sub へ戻る。戻すときは、先に B へ切り替えてからサブを止める → 何も起きない || 2026-11-21: 合格。T-Sub にした直後に待機へ（return T-Sub）、sub が OK になると T-Sub へ。T-B に戻してから sub を止めても何も起きない |
| C-4 | OBS で A のサブの項目を手で非表示にする → サブが悪くても A は待機へ行かない || 2026-11-21: 合格（2回目。1回目は Android のアプリの5分の制限で main が止まった）。cam-sub を隠している間は T-A のまま、表示に戻すと待機へ、sub が OK になると T-A へ |
| C-5 | 2台を同じ上り回線（同じモバイルルーター）から送ったときの乱れ方と、別の回線から送ったときの違い（別の回線: Android は LAN、iOS は WAN〔モバイル回線 + Tailscale〕） || 2026-11-21: 記録のみ。同じ Wi-Fi で60秒、main 平均 3,733 kbps、sub 平均 2,110 kbps、LOW / DOWN なし。片方が下がると片方が上がる様子はなかった（相関 0.14）。別の回線はモバイル回線の用意がなく、できなかった |
| C-6 | OBS の負荷: メディアソース2本のデコード（「ハードウェアデコードを使用」の有無で CPU と GPU の使い方を比べる） || 2026-11-21: 記録のみ。試験のソース2つのハードウェアデコードを切ると、obs64 の CPU が論理コア1つ分の 25% → 31%、GPU のビデオデコードは 13.3% → 11.7%（もとからある IRL と メディア はハードウェアデコードのまま）。`reader is too slow` は on・off とも0件 |
| C-7 | status（`curl.exe`）で、`cameras` に2台、`scene_cameras`、`scene_health` が出る || 2026-11-21: 合格。`cameras` に2台、`scene_cameras` は T-B で [main]、T-A で [main, sub]、`scene_health` はどちらも OK |

## 3.4 隠す・戻す（M-13 の Windows、スマホ2台）

2026-11-22 に Windows の Claude Code のセッションが代理で進めた（指示書は `m9-windows/hide-agent-brief.md`、結果は `m9-windows/hide-result.md`）。本体は 2026-11-21 22:38 の版（14,125,568 バイト）。`main`（Android、LAN）は `on_bad: standby`、`sub`（iOS、**全項目を WAN〔モバイル回線〕から Tailscale 経由**）は `on_bad: hide`。シーンは `ctl.exe test setup` の `T-A`・`T-B`・`T-Sub`。

| # | 確かめること | 結果 |
|---|---|---|
| H-1 | T-A で sub を止める → T-A のまま sub の項目を隠す。OK で戻す | 2026-11-22: 合格。約2秒で `camera hidden items=2`（T-A と T-Sub の両方）、`hidden.json` 2件。OK の1秒後に `camera shown reason=ok`、0件 |
| H-2 | T-Sub（全部隠れる）で sub を止める → 待機へ | 2026-11-22: 合格。待機へ（return T-Sub）送ってから隠した。戻るときは表示に戻す → T-Sub の順 |
| H-3 | 人が表示に戻した項目を隠し直さない | 2026-11-22: 合格。20秒以上隠し直さず（`user_shown=True`）、OK で印が消えた |
| H-4 | すべて表示に戻す（`show-all.ps1`） | 2026-11-22: 合格。`200 {"ok":true}`、`reason=manual_all`。sub が悪いままなので 2.55秒後に隠し直した（指示書の「5秒ほど」は目安の書き違い。期待を応答から2秒残す規則のとおり） |
| H-5 | 配信の停止で戻す。始め直すと隠す | 2026-11-22: 合格（`reason=stream_stop`） |
| H-6 | Ctrl+C で戻す | 2026-11-22: 合格（`reason=shutdown`、`hidden.json` 0件） |
| H-7 | 本体が落ちたとき（タスクの終了）| 2026-11-22: 合格。隠れたまま、`hidden.json` 2件で残り、起動し直した本体が引き継いで OK で戻した。本体の MediaMTX も一緒に消えた。引き継いだことを示すログの行がなかった → 起動のときに `隠した項目の記録を読んだ items=N` を出すようにした |
| H-8 | Studio Mode（`studio_mode_hide` の既定 `standby`） | 2026-11-22: 合格。warnings に Studio Mode の文、sub の不調で隠さずに待機へ、OK で T-A へ。ただし Studio Mode を切るときの動きで F-9 を見つけた |

結果のファイルの「気になった点」のうち、設計どおりのもの:

- 人が表示に戻した sub の項目は、sub が DOWN のままでも映り、待機へも行かない（人の判断を変え直さない。hide のカメラは、全部隠れるシーンでなければ健康を悪くしない）
- 本体が隠している間の `scene_cameras` は `[main, sub]`（カメラの spec 第3節: 本体が隠した項目は「映る」では表示として、「実際に表示」では非表示として数える）。C-4 で ctl が手で隠したときの `[main]` とは数え方が違う
- 起動し直した直後に、猶予なく待機へ移った（本体の MediaMTX が一緒に止まり、スマホの接続が切れていたため。配信の画面はすでに止まっていた）

## 3.6 シーンごとの逃がし先（0c、スマホ2台）

2026-11-22 に Windows の Claude Code のセッションが代理で進めた（指示書は `m9-windows/fallback-agent-brief.md`、結果は `m9-windows/fallback-result.md`）。本体は 2026-11-22 20:16 の版（14,224,384 バイト）、`config.yaml` に `scene_fallbacks: [{scene: T-A, fallback: T-Sub}]`。main（Android、LAN、RTMP）は `on_bad: standby`、sub（iOS、キャリア回線から Tailscale、RTMP）は `on_bad: hide`。Linux の確かめは `2026-11-22-0c-linux-results.md`（L-1〜L-13 合格）。

| # | 確かめること | 結果 |
|---|---|---|
| W0c-0 | `test setup` の前の名前の警告と、あとで消えること | 2026-11-22: 合格 |
| W0c-1〜4 | T-A で main が悪い → T-Sub、sub も悪い → 待機、sub が戻る → T-Sub、main も戻る → T-A（stack の変化） | 2026-11-22: 合格 |
| W0c-5 | 逃がしている間の人の切替（T-B） | 2026-11-22: 合格 |
| W0c-6 | 手動の待機の ON → OFF（main は悪いまま。OFF で T-Sub へ） | 2026-11-22: 合格（`standby.ps1` は `-Config` を渡して動かした。下の F-10） |
| W0c-7 | 自動の待機中の ON の特例（2台が戻っても待機のまま） | 2026-11-22: 合格。OFF から切替のログまで約1秒（戻る前のメディアソースの再起動の待ち。W0c-6 の T-Sub への OFF は約20ms） |
| W0c-8 | sub だけ悪い → 隠す（0b のまま） | 2026-11-22: 合格 |
| W0c-9 | 2台が同時に悪くなる（記録のみ） | 2026-11-22: T-Sub を経ずに待機へ直接、stack `["T-A","T-Sub"]`。`camera hidden`（T-A の sub）が待機への切替の確認より約0.3秒早かった。T-A には main が映ったままなので「今のシーンを空にしない」に当たらず、規則どおり（切替を送ってから OBS のトランジションが終わるまでの間に隠した） |

## 3.5 確認の途中で見つけて直したこと

| # | 見つけたこと | 直し | 結果 |
|---|---|---|---|
| F-1 | OBS を本体より先に起動すると、メディアソースが黒いまま戻らない（起動の直後に1回だけ開こうとして失敗し、試し直さない。`mediastatus` は `PLAYING` で位置が 0 のまま）。RESTART では戻らず、設定の入れ直し（`ctl.exe setinput`）で戻る。一度つながったあとの切断では、OBS が自分で試し直す | メディアソースの見張り（spec v0.13 第3節。f56b2f3）。カメラが届き始めて5秒後に位置が進んでいなければ、設定を入れ直す | 2026-11-21: 合格。OBS → 本体の順に起動し、スマホを送り始める（16:47:45 に受信）→ 16:47:50 に `media source refreshed`（`IRL` と `メディア`）→ OBS は 16:47:54 に `Reconnected`、16:47:56 に回復で Camera へ。映像が問題なく映ることを目で確かめた。そのあとスマホを止めて再開したときは、本体は入れ直さず（ログなし）、OBS が自分で試し直して 16:48:51 に `Reconnected` |
| F-3 | 17:07 の回（192.168.1.11 のスマホ、約 3.8 Mbps）で、MediaMTX が OBS の2つの読み手について `reader is too slow, discarding N frames` を毎秒出し続けた（OK になって5秒後から、配信の終わりまで）。2 Mbps の別のスマホ（192.168.1.17）の回では出ていない。OBS のメディアソースはハードウェアデコードが切れていた（`is_hw_decoding: no`）。スマホは Android の RTMP Streamer（720p、3000 kbps、30fps） | 2つのソースの「ハードウェアデコードを使用」を入れる。README の勧めを強めた | 2026-11-21: 17:15 の回（同じ Android、OK の間の合計約35秒）では警告が出なかった。長い時間の確認は T-15・C-6 で |
| F-4 | C-1〜C-7 の準備で、`ctl.exe test setup`（試験のプロファイルを作った約0.9秒後に、試験のシーンコレクションを WebSocket の `CreateSceneCollection` で作る）のあと、Windows の OBS 33.0.1 の WebSocket が `NotReady (207)` のまま約2分半戻らなかった（OBS のログでは、シーンコレクションの作成は終わっていた）。そのあと手で切り替えると、OBS が落ちた。Linux の OBS 33.0.0 では同じ手順を何度くりかえしても起きなかった。記録は `m9-windows/cam-result-1.md` | 試験の道具の側で、シーンコレクションを WebSocket で作ったり切り替えたりするのをやめる（作り直し中）。本体は、切替の始まりのあと30秒で読み直しに入り、NotReady の間も落ちず、OBS が起動し直すと5秒でつなぎ直した（設計どおり） | |
| F-5 | Android の RTMP Streamer は無料の版で、約5分ごとに送信が止まる（C-4 のやり直しと、C-6 の1回目の測りの捨てはこのため） | 試験の計画で考える（長い確認は iOS か、制限のないアプリで） | |
| F-6 | AutoRecover の `scene switch` のログの `camera` が、回復のきっかけのカメラでなく、戻り先に映るカメラのうち kbps がいちばん低いもの（19:03:17、main が OK に戻ったのに `camera=sub`）。M-8 で決めた形だが、読み手には分かりにくい | 回復のきっかけ（戻り先に映るカメラのうち、最後に OK になったもの）にした（カメラの spec v0.14、ユーザーの裁定） | テストで確かめた（実機はまだ） |
| F-7 | 試験の道具: `ctl.exe test restore` が、program に出ていた試験のシーンをトランジションの途中で消し、本体がシーンコレクションの切替とみなして読み直した（害はない）。`ctl.exe watch` を PowerShell の `>` で書くと、止めたときに最後の行が欠けた | restore はトランジションの長さ + 1秒待ってから消す。watch はファイルを引数で渡すと直接書く（tools/obsctl） | Linux で確かめた |
| F-8 | iPhone の配信アプリで、配信中に縦と横を切り替えると、OBS の映像の縦横の比率がうまく追従しないことがある（本体はメディアソースの位置と大きさに触れない。OBS の側の動き） | 既知の症状として保留（2026-11-21 のユーザーの裁定）。境界ボックスは、もとから「内側に合わせる」で、原因ではなかった。配信の前にアプリで向きを固定する。崩れたら OBS でソースのプロパティを開いて OK（開き直す）。屋外からはボタンを押せないので、開き直すボタンは作らない。解像度の変化を本体が見つけて自動で開き直す案は、MediaMTX から読めるかがわかってからの候補 | |
| F-9 | Studio Mode を切ると、OBS が program を一瞬 preview のシーンにしてから戻す（`CurrentProgramSceneChanged <preview>` → `CurrentProgramSceneChanged <program>` → `StudioModeStateChanged false` の順に、同じミリ秒で届く。Windows の H-8 の片づけと、Linux の OBS 33.0.0 で確かめた）。本体は2つとも人の切替と受け取る。**本体が自動で待機にいて（持ち主 `auto`）、preview が健康の良い監視対象のシーンのとき、持ち主が `none` になり、カメラが戻っても待機のまま戻らない**（Linux で再現: return A、preview H、Studio Mode を切る → `scene changed by user Standby→H`、`H→Standby`、owner `none`、main が OK になっても Standby のまま）。preview が悪いシーンのときは、本体がすぐ待機へ送り直したが、戻り先が preview のシーンに置き換わり、カメラが戻ると元のシーンでなく preview のシーンへ戻った（承認とイベントの順しだいでは持ち主 `none` で止まる）。preview が program と同じでも、同じシーンへの切替が2回届き、持ち主 `none` で止まった。手動の待機でも持ち主と戻り先が消え、待機の OFF で戻らなかった。イベントの順は `CurrentProgramSceneChanged <preview>` → `<元の program>` → `StudioModeStateChanged false`（数ミリ秒の間） | カメラの spec v0.17: obs の層で、Studio Mode の間の program の切替を 250ms 取っておき、`StudioModeStateChanged false` が続いて元のシーンに戻っていれば捨てる。あわせて、屋外配信では Studio Mode を勧めない（2026-11-22 のユーザーの裁定）。実装は 1d89661、4016dc1 | 2026-11-22: Linux で合格。自動の待機で preview が Standby・H・B、手動の待機で preview が H の4つで `studio mode off bounce ignored`、持ち主と戻り先が残り、カメラが戻る（手動は OFF）と A へ戻った。Studio Mode の中の人の切替は、ふだんどおり人の切替として届いた。2026-11-22: Windows で合格（`m9-windows/studio-agent-brief.md`、`studio-result.md`。exe は 14:35 の版 14,135,808 バイト、sub は Tailscale、main は LAN）。S-1〜S-3（preview が T-Sub、⚠回線待機中、T-B）で `studio off` のたびに `bounce ignored` が1行、`scene changed by user` は出ず、owner `auto` return `T-A` のまま、main が戻ると3つとも T-A へ（S-3 も T-B でなく T-A）。S-4 の Studio Mode の中の人の切替は2回とも人の切替として届いた。S-5 で起動のときに `隠した項目の記録を読んだ items=2` が出た |
| F-10 | `standby.ps1` が Windows PowerShell 5.1 で `Join-Path : Cannot bind argument to parameter 'Path' because it is an empty string.` で止まった（param の既定値の中の `$PSScriptRoot` が空。`[Parameter(Mandatory)]` を付けた形で起きた見立て） | 4つのスクリプト（show-all、standby、watch-status、watch-obs）の既定のパスを、param の既定値でなく本文で決める | |
| F-2 | ログの kbps が長い小数で読みにくい | 小数1桁に丸める（568cd6b） | 2026-11-21: 合格（`kbps=1827.7`） |

## 4. 終わったら

- 本体を Ctrl+C で止め、`Get-Process mediamtx -ErrorAction SilentlyContinue` に何も出ないことを確かめる
- `config.yaml` には OBS のパスワードが、`ws-password.txt` には OBS の WebSocket のパスワードが入っているので、共有しない。`ws-password.txt` は消す
- 結果をこの手順書の「結果」の欄に書き、気になる動きはメモに残す
