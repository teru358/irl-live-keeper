# 画面に同梱する外のファイルの取り直し

spec v0.20 2章。プロジェクトのビルドでは使わない。開発者が、許可を得て、版を上げるときだけ手で行う。

1. `fetch.py` の `PKGS` の版を直す（出てから2週間以上たった版）
2. 取得（ネットワークを使う。tarball を npm の登録所の `dist.integrity`〔sha512〕と照合して展開するだけで、パッケージの中のプログラムは動かさない）:
   `python3 -I tools/vendor/fetch.py <空の作業フォルダ>`
3. 配置（入口のファイルと LICENSE を手を加えずに置き、`internal/web/assets/vendor/VENDOR.txt` を書き直す）:
   `python3 -I tools/vendor/place.py <作業フォルダ> <リポジトリ>`
4. 古い版のファイルを消し、HTML の import map と `THIRD_PARTY_NOTICES.md` を直す。`go test ./internal/web/`（`TestVendorFilesMatchRecord`）で記録と合うことを確かめる

2026-11-26 に lit-html 3.3.3 と CodeMirror 6 の16パッケージをこの手順で置いた（ユーザーの許可）。
