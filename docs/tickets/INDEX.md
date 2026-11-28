# チケット一覧

自動生成: `ticket.py index` (tickets skill)。手で編集しない。
未完了: 10 件。完了分は closed/。

## 設計待ち (10)

### 不具合 (1)

| id | 優先 | 起票 | 概要 |
|---|---|---|---|
| [html-split-charref](open/html-split-charref.md) | 低 | 2026-11-28 | 文字参照を差し込みや if で割ると、保存できたのに表示で誤りになる |

### 機能 (6)

| id | 優先 | 起票 | 概要 |
|---|---|---|---|
| [source-unknown-name-reason](open/source-unknown-name-reason.md) | 中 | 2026-11-27 | 「直しが要る」と書きかけの誤りに、知らない値の名前を出す |
| [ntfy-notify](open/ntfy-notify.md) | 低 | 2026-11-24 | ntfy で回線の悪化と待機の切り替えをスマホに知らせる（検討） |
| [unused-camera-display](open/unused-camera-display.md) | 低 | 2026-11-25 | 今のコレクションにメディアソースがないカメラを、警告でなく「使っていない」と出す（検討） |
| [source-font-candidates](open/source-font-candidates.md) | 低 | 2026-11-27 | ソースの本文で、PC に入っているフォント名の候補を出す |
| [source-value-meanings](open/source-value-meanings.md) | 低 | 2026-11-27 | 値の一覧の意味の文を分かりやすくする（camera.hidden、時間の書き方） |
| [source-values-open-state](open/source-values-open-state.md) | 低 | 2026-11-27 | 「値の一覧」を開いた状態を覚える |

### 保守 (3)

| id | 優先 | 起票 | 概要 |
|---|---|---|---|
| [integ-overload-failures](open/integ-overload-failures.md) | 低 | 2026-11-23 | 結合のテストが強い過負荷（32本×2回）で INV-03・INV-04・入場の逆順で落ちる |
| [html-nul-code-mismatch](open/html-nul-code-mismatch.md) | 低 | 2026-11-28 | 閉じない属性値の中の NUL で、Go と JS の誤りの code が違う |
| [template-compile-quadratic](open/template-compile-quadratic.md) | 低 | 2026-11-28 | テンプレートの Compile が本文の長さの2乗に近い時間がかかる |

