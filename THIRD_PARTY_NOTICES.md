# 第三者ソフトウェアのライセンス

irl-live-keeper（MIT License）は、次のソフトウェアを利用しています。配布用の zip には、各ライセンスの全文を同梱します（第4段階で、ビルド時に依存関係から自動で集める予定）。

## 同梱するプログラム: MediaMTX

| 項目 | 内容 |
|---|---|
| ソフトウェア | MediaMTX（RTMP・SRT などを受けて中継するサーバー） |
| 版 | v1.21.4（2026-11-15 公開。タグの commit `e65f9411a56b19076a0dfde1fb4a59414bdf642c`） |
| ライセンス | MIT License（SPDX: `MIT`） |
| 著作権表示 | Copyright (c) 2019 aler9 |
| ソースの入手先 | https://github.com/bluenviron/mediamtx （この版のソース: https://github.com/bluenviron/mediamtx/tree/v1.21.4） |
| 配布物の入手先 | https://github.com/bluenviron/mediamtx/releases/tag/v1.21.4 |
| ライセンスの全文 | このリポジトリの [`licenses/MediaMTX-LICENSE.txt`](licenses/MediaMTX-LICENSE.txt)（https://github.com/bluenviron/mediamtx/blob/v1.21.4/LICENSE をそのまま写したもの） |

同梱する公式の配布物と、公式の `checksums.sha256`（https://github.com/bluenviron/mediamtx/releases/download/v1.21.4/checksums.sha256 ）にある SHA-256:

| 同梱する先 | 公式の配布物 | SHA-256 |
|---|---|---|
| Windows の zip | `mediamtx_v1.21.4_windows_amd64.zip` | `835645b2700ffce4d2b6cb04bfa0aadce95335da6dfc0d7c2114e6da585734f8` |
| Linux の版 | `mediamtx_v1.21.4_linux_amd64.tar.gz` | `be0c05d9c8af8338116977e7f1c3fcb409fbf882075e5ab075cf8baad76a59d2` |

- **同梱の形:** 上の公式の配布物を、配る zip を作るときに取ってきて、SHA-256 が上の値と同じことを確かめてから、中の `mediamtx(.exe)` を改変せずに入れる。値が違えば zip を作らない。MediaMTX の設定ファイル（`mediamtx.yml`）は、irl-live-keeper が起動のたびに作る（公式の配布物の `mediamtx.yml` は入れない）
- **ライセンスの表示:** 配る zip に `licenses/MediaMTX-LICENSE.txt`（著作権表示と許諾文の全文）を必ず入れる
- **別のプログラムであること:** irl-live-keeper は MediaMTX を子プロセスとして起動し、API で通信するだけ。MediaMTX のソースコードは irl-live-keeper に組み込んでいない。irl-live-keeper も MediaMTX も MIT なので衝突せず、MediaMTX のライセンスの条件は irl-live-keeper のコードには及ばない
- **MediaMTX のバイナリに組み込まれた依存:** MediaMTX の実行ファイルには、MediaMTX 自身が依存するライブラリ（Apache-2.0、BSD、MIT など）が組み込まれている。MediaMTX の公式リリースは `LICENSE` だけを付けて配布しており、irl-live-keeper も公式の配布物を改変せずに同梱する。依存のライセンスは、配る zip を作るときに MediaMTX v1.21.4 の `go.mod` から集めて `licenses/mediamtx-deps/` に添える（GPL 系の依存がないことも、そのときに確かめる。α版の準備 S2-96）
- **名前:** 「MediaMTX」の名前は、同梱しているプログラムを示すためだけに使う。irl-live-keeper は MediaMTX の作者による公式のソフトウェアではなく、MediaMTX の作者が irl-live-keeper を推奨・保証しているかのような表現はしない
- **コーデックについて:** MediaMTX は受け取った映像を中継するだけで、映像の符号化や復号はしない（irl-live-keeper も同じ）
- **版を上げるとき:** この表の版・commit・SHA-256 と `licenses/MediaMTX-LICENSE.txt` を、同じ commit で新しい版のものに替える

## 実行ファイルに組み込まれる Go のライブラリ

