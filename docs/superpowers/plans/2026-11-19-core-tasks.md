# irl-live-keeper 第1段階（コア）task 表

- 対象 spec: `docs/superpowers/specs/2026-11-18-core-design.md` v1.4（26ed572）
- 進め方: 詳しい計画文書は書かない。task ごとに TDD（テストを先に書いて赤 → 実装で緑）。spec と食い違ったら、実装で合わせず spec を直して変更履歴に1行足す
- 完了の判定: その task の受入条件のテストが `go test -race ./...` で緑、`go vet ./...` が通る。主張の前に実行結果を貼る
- 依存の向き: 下の task は上の task にだけ依存する（表の「依存」列）

## task 一覧

| # | task | 依存 | 受入条件（spec の章） | テストの完成形 |
|---|---|---|---|---|
| T0 | spike（実機。コードは捨てる） | — | 12章 ①②③ の3つに答えが出て、spec に反映されている | なし。結果は spec の 8章（OBS 用 URL）、6章（`RestartMedia.Wait`）、5章（publisher の識別子のフィールド）と `internal/ingest/testdata/` の JSON |
| T1 | 雛形 | — | `go.mod`（module `github.com/teru358/irl-live-keeper`、`go 1.27`）、3章のディレクトリ、`go test ./...` が空で通る。外部の依存は goobs、yaml.v3、x/sys だけとし、使う task で足す | `go build ./...` と `go test ./...` が通る |
| T2 | config | T1 | 4章の既定値の生成（`*` はランダム、`[A-Za-z0-9_-]`）、読み込み、検証の全項目、`admin.listen` が loopback 以外のときの警告 | 検証の各項目ごとに、通る例と落ちる例を表形式で。生成した値が検証を通ること。警告の有無 |
| T3 | ingest.Judge | T1 | 5章 Judge の遷移（評価順を含む） | 5章「実時刻の例」#1〜#11 の全行と、閾値ちょうどの値 |
| T4 | ingest.Poller | T1、T0③ | 5章 Poller（基準、識別子、SRT の統計、失敗時のフィールド） | 5章の P1〜P4 の全行。録った JSON を `httptest` で返す。タイムアウト。SRT の統計だけの失敗 |
| T5 | switcher: 判定・手動保持・期待 | T3 | 6章の自動切替、手動保持（入る・解ける条件の全部）、期待（登録、期限、`Override`、見分け方）、Pause の直前の役割 | 11章「入口 × 状態」の表のうち Event の入口。11章の Event の列のケースのうち、期待、手動保持、重複の歯止め、配信状態の列 |
| T6 | switcher: Command・承認・配信開始 | T5 | 6章「目的と有効条件」の表、`Approve` の4つの結果、`Replace`、`Skip`、`BatchDone`、配信開始の待ちの2段階、`cmdErr` の型 | 11章の表のうち Command と `Approve` の入口。11章の Event の列のケースのうち、有効条件、置き換え、配信開始、配信の区切り、バッチ内の失敗 |
| T7 | obs ラッパー | T1 | 7章（program だけ、配信状態の写像、トランジションの上書き、応答待ち5秒、`ErrTimeout`） | 単体テストは goobs のイベントから Event への変換関数だけ（`outputState` の写像の全値）。接続と要求は実機で手動確認（T12） |
| T8 | supervisor | T2 | 8章（mediamtx.yml の生成、ポートの事前確認、再起動の待ち時間、ログの置き換え、Job / Pdeathsig） | ゴールデンファイル（全スイッチの明示、RTSP は TCP だけ、MoQ 無効、read に資格情報）。ポートの事前確認。偽の子プロセスで待ち時間 1→2→4…→30 と60秒後の戻し。パスワードの置き換え。Job と Pdeathsig は OS 別のファイルに分け、手動確認（T12） |
| T9 | core（中核ループと実行役） | T4、T6、T7 | 9章（チャネルの決まり、承認、`SceneRequestDone`、`BatchDone`、`Replace` の列の先頭への挿入、Snapshot） | 偽の obs.Client で通しのテスト。応答を遅らせて承認の却下。実行役が詰まっている間も Sample と Command を受け続ける。途中のイベントを省略する列。`ctx` の取り消しで両側が抜ける |
| T10 | web | T9 | 9章 API（Basic 認証、独自ヘッダーと 403、`cmdErr` の型と 503 / 409、status の JSON） | `httptest` で各ステータスコードと JSON の形 |
| T11 | logging と main | T8、T10 | 10章のログ（日付ごとのファイル、`keep_days`、パスワードと URL をファイルに書かない、切替の理由）、起動時の表示（送信用 URL、OBS 用 URL、生成したパスワード）、終了時の MediaMTX の停止 | ログファイルの切替と削除、ファイルにパスワードが出ないこと。main は配線だけなので T12 で確認 |
| T12 | 手動の結合テスト | T11、T0 | 11章「手動の結合テスト」の1〜12 | チェックリストの結果を記録する（Windows と Linux） |

## 進める順序

1. **T1 → T2 / T3 を並行 → T5 → T6**: 実機が要らない純粋な部分から始める。T0 を待たずに進められる
2. **T0（spike）**: ユーザーの都合のよいときに Windows の OBS 実機で行う。T4 と T12 の前に要る
3. **T7、T8 を並行 → T4 → T9 → T10 → T11 → T12**

## 実装の分担（目安）

- 各 task は 2〜3 の受入条件ずつ実装役（サブエージェントか codex）に渡し、worktree で TDD する。指揮者はブリーフ、検収（テストの再実行と diff の確認）、spec との照合、報告を行う
- T5 と T6 は状態機械の中心なので、実装の前に「入口 × 状態」の表（期待値つき）を先に書き、表をレビューしてから実装に入る
- レビューは task のまとまりごとに1回（T2〜T6、T7〜T9、T10〜T11）。codex sol と Claude 系を交互に使う

## 変更履歴

| 日付 | 版 | 変更 | 理由 | commit |
|---|---|---|---|---|
| 2026-11-19 | v1.0 | 初版 | spec v1.4 の確定 | この版の commit |
| 2026-11-19 | v1.1 | 進捗: T1〜T9 は main に取り込み済み（T5・T6 は spec v2 のシーンのモデルで作り直し、d467760）。残りは T10（web）、T11（logging と main）、T12（手動の結合テスト）と、spike ①②④（Windows の OBS 実機） | 進捗の記録 | この版の commit |
| 2026-11-20 | v1.2 | 進捗: T10（web）と T11（logging、main の配線）を main に取り込んだ（spec v2.4）。秘密の置き換えは `internal/redact` に共有し、logging と supervisor が使う。T10〜T11 のまとめのレビューはまだ。残りは T12（手動の結合テスト）と spike ①②④（Windows の OBS 実機）。`restartWait` は spike ② まで仮の1秒、OBS 用 URL は spike ① まで仮に RTSP | 進捗の記録 | この版の commit |
