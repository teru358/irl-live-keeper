# 管理ページ

状態は store.js の入れ物に集め、get / update(fn) / subscribe を通して扱う。
get は凍結した値を返す。購読通知と描画は queueMicrotask でまとめる。
画面は状態を受け取って lit-html の html を返す関数。root ごとの try/catch で描画の失敗を隔離する。
要素を ID で探して内容を書き換えない。root の取得、スクロール、フォーカスなどの DOM 操作だけを境界に置く。
notice / senderNotice は操作の知らせ、status は SSE の状況専用。SSE は知らせを変更しない。
EventSource 標準の再接続を使う。events.js と api.js と store.js は DOM 非依存。

shell.js はログイン・初回パスワード・メニュー・テーマ・言語、settings.js は設定と再生成の確認窓、phone.js はスマホ用リンクを担当する。
入力値と未保存の基準、欄のエラー、操作の知らせは各 store に保持する。入力には live() を使う。
SSE は設定の状況と保存済み値の再取得だけを扱い、操作の知らせを変更しない。
Node テストは assets 外の jstest に置く。Go から Node を自動起動せず、開発時に明示実行する。

editor.js の mountEditor(container, {kind: 'template' | 'css', value, names,
label, t, onChange}) は Promise でエディターを返す。ソース編集を開いたときに
呼ぶ。この呼び出しまで CodeMirror は読み込まれない。names は値の名前の文字列配列、
label は翻訳済みの欄名、t は既存の UI 文の表を読む関数。
lit-html は空の安定した入れ物（同じテンプレート位置の div）だけを描き、
その子を描かない。描画後に mountEditor を呼ぶ。同じ要素では同じ部品を返す。
画面を閉じるとき destroy()、作り直すときは再び mountEditor を呼ぶ。
入力途中の setValue は避ける（IME のイベントは CodeMirror 標準のまま）。

返り値: getValue()、setValue(text)、insert(text)、insertValue(name)、
insertCondition(condition, body)、insertAsset(name)、setErrors([{line, column, message}])、
destroy()、view。差し込みは、人がカーソルを置いたあとは選択範囲を置き換え、未選択ならカーソル位置に入れる。
まだ置いていないときは末尾に入れる。insertAsset は未操作の CSS では新しい行に完全な背景の規則を入れる。
条件には式（例: cam1.kbps < 800）を渡す。message は API が翻訳した文を渡す。
line/column は1始まり、column は Unicode 文字単位。エラーは現在の文書に対応する
プレビュー応答だけを渡し、編集後には setErrors([]) で古い印を消す。

ソースの編集は previewError / editorError / editorNotice を持ち、一覧の notice と分ける。
sources の SSE によるテスト解除の案内は testNotice に置き、操作の知らせは上書きしない。
プレビューは sandbox="allow-same-origin" の iframe に保持する。reconcile.js の updateRendered で文字列・DOMParser の木を再検査し、検査を通った本文を共通の reconcile で更新する。

settings.js は cameras / settings のページごとに独立した store と確認窓を持つ。
GET の field_locations が項目の所属を決め、settings-pages.js の projectPage で送る値を絞る。
settings-pages.js の rebaseDraft は新しい保存済み値へ差分を当て直し、カメラは id で照合する。
keeper-settings-refresh で両ページがハッシュを更新する（すぐ反映のときだけ）。再起動して反映のときは読みに行かず、
status の apply が applied か reverted になったとき（connection.js の reloadAfterApply）に、両ページの設定と送り先を読み直す。
keeper-draft はメニューの印だけに使う。「直前の保存を取り消す」はない（窓の「元に戻して再起動」と --cli restore-backup だけ）。
connection.js は接続・知らせ・読み直しの DOM を使わない部品（isOnline、setNotice、createSenderLoader など）、
settings-flow.js は設定ページの失敗・保存の終わり・変更なしの判定（Node で偽の部品を渡して試せる）。
api.js の request は、応答のない失敗（fetch の TypeError）を文の表の「本体につながっていません」の文にして network を立てる。
network の知らせは、つながり直したとき（keeper-reconnected）に消す。知らせは、別のページへ移ったとき、同じ欄の次の操作を始めたとき、
元の状態が終わったとき（例: OBS のテストの解除の知らせは配信が止まったとき）に消す。
source-body.js の cssContext は style の中のカーソルの位置（declaration / rule / literal）を返し、素材の差し込みの形を決める。
カメラの送り先は main.js が状態を持ち、settings.js の空の安定した入れ物を root として描く。
