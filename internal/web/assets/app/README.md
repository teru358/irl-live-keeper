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
insertCondition(condition, body)、setErrors([{line, column, message}])、
destroy()、view。差し込みは選択範囲を置き換え、未選択ならカーソル位置に入れる。
条件には式（例: link.kbps < 800）を渡す。message は API が翻訳した文を渡す。
line/column は1始まり、column は Unicode 文字単位。エラーは現在の文書に対応する
プレビュー応答だけを渡し、編集後には setErrors([]) で古い印を消す。
