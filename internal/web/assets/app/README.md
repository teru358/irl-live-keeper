# 管理ページ

状態は store.js の入れ物に集め、get / update(fn) / subscribe を通して扱う。
get は凍結した値を返す。購読通知と描画は queueMicrotask でまとめる。
画面は状態を受け取って lit-html の html を返す関数。root ごとの try/catch で描画の失敗を隔離する。
要素を ID で探して内容を書き換えない。root の取得、スクロール、フォーカスなどの DOM 操作だけを境界に置く。
notice / senderNotice は操作の知らせ、status は SSE の状況専用。SSE は知らせを変更しない。
EventSource 標準の再接続を使う。events.js と api.js と store.js は DOM 非依存。

login.js は設定・再生成の確認窓・スマホ・ログイン・初回パスワードと、メニュー・テーマ・言語の操作を担当する。
API と翻訳は共通 module を使う。keeper-page / keeper-status / keeper-regenerate / keeper-restarting が移行境界。
移行した画面と既存設定のエラー root も分け、SSE の再描画で設定の知らせを消さない。
再生成の確認窓と残りの共通部分は S2-63 で移す。
Node テストは assets 外の jstest に置く。Go から Node を自動起動せず、開発時に明示実行する。