| モジュール | 版 | ライセンス |
|---|---|---|
| github.com/andreykaipov/goobs | v1.11.1 | Apache-2.0 |
| gopkg.in/yaml.v3 | v3.0.1 | MIT と Apache-2.0 |
| github.com/rodrigocfd/windigo | v0.2.7 | MIT |
| golang.org/x/sys | v0.50.0 | BSD-3-Clause |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause |
| github.com/buger/jsonparser | v1.6.1 | MIT |
| github.com/hashicorp/logutils | v1.0.0 | MPL-2.0 |
| github.com/mitchellh/mapstructure | v1.5.0 | MIT |
| github.com/mmcloughlin/profile | v0.1.1 | BSD-3-Clause |
| github.com/nu7hatch/gouuid | v0.0.0-20131221200532-179d4d0c4d8d | MIT |
| Go の標準ライブラリ | 1.27 | BSD-3-Clause |

- **MPL-2.0 の扱い:** github.com/hashicorp/logutils（goobs の依存）は改変せずに使っている。MPL-2.0 はバイナリで配布するときに、その部分のソースコードの入手先を示すことを求める。ソースコードは https://github.com/hashicorp/logutils から入手できる
- **Apache-2.0 の扱い:** goobs などの Apache-2.0 のライブラリは、ライセンス全文と、NOTICE ファイルがあればその内容を同梱する


## 画面に同梱する JavaScript ライブラリ

| ソフトウェア | 版 | ライセンス | 著作権者 | 同梱ファイル |
|---|---|---|---|---|
| qrcode-generator | 2.0.4 | MIT | Copyright (c) 2009 Kazuhiko Arase | `internal/web/assets/vendor/qrcode-generator-2.0.4.js` |
| lit-html | 3.3.3 | BSD-3-Clause | Copyright (c) 2017 Google LLC | `internal/web/assets/vendor/lit-html-3.3.3/`（`lit-html.js`、`directive.js`、`directive-helpers.js`、`directives/live.js`・`repeat.js`・`class-map.js`） |
| @codemirror/state | 6.7.6 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-state@6.7.6.js` |
| @codemirror/view | 6.43.15 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-view@6.43.15.js` |
| @codemirror/language | 6.13.1 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-language@6.13.1.js` |
| @codemirror/streamparser | 6.0.0 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-streamparser@6.0.0.js` |
| @codemirror/commands | 6.11.1 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-commands@6.11.1.js` |
| @codemirror/autocomplete | 6.20.4 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-autocomplete@6.20.4.js` |
| @codemirror/lint | 6.9.7 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-lint@6.9.7.js` |
| @codemirror/lang-css | 6.3.1 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/codemirror-lang-css@6.3.1.js` |
| @lezer/common | 1.5.5 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/lezer-common@1.5.5.js` |
| @lezer/highlight | 1.2.5 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/lezer-highlight@1.2.5.js` |
| @lezer/lr | 1.4.12 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/lezer-lr@1.4.12.js` |
| @lezer/css | 1.3.9 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/lezer-css@1.3.9.js` |
| style-mod | 4.1.4 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/style-mod@4.1.4.js` |
| crelt | 1.0.7 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/crelt@1.0.7.js` |
| w3c-keyname | 2.2.8 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/w3c-keyname@2.2.8.js` |
| @marijn/find-cluster-break | 1.0.4 | MIT | Copyright (C) Marijn Haverbeke and others | `internal/web/assets/vendor/codemirror/marijn-find-cluster-break@1.0.4.js` |

- QR Code は株式会社デンソーウェーブの登録商標です。
- 同梱するライセンスファイル: `internal/web/assets/vendor/LICENSE-qrcode-generator.txt`、`internal/web/assets/vendor/lit-html-3.3.3/LICENSE`、`internal/web/assets/vendor/codemirror/LICENSES/*.txt`
- 同梱したファイルの版・取得元・sha256 は `internal/web/assets/vendor/VENDOR.txt`（npm の登録所の tarball を `dist.integrity` と照合して、手を加えずに置いた。2026-11-26）

### qrcode-generator の MIT License

MIT License

Copyright (c) 2009 Kazuhiko Arase

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

この文書は法的な助言ではありません。配布の前に、第4段階で依存のライセンスを自動で集めた結果と照らし合わせて見直します。

<!-- 草案: 版は go.mod / go.sum と一致させること。依存を足したらこの表も更新する -->
