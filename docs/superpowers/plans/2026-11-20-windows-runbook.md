# Windows 実機での確認の手順（spike ①②④ と T12 の残り）

- 前提: 本体（irl-live-keeper.exe）と MediaMTX（mediamtx.exe）を Windows の配信 PC に置き、同じ PC の OBS と IRL Link で使う
- Linux で済んだこと: `2026-11-20-t12-linux-results.md`（T12 の 1〜16 のうち Windows と IRL Link が要らないもの、spike ① の RTSP の読み出し、② の RESTART が要ること、V-02 と V-09 の前提）。Windows では、Linux と違いうる所と、実機でしか決められない値に絞る
- 結果は、この文書の各項目の「結果」に書き込む（指揮者が spec と定数に反映する）

## 0. 準備

1. 試験用の一式（指揮者が `tmp/windows-test/` に用意する）を、Windows の作業フォルダ（例: `C:\irl-test\`。日本語や空白を含まないパス）にコピーする
   - `irl-live-keeper.exe`（main をクロスビルドしたもの）
   - `mediamtx.exe`（v1.21.4 の windows_amd64。公式リリースを checksums で照合済み）
   - `ctl.exe`（試験用の OBS 操作ツール。イベントの時刻を見るのに使う）
2. OBS の「ツール → WebSocket サーバー設定」でサーバーを有効にし、パスワードを控える（ポートは既定の 4455）
3. 試験用のシーンコレクションを作る（普段のコレクションを壊さないため）。シーンは `Standby`（名前は「⚠回線待機中」などでもよい）、`Camera`、`Camera2`、`Talk`。`Camera` と `Camera2` にメディアソース `IRL` を置く（設定は 1. で決める）
4. 初回起動: `irl-live-keeper.exe` をダブルクリックかターミナルから実行する。`config.yaml` ができ、送信用 URL とパスワードが表示される。`obs.password` に OBS のパスワードを書き、`obs.monitored_scenes` に `Camera` と `Camera2` を書いて、起動し直す
   - Windows のファイアウォールの許可を求められたら、プライベートネットワークだけ許可する（mediamtx.exe と irl-live-keeper.exe）
   - SmartScreen（署名のない実行ファイルの警告）が出たら、「詳細情報 → 実行」
5. 配信先: 配信開始の試験（④、T12 の 13）と、配信中の切替の試験には配信先が要る。Twitch の帯域幅テストモード（OBS の「設定 → 配信」で Twitch を選び、「帯域幅テストモードを有効にする」）を使う。配信は始まるが視聴者には届かない（2026-11-20 のユーザーの方針）

## 1. 起動まわり（Windows でしか確かめられない）

| # | 確かめること | 見るところ | 結果 |
|---|---|---|---|
| W1 | ポートの事前確認。1935 / 8890 / 8554 / 9997 / 8787 がほかのソフトで使われていないか。使われていたら、表示される理由で止まるか。わざと使用中にするには、別の窓で `powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target rtmp` を動かしたまま本体を起動する（`-Target` は rtmp / srt / rtsp / api / admin。Enter で解放）。Linux では 5つとも終了コード 1 と理由が出て、MediaMTX は残らなかった | 起動の表示、終了コード（PowerShell で `$LASTEXITCODE`） | |
| W2 | 送信用 URL の host の自動（LAN の IPv4）。Hyper-V、WSL、VPN、VirtualBox などの仮想アダプターがあると、そちらのアドレスが選ばれないか | 起動の表示の SRT / RTMP の URL。違っていたら `stream.publish_host` に書いて直るか | |
| W3 | `config.yaml` に日本語や絵文字のシーン名（「⚠回線待機中」など）を書いても、OBS のシーンと一致するか | status の `warnings` が空か、切替が動くか | |
| W4 | ログの置き場所（実行ファイルと同じフォルダの `logs\`）に日付ごとのファイルができ、パスワードが出ていないか | `logs\irl-live-keeper-YYYY-MM-DD.log` を開く | |
| W5 | Job Object: タスクマネージャーで irl-live-keeper.exe を「タスクの終了」したら、mediamtx.exe も消えるか | タスクマネージャーの詳細 | |
| W6 | Job Object が使えない場合の警告（入れ子にできない Job の中から起動したとき。VS Code のターミナルなど）。警告が出ても動き、終了時に MediaMTX が残らないか | ログの WARN、タスクマネージャー | |
| W7 | Ctrl+C で終了コード 0、MediaMTX が止まる | ターミナル、タスクマネージャー | |
| W8 | コンソールの「×」で閉じたとき、MediaMTX が残らないか（Windows は閉じるときに数秒しか待たない。Job Object で消えるはず） | タスクマネージャー | |

## 2. spike ① OBS の読み出し URL とメディアソースの設定

メディアソース `IRL` に、本体が表示した OBS 用 URL（`rtsp://obs:…@127.0.0.1:8554/live`）を入れる。「ローカルファイル」は外す。

