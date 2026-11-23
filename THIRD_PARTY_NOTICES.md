# 第三者ソフトウェアのライセンス

irl-live-keeper（MIT License）は、次のソフトウェアを利用しています。配布用の zip には、各ライセンスの全文を同梱します（第4段階で、ビルド時に依存関係から自動で集める予定）。

## 同梱するプログラム: MediaMTX

| ソフトウェア | 版 | ライセンス | 著作権者 | 入手先 |
|---|---|---|---|---|
| MediaMTX | v1.21.4 | MIT | aler9 | https://github.com/bluenviron/mediamtx |

- **同梱の形:** 公式リリースの配布物（`mediamtx(.exe)` と `LICENSE`）を改変せずに同梱する。irl-live-keeper とは別のプログラムで、irl-live-keeper は子プロセスとして起動し、API で通信するだけ。MediaMTX のソースコードは irl-live-keeper に組み込んでいない
- **ライセンスの兼ね合い:** irl-live-keeper も MediaMTX も MIT なので衝突しない。また、別のプログラムとして同梱するだけなので、MediaMTX のライセンスの条件は irl-live-keeper のコードには及ばない
- **守ること:** MediaMTX の `LICENSE`（著作権表示と許諾文）を、配布する zip に必ず入れる。ファイル名は `licenses/MediaMTX-LICENSE.txt` とする予定
- **MediaMTX のバイナリに組み込まれた依存:** MediaMTX の実行ファイルには、MediaMTX 自身が依存するライブラリ（Apache-2.0、BSD、MIT など）が組み込まれている。MediaMTX の公式リリースは `LICENSE` だけを付けて配布しており、irl-live-keeper も公式の配布物を改変せずに同梱する。第4段階で、MediaMTX v1.21.4 の go.mod から依存のライセンスを自動で集め、`licenses/mediamtx-deps/` に添える予定（GPL 系の依存がないことも、そのときに確かめる）
- **名前:** 「MediaMTX」の名前は、同梱しているプログラムを示すためだけに使う。MediaMTX の作者が irl-live-keeper を推奨・保証しているかのような表現はしない
- **コーデックについて:** MediaMTX は受け取った映像を中継するだけで、映像の符号化や復号はしない（irl-live-keeper も同じ）

## 実行ファイルに組み込まれる Go のライブラリ

| モジュール | 版 | ライセンス |
|---|---|---|
| github.com/andreykaipov/goobs | v1.11.1 | Apache-2.0 |
| gopkg.in/yaml.v3 | v3.0.1 | MIT と Apache-2.0 |
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

- QR Code は株式会社デンソーウェーブの登録商標です。
- 同梱するライセンスファイル: `internal/web/assets/vendor/LICENSE-qrcode-generator.txt`

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
