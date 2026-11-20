# T12 手動の結合テスト — Linux の結果（2026-11-20）

- 対象: main（3a6caac 以降）。spec v2.5 の 11章「手動の結合テスト（チェックリスト）」
- 環境: 画面のない Linux サーバー。OBS Studio 33.0.0（obs-websocket 5.7.6）を Xvfb の上で動かし、設定は専用のディレクトリに閉じ込めた。MediaMTX v1.21.4（公式リリース、checksums で照合）。本体の待ち受けは別のポート（RTMP 11935、SRT 18890、RTSP 18554、API 19997、管理 API 18787、obs-websocket 14455）。配信先は試験だけの MediaMTX（RTMP 21935）。送信は ffmpeg（testsrc2、x264）。帯域を絞る試験は、接続を保ったまま速さを変えられる TCP の中継（31935 → 11935）を通した
- 人の操作（IRL Link の代わり）は、obs-websocket の `SetCurrentProgramScene` を送る試験用のツールで行った

## 結果

| # | 項目 | 結果 | 記録 |
|---|---|---|---|
| 1 | RTMP と SRT の kbps と protocol | 通った | RTMP 約 2,700kbps、SRT 約 2,600〜2,800kbps と RTT・ロス率・受信 Mbps |
| 2 | 送信を止めて約2秒で待機シーン、持ち主 `auto` と戻り先 | 通った | 停止から DOWN まで約 3.3秒（`down_sec` 2秒 + 1秒ごとの読み取り） |
| 3 | 低いビットレートで約5秒で待機シーン | 通った | 接続を保ったまま 500kbps に絞ると、800kbps を下回ってから約5秒で LOW、`reason=low` で切替 |
| 4 | 送信を戻して約10秒で戻り先へ、映像が出る | 通った | OK から RESTART、1秒待って切替。メディアソースは RESTART を受けて再生を始めた（下の spike ②） |
| 5 | MediaMTX を kill して再起動 | 通った | 約2秒後に再起動。起動直後の kill で `last_error` が INF の行になる不具合を見つけて直した（3a6caac） |
| 6 | OBS を再起動して再接続、判定が再開 | 通った | 切断の間は API が 503。起動から約7秒で再接続し、シーンと配信状態を取り直した |
| 7 | 手動の待機 ON / OFF | 通った | 回線が良いとき: ON → OFF で元のシーン（Camera、Talk）へ。回線が悪くなってから OFF: 待機シーンのまま持ち主 `auto`、回復で戻った。待機中の2回目の ON と、手動の待機でないときの OFF は 409 |
| 8 | 回線が悪いときにカメラのシーンへ切り替えると待機シーンへ戻される。回復で最後に選んだカメラへ | 通った | Camera2 を選ぶと待機シーンへ戻され、戻り先が Camera2 になった |
| 9 | 監視対象でないシーンへの切替 | 通った | Talk へ切り替えると持ち主 `none`、戻り先なし。自動の待機中に Talk へ切り替えると、回復しても戻されない。回線が悪いまま Talk から Camera へ切り替えると待機シーンへ |
| 10 | 回線が良いときに待機シーンを選ぶとそのまま残る | 通った | 持ち主 `none` のまま残った |
| 11 | 長いトランジションと、シーンごとの上書き | 通った | 5秒のフェードと、8秒の上書き（Standby、Camera）で、本体の切替は本体の切替として扱われた（持ち主 `auto`） |
| 12 | 配信先への出力の再接続で、持ち主と戻り先が保たれる | 通った | 配信先を止めて約11秒後に OBS が再接続。その間、配信は true、持ち主 `auto`、戻り先は保たれた |
| 13 | `on_stream_start` | 通った | OBS から始めても API から始めても開始時のシーン（Talk）へ。回線が悪く開始時のシーンが監視対象（Camera2）なら、続けて待機シーンへ（戻り先 Camera2）。OBS にないシーン（Opening）は接続時に `warnings` に出て、配信開始で失敗し `last_error` に出た。再送はしない |
| 14 | `switch.auto_enabled: false` | 通った | 回線が悪くても切り替わらない。回線が悪いまま ON → OFF で、すぐカメラのシーンへ戻った |
| 15 | 本体の強制終了で MediaMTX が残らない（Linux） | 通った | kill -9 で MediaMTX も終わり、ポートが空いた。Ctrl+C では終了コード 0 |
| 16 | OBS 用 URL から read の資格情報を外すと読めない | 通った | 資格情報なし、誤ったパスワード、publish の資格情報は 401。UDP は 461（TCP だけ） |

Windows でしか確かめられないもの（Job Object、Windows 版 OBS のメディアソース）と、IRL Link との併用は、メイン PC で行う。

## spike に関わる観察（Linux の OBS 33）

- **① 読み出し URL:** OBS のメディアソース（`ffmpeg_source`、`is_local_file: false`、`ffmpeg_options: rtsp_transport=tcp`）は `rtsp://obs:…@127.0.0.1:18554/live` を読めた（`OBS_MEDIA_STATE_PLAYING`）。Windows 版でも確かめる
- **② RESTART:** 送信が止まるとメディアソースは ENDED のままで、RESTART を受けてから再生を始めた。RESTART が要る。RESTART から再生開始（`MediaInputPlaybackStarted`）まで約 1.03秒。仮の待ち時間 1秒はぎりぎりで、ネットワーク越しの Windows では足りないかもしれない。1.5〜2秒を候補に、実機で決める。OBS は1回の RESTART に `MediaInputActionTriggered` を2回出した（本体は使っていない）
- **V-02、V-09 の前提:** 現在のシーンへの `SetCurrentProgramScene` では `CurrentProgramSceneChanged` が出なかった（Camera → Camera、Talk → Talk）
- **イベントの時刻:** `CurrentProgramSceneChanged` はトランジションの終わりに出た（5秒のフェードなら要求の5秒後）
- メディアソースの「アクティブになったときに再生を再開」（`restart_on_activate`、既定で有効）が有効なので、人が IRL Link などでカメラのシーンへ切り替えたときも、映像は新しい送信を読み直した。これを無効にすると、人の切替では映像が戻らないので、README で案内する

## 見つけた小さな点（未対応）

- ログの `scene switch` は承認の時点で書くので、失敗した切替（OBS にないシーンへの StartScene）も切替として残る。続けて失敗の WARN が出る
- シーンの変化を本体の切替とみなしたか、人の切替とみなしたかがログに出ない。結合テストで扱いを確かめにくい
- OBS と切断している間、status の `streaming` と `scene` は切断の直前の値のまま（`connected: false` は出る。spec 6章の、切断前の値を再接続で比べる設計による）
- 回復の途中（DOWN から戻って LOW の間）に配信を始めると、`reason=low` で待機シーンへ切り替わり、kbps は高い値がログに出る。規則どおりだが、ログを読む人には紛らわしい
- 初回起動でポートの事前確認に落ちると、生成した管理 API のパスワードが表示されない（config.yaml にはある）
- status の更新は、実際の変化から 0.5〜1秒遅れる（中核ループの Tick が1秒）