| # | 確かめること | 結果 |
|---|---|---|
| S1-1 | RTSP で映像が出るか。遅延（スマホの時計を映すなどで目で比べる）。FFmpeg オプションに `rtsp_transport=tcp` を入れる | |
| S1-2 | 比べるために RTMP（`rtmp://127.0.0.1:1935/live?user=obs&pass=…`）と SRT（`srt://127.0.0.1:8890?streamid=read:live:obs:…`）でも試し、遅延と、送信を切って戻したときの再接続の確実さを比べる | |
| S1-3 | 「アクティブになったときに再生を再開」（既定で有効）のままにする。無効にすると、人の切替で映像が戻らない（Linux で確認）ことを Windows でも確かめる | |
| S1-4 | バッファリング（MB）、ネットワークバッファ、「再接続までの遅延」の値を決める | |

決めた URL を spec 8章の OBS 用 URL にする（今は仮に RTSP）。

## 3. spike ② 復帰のときの RESTART と待ち時間

ctl.exe で OBS のイベントを見ながら、送信を止めて戻す。

```
set OBS_WS_ADDR=127.0.0.1:4455
set OBS_WS_PASSWORD_FILE=C:\irl-test\ws-password.txt   ← OBS の WebSocket のパスワードを1行で
ctl.exe watch 60
```

| # | 確かめること | 結果 |
|---|---|---|
| S2-1 | 送信を止めると待機シーンへ。戻すと約10秒で RESTART → 待ち → 戻り先へ。戻ったときに映像がすぐ出るか、黒い画面や止まった絵が見えないか | |
| S2-2 | RESTART から `MediaInputPlaybackStarted` までの時間（ctl の時刻）。Linux では約 1.03秒。何回か測る | |
| S2-3 | 待ち時間（今は仮の1秒）を決める。測った時間に余裕を足した値（候補 1.5〜2秒）。決めた値は `cmd/irl-live-keeper/main.go` の `restartWait` と spec 6章へ | |

## 4. spike ④ 配信開始時の切替

`config.yaml` の `on_stream_start.enabled: true`、`scene: "Talk"` にして起動し直す。`Camera` のまま、OBS の「配信開始」を押す。

| # | 確かめること | 結果 |
|---|---|---|
帯域幅テストモードでは配信の中身を見られないので、配信に切替前のシーンが映る長さは、`ctl.exe watch 60` が出すイベントの時刻の差で見積もる（`StreamStateChanged OBS_WEBSOCKET_OUTPUT_STARTING` / `…_STARTED` と、`SceneChanged Talk`）。OBS は `SceneChanged` をトランジションの終わりに出す（Linux で確認）ので、トランジションの長さも控える。

| # | 確かめること | 結果 |
|---|---|---|
| S4-1 | `STARTING` から `STARTED` までの時間と、`STARTED` から `SceneChanged Talk` までの時間（何回か測る）。`STARTED` で切り替える今の作りで、切替前のシーンが配信に映る長さの見積もりになる | |
| S4-2 | `STARTING` の時点で切替を送れば、`STARTED` より前にシーンが変わるか（本体の作りを変える必要があるので、手で試す: `on_stream_start.enabled: false` にして、ctl.exe watch で `STARTING` が出た直後に別の窓から `ctl.exe scene Talk`）。`STARTING` の間に切替の要求が受け付けられるか（エラーにならないか）も見る | |

結果で spec 6章と 7章の配信状態の写像を見直す。

## 5. T12 の残り（IRL Link と Windows 版 OBS）

| # | 確かめること | 結果 |
|---|---|---|
| T-8 | IRL Link から、回線が悪いときにカメラのシーンへ切り替えると、待機シーンへ戻される。別のカメラのシーンを選んだら、回復でそのカメラへ戻る | |
| T-9 | IRL Link から Talk へ切り替えると何も起きない。自動の待機中に Talk へ切り替えたら、回復しても戻されない | |
| T-10 | 回線が良いときに IRL Link から待機シーンを選ぶと、そのまま残る | |
| T-11 | 実際に使うトランジション（スティンガーなど）で、自動切替が人の切替として扱われない（待機の持ち主が auto になり、回復で戻る） | |
| T-12 | 配信中に上り回線を一時的に切って、OBS の出力が再接続しても、待機の持ち主と戻り先が保たれる | |
| T-13 | 待機シーンの名前（「⚠回線待機中」など）が IRL Link の画面で見えて、待機に入ったことに気づけるか（通知の案 A） | |
| T-14 | IRL Link 側のビットレートによる自動切替を切っておく（二重に切り替わらないように）。入れたままだとどうなるかも見ておく | |
| T-15 | 実際のスマホの配信アプリ（SRT か RTMP）で、屋外に近い回線（モバイル回線など）から送り、kbps の揺れと、誤った切替が起きないか。SRT の RTT とロス率が status に出るか | |

## 6. 終わったら

- 結果をこの文書に書き、指揮者に渡す（spec の 8章の OBS 用 URL、6章の待ち時間、6章と7章の配信開始の写像、README の案内を直す）
- 試験用のシーンコレクションは残しても消してもよい。`config.yaml` には OBS のパスワードが入っているので、共有しない
