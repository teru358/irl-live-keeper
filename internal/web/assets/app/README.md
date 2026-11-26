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
