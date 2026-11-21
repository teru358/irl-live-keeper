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
| T-15 | 実際のスマホの配信アプリ（SRT か RTMP）で、屋外に近い回線（モバイル回線など）から送り、kbps の揺れと、誤った切替が起きないか。SRT の RTT とロス率が status に出るか（Tailscale で送るときは `stream.publish_host` に Tailscale のアドレスを書く） || 2026-11-21: 一部。RTMP の接続だけ確かめた: iPhone 15 Pro（It's My Live）をモバイル回線にし、Tailscale（`direct`）で Windows の Tailscale のアドレスへ送ると、つながって約 2 Mbps で OK になった。そのために、今のフォルダの mediamtx.exe の受信を Tailscale（プライベート）で許す規則を足した（もとの規則はパブリックだけだった）。kbps の揺れ、SRT、長い時間はこれから |

## 3. 複数のカメラ（スマホ2台）

`cameras` に2台（`live` と `sub`）を書き、OBS にシーン A（メインとサブ）、B（メインだけ）、Sub（サブだけ）を作る。

2026-11-21 の確認は、Windows の Claude Code のセッションが代理で進めた（指示書と結果は `m9-windows/` の `cam-agent-brief.md`、`cam-result-1.md`〔準備で止まった回〕、`cam-result-2.md`）。カメラは `main`（Android の RTMP Streamer）と `sub`（iOS の It's My Live）。シーンは `ctl.exe test setup` が `test用` に足した `T-A`・`T-B`・`T-Sub`（表の A・B・Sub にあたる）。

| # | 確かめること | 結果 |
|---|---|---|
| C-1 | A にいるとき、サブの送信を止める（またはサブのスマホの回線を悪くする）→ 待機へ。戻すと A へ || 2026-11-21: 合格。sub を止めて約2.5秒で待機へ（return T-A）、sub が OK になって約1秒で T-A へ |
| C-2 | B にいるとき、サブの送信を止める → 何も起きない（B にサブは映らない） || 2026-11-21: 合格。T-B では sub が止まっても何も起きない |
| C-3 | 使わない日のカメラの運用: B にいる → Sub へ切り替える（サブはまだ送っていない）→ 待機へ → サブを送り始める → OK になったら Sub へ戻る。戻すときは、先に B へ切り替えてからサブを止める → 何も起きない || 2026-11-21: 合格。T-Sub にした直後に待機へ（return T-Sub）、sub が OK になると T-Sub へ。T-B に戻してから sub を止めても何も起きない |
| C-4 | OBS で A のサブの項目を手で非表示にする → サブが悪くても A は待機へ行かない || 2026-11-21: 合格（2回目。1回目は Android のアプリの5分の制限で main が止まった）。cam-sub を隠している間は T-A のまま、表示に戻すと待機へ、sub が OK になると T-A へ |
| C-5 | 2台を同じ上り回線（同じモバイルルーター）から送ったときの乱れ方と、別の回線から送ったときの違い || 2026-11-21: 記録のみ。同じ Wi-Fi で60秒、main 平均 3,733 kbps、sub 平均 2,110 kbps、LOW / DOWN なし。片方が下がると片方が上がる様子はなかった（相関 0.14）。別の回線はモバイル回線の用意がなく、できなかった |
| C-6 | OBS の負荷: メディアソース2本のデコード（「ハードウェアデコードを使用」の有無で CPU と GPU の使い方を比べる） || 2026-11-21: 記録のみ。試験のソース2つのハードウェアデコードを切ると、obs64 の CPU が論理コア1つ分の 25% → 31%、GPU のビデオデコードは 13.3% → 11.7%（もとからある IRL と メディア はハードウェアデコードのまま）。`reader is too slow` は on・off とも0件 |
| C-7 | status（`curl.exe`）で、`cameras` に2台、`scene_cameras`、`scene_health` が出る || 2026-11-21: 合格。`cameras` に2台、`scene_cameras` は T-B で [main]、T-A で [main, sub]、`scene_health` はどちらも OK |

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
| F-2 | ログの kbps が長い小数で読みにくい | 小数1桁に丸める（568cd6b） | 2026-11-21: 合格（`kbps=1827.7`） |

## 4. 終わったら

- 本体を Ctrl+C で止め、`Get-Process mediamtx -ErrorAction SilentlyContinue` に何も出ないことを確かめる
- `config.yaml` には OBS のパスワードが、`ws-password.txt` には OBS の WebSocket のパスワードが入っているので、共有しない。`ws-password.txt` は消す
- 結果をこの手順書の「結果」の欄に書き、気になる動きはメモに残す
