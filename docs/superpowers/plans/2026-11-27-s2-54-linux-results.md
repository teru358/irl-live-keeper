# S2-54 区切り4の実機の確認（Linux の分）の結果

- 日付: 2026-11-27 21:24 ごろ
- 対象: main（42a838b。値の一覧、直しが要るソース、`camera.*` を含む）を Linux（画面なし）でビルドした本体。試しのフォルダ（`--dir`）、ポートは 28787 など既定から変えたもの。MediaMTX と OBS はなし（ソースのページの配り方だけを見る）
- 形: 中継 PC の形（`admin.lan: true`）。ソースを1件（`relay-test`、テンプレート `回線 {camera.main.kbps:0} kbps {now:hms}`、自由な CSS で素材の `red.png` を背景に）
- 読んだ所: この PC の LAN のアドレス（番地は記録しない）から `curl` で。別の PC の OBS からの読み込みは、まだしていない（下の「残り」）

## 結果

| # | 確かめたこと | 結果 |
|---|---|---|
| L-1 待ち受け | `admin.lan: true` で `*:28787`（すべてのインターフェース） | 合格 |
| L-2 ソースのページ | LAN のアドレスから `/source/relay-test` が、ログインなしで 200。`Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:`、`X-Content-Type-Options: nosniff` | 合格 |
| L-3 ページが読むもの | `/source/assets/source.js`（200、`application/javascript`）、`/source/relay-test/style.css`（200、`text/css`、自由な CSS が入る）、`/source/files/red.png`（200、`image/png`） | 合格 |
| L-4 SSE | `/source/relay-test/events` で、組み立てた文字（`"回線 0 kbps 21:24:38"`）が1秒ごとに届く | 合格 |
| L-5 管理の API | LAN のアドレスから、ログインなしの `/api/status` と `/api/sources` は 401。管理ページ（`/`）はログインの画面（200） | 合格 |
| L-6 LAN から開かない形 | `admin.lan: false` に戻すと `127.0.0.1:28787` だけで待ち受け、LAN のアドレスからはつながらない（curl の終了コード 7）。`127.0.0.1` からは 200 | 合格 |

## 残り

- **別の PC の OBS から読むこと**: Windows の OBS のブラウザソースで、この PC の本体のソースのページ（LAN の番地）を読む。この PC の ufw の状態（root でないと見られない）によっては、28787 を開ける必要がある。ユーザーの都合のよいときに、Windows の確かめと合わせて行う
