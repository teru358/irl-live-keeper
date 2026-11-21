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
| T-12 | 配信中に上り回線を一時的に切って、OBS の出力が再接続しても、待機の持ち主と戻り先が保たれる | |
| T-15 | 実際のスマホの配信アプリ（SRT か RTMP）で、屋外に近い回線（モバイル回線など）から送り、kbps の揺れと、誤った切替が起きないか。SRT の RTT とロス率が status に出るか（Tailscale で送るときは `stream.publish_host` に Tailscale のアドレスを書く） | |

## 3. 複数のカメラ（スマホ2台）

`cameras` に2台（`live` と `sub`）を書き、OBS にシーン A（メインとサブ）、B（メインだけ）、Sub（サブだけ）を作る。

| # | 確かめること | 結果 |
|---|---|---|
| C-1 | A にいるとき、サブの送信を止める（またはサブのスマホの回線を悪くする）→ 待機へ。戻すと A へ | |
| C-2 | B にいるとき、サブの送信を止める → 何も起きない（B にサブは映らない） | |
| C-3 | 使わない日のカメラの運用: B にいる → Sub へ切り替える（サブはまだ送っていない）→ 待機へ → サブを送り始める → OK になったら Sub へ戻る。戻すときは、先に B へ切り替えてからサブを止める → 何も起きない | |
| C-4 | OBS で A のサブの項目を手で非表示にする → サブが悪くても A は待機へ行かない | |
| C-5 | 2台を同じ上り回線（同じモバイルルーター）から送ったときの乱れ方と、別の回線から送ったときの違い | |
| C-6 | OBS の負荷: メディアソース2本のデコード（「ハードウェアデコードを使用」の有無で CPU と GPU の使い方を比べる） | |
| C-7 | status（`curl.exe`）で、`cameras` に2台、`scene_cameras`、`scene_health` が出る | |

## 3.5 確認の途中で見つけて直したこと

| # | 見つけたこと | 直し | 結果 |
|---|---|---|---|
| F-1 | OBS を本体より先に起動すると、メディアソースが黒いまま戻らない（起動の直後に1回だけ開こうとして失敗し、試し直さない。`mediastatus` は `PLAYING` で位置が 0 のまま）。RESTART では戻らず、設定の入れ直し（`ctl.exe setinput`）で戻る。一度つながったあとの切断では、OBS が自分で試し直す | メディアソースの見張り（spec v0.13 第3節。f56b2f3）。カメラが届き始めて5秒後に位置が進んでいなければ、設定を入れ直す | 2026-11-21: 合格。OBS → 本体の順に起動し、スマホを送り始める（16:47:45 に受信）→ 16:47:50 に `media source refreshed`（`IRL` と `メディア`）→ OBS は 16:47:54 に `Reconnected`、16:47:56 に回復で Camera へ。映像が問題なく映ることを目で確かめた。そのあとスマホを止めて再開したときは、本体は入れ直さず（ログなし）、OBS が自分で試し直して 16:48:51 に `Reconnected` |
| F-3 | 17:07 の回（192.168.1.11 のスマホ、約 3.8 Mbps）で、MediaMTX が OBS の2つの読み手について `reader is too slow, discarding N frames` を毎秒出し続けた（OK になって5秒後から、配信の終わりまで）。2 Mbps の別のスマホ（192.168.1.17）の回では出ていない。OBS のメディアソースはハードウェアデコードが切れていた（`is_hw_decoding: no`）。スマホは Android の RTMP Streamer（720p、3000 kbps、30fps） | 2つのソースの「ハードウェアデコードを使用」を入れる。README の勧めを強めた | 2026-11-21: 17:15 の回（同じ Android、OK の間の合計約35秒）では警告が出なかった。長い時間の確認は T-15・C-6 で |
| F-2 | ログの kbps が長い小数で読みにくい | 小数1桁に丸める（568cd6b） | 2026-11-21: 合格（`kbps=1827.7`） |

## 4. 終わったら

- 本体を Ctrl+C で止め、`Get-Process mediamtx -ErrorAction SilentlyContinue` に何も出ないことを確かめる
- `config.yaml` には OBS のパスワードが、`ws-password.txt` には OBS の WebSocket のパスワードが入っているので、共有しない。`ws-password.txt` は消す
- 結果をこの手順書の「結果」の欄に書き、気になる動きはメモに残す
