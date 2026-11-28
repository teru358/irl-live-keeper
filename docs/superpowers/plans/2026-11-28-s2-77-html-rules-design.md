# S2-77 HTML 本文検査の設計案 v3.1

改訂 v3.1: レビュー回帰入力 H298 以降を共通入力表に追加。HTML の Number は NaN・±Inf を未定義と同じ空文字で描画する。

改訂 v3: Fable の閉鎖確認 N1〜N8 と指揮者の決めを反映。文字参照・差し込み・木の再検査・共通入力表を補強。

改訂 v2: sol・Fable の全文レビューと指揮者の決めを反映。
URL の字句・CSS の改行と差し込み・文字参照の規則を確定。
入れ子を JSON に集約し、DOMParser 前の文字列検査を追加。
共通入力表を拡充し、rendered の期待と S2-78 のブラウザ試験を分離。

2026-11-28。対象: task S2-77、spec v0.31 第6・9・14章。Fable・codex sol のレビューを反映した版。実装は行わない。以下の JSON は将来のファイルの内容案であり、この変更では文書にだけ置く。

## 1. 許すものの表

表にない要素・属性は拒否。要素名・属性名・CSS の関数名と at-rule 名は ASCII 小文字にして比較する。属性値・素材パスは小文字化しない。`data-*`・`aria-*` の正規表現はこの設計の提案（接尾辞は1文字以上）。`style` 属性の差し込みは別規則。attributeFreeElements は globalAttributes より優先し、style 要素の属性はすべて template.html_attribute。許可表の裏返しの禁止表は持たない。

`internal/sources/html_rules.json` 案:

```json
{
  "version": 3,
  "elements": [
    "div",
    "span",
    "p",
    "br",
    "hr",
    "img",
    "style",
    "b",
    "i",
    "u",
    "s",
    "strong",
    "em",
    "small",
    "sub",
    "sup",
    "ul",
    "ol",
    "li",
    "table",
    "thead",
    "tbody",
    "tr",
    "th",
    "td",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "section",
    "header",
    "footer"
  ],
  "globalAttributes": [
    "class",
    "id",
    "title",
    "alt",
    "width",
    "height",
    "style",
    "lang",
    "dir"
  ],
  "attributeNamePatterns": [
    "^data-[a-z0-9_.:-]+$",
    "^aria-[a-z0-9_.:-]+$"
  ],
  "elementAttributes": {
    "img": [
      "src"
    ]
  },
  "voidElements": [
    "br",
    "hr",
    "img"
  ],
  "urlAttributes": {
    "img": [
      "src"
    ]
  },
  "templateAttributes": [
    "class",
    "title",
    "alt"
  ],
  "templateAttributePatterns": [
    "^data-[a-z0-9_.:-]+$",
    "^aria-[a-z0-9_.:-]+$"
  ],
  "styleAttribute": {
    "substitutionTypes": [
      "number"
    ],
    "controlTags": false
  },
  "css": {
    "rejectBackslash": true,
    "rejectAtRules": [
      "import"
    ],
    "urlFunction": "url",
    "rejectFunctions": [
      "image-set",
      "image",
      "cross-fade",
      "element",
      "src"
    ],
    "urlKinds": [
      "asset",
      "data:image/",
      "data:font/",
      "fragment"
    ],
    "stripVendorPrefixes": [
      "-webkit-",
      "-moz-",
      "-ms-",
      "-o-"
    ],
    "fragmentPattern": "^#[A-Za-z_][A-Za-z0-9_-]*$"
  },
  "asset": {
    "prefix": "/source/files/",
    "segmentPattern": "^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$",
    "maxSegments": 4,
    "maxPathLength": 512,
    "extensions": [
      "png",
      "jpg",
      "jpeg",
      "gif",
      "webp",
      "avif",
      "bmp",
      "svg"
    ],
    "savedQuery": false,
    "renderedQueryPattern": "^v=[0-9a-f]{12}$"
  },
  "attributeFreeElements": [
    "style"
  ],
  "nesting": {
    "parents": {
      "thead": [
        "table"
      ],
      "tbody": [
        "table"
      ],
      "tr": [
        "table",
        "thead",
        "tbody"
      ],
      "td": [
        "tr"
      ],
      "th": [
        "tr"
      ]
    },
    "children": {
      "table": [
        "thead",
        "tbody",
        "tr"
      ],
      "thead": [
        "tr"
      ],
      "tbody": [
        "tr"
      ],
      "tr": [
        "th",
        "td"
      ]
    },
    "restrictedTextParents": [
      "table",
      "thead",
      "tbody",
      "tr"
    ],
    "allowWhitespaceCommentsControlTags": true,
    "rejectSubstitutions": true,
    "pForbiddenDescendants": [
      "div",
      "p",
      "section",
      "header",
      "footer",
      "h1",
      "h2",
      "h3",
      "h4",
      "h5",
      "h6",
      "ul",
      "ol",
      "table",
      "hr",
      "li"
    ],
    "liAncestorBoundary": [
      "ul",
      "ol"
    ],
    "liRejectAncestorBeforeBoundary": "li",
    "headingElements": [
      "h1",
      "h2",
      "h3",
      "h4",
      "h5",
      "h6"
    ],
    "rejectHeadingDescendants": true,
    "implicitTbody": true,
    "maxDepth": 64
  }
}
```

JSON の正規表現は全体一致。URL 属性は `img.src` のみ。`style` は CSS の URL を別に走査する。正規表現だけで素材を検証せず第4節の条件もすべて適用する。

## 2. 字句と位置

Go と JS とも、ブラウザの HTML 自動修復に依存しない同じ走査器を保存時に使う。正規表現一発の HTML 分割・テンプレートを先に全文展開する方式は採らない。ブラウザの木は組み立て後の防御に使う。

1. 元入力と位置を保持して左から読む。NUL は文字・コメント・CSS・差し込み内を含め常に拒否。先に HTML の状態（本文、開始要素、終了要素、属性名、二重引用符値、コメント、style 生テキスト）を決める。
2. 本文と属性値では `{…}` を一字句として先に切り出し、既存テンプレートの字句・文法・型を適用する。タグ内部の文字列の `\"` と `\\`、文字列中の波括弧を扱い、文字列の外の最初の `}` で終える。条件式の `"` や `<` は HTML を閉じない。`{{`・`}}` は文字そのもの。禁止位置の `{…}` は構文を解釈する前に位置の誤りにする。エスケープの波括弧も禁止位置には置けない。
3. `<!--` はコメントへ入り、`-->` で終える。中ではテンプレートも要素も読まない。`<!-->`、`<!--->`、`--!>` は拒否。閉じない場合は開始位置を示す。`<!--` 以外の `<!`（DOCTYPE・CDATA 含む）、`<?` は拒否。コメント外の `--!>` は単なる文字。
4. `<` の直後（終了なら `</` の直後）に ASCII 英字で始まる要素名を読む。名前は ASCII 英数字と `-`。それ以外の `<` も字句誤り。要素名・属性名に `{` があれば位置の誤り。要素と属性の間、属性どうしの間は HTML 空白（SP、TAB、LF、CR、FF）が必要。
5. 属性名は ASCII 英字・数字・`_`・`-`・`.`・`:` の列。`=` の前後の空白は許す。値は `"…"` のみで、省略・無引用・単引用は拒否。直前の属性の引用符から次の属性名までにも空白が要る。小文字化後の同名属性は重複。閉じは `>` または `/>`。`/` は直後の `>` と一組のときだけ許し、属性区切りや `/ >` は拒否。終了要素には属性・自己終了を許さず空白と `>` のみ。
6. 開始 `style` 要素が確定したら生テキストへ。ASCII 大小を無視した `</style` の直後が HTML 空白・`/`・`>` のときだけ生テキストを終了する。その後も終了要素の文法で読む（`</style/>` は終了要素の字句誤り）。`</stylesheet>` は CSS の文字のまま。`</STYLE >` は正しい閉じ。引用符・CSS コメント内であってもこの終了判定を行う。EOF なら開始位置で誤り。
7. style と src の固定値は、セミコロン必須の数値参照（10進・16進）と `&amp;` `&lt;` `&gt;` `&quot;` `&apos;` `&nbsp;` だけを1回戻す。これらの許可参照の先頭でない `&` は、裸の `&` やセミコロンのない数値参照も含めすべて template.html_lex（`&` の位置）。数値参照の桁あふれ、Unicode 範囲外、0x80〜0x9F、サロゲート、NUL は template.html_lex（`&` の位置）。戻した文字を HTML/テンプレートとして再字句化しない。style 生テキストでは戻さない。ほかの属性と本文では数値参照ゼロも template.html_nul とし、それ以外の文字参照は出力で原文を保持し、table・thead・tbody・tr の直下の空白判定では、TAB・LF・FF・CR・SP になるセミコロンつき数値参照だけを空白とする。それ以外の `&` は名前つき参照を含め非空白文字として template.html_nesting（`&` の位置）。
8. 復号後の CSS/URL を検査する。復号文字から元入力への対応を保持し、参照から生じた違反は `&` の位置へ戻す。ただし CSS 文字列内の改行は開始引用符を指す。組み立ては固定文字の意味を保ち、差し込む値を必ず HTML エスケープする。値の NUL は除去する。本文・属性とも Go の `html.EscapeString` と同じく `& < > " '` をそれぞれ `&amp;` `&lt;` `&gt;` `&#34;` `&#39;` に逃がす。

位置は元入力の Unicode コードポイント、行・文字目とも1始まり。LF で行を進め文字目を1へ戻し、CR は通常の1文字（CRLF は LF で改行）。Go の byte offset と JS の UTF-16 offset をそのまま返さない。EOF は末尾の次。制御タグだけの行の除去や復号で元位置を失わない。

誤りは左から走査して最初に確定したもの1件。style/src の固定値では文字参照を先にすべて戻し、その段階の誤りは元位置で左から最初を返す。復号がすべて成功した後に CSS/URL を検査し、元の文字の位置で左から最初の誤りを返す（H289 は URL より後ろの &foo; の html_lex が先）。開始・終了要素の字句が確定してから入れ子を見る（H112）。禁止要素/属性は名前が確定した時点で報告。位置は要素なら `<`、属性なら属性名先頭、属性値の引用符誤りなら値先頭、CSS の違反なら関数名・at-rule の `@`・逆斜線、URL なら値の先頭（引用符つき CSS URL は開始引用符）。閉じ不足は開始 `<`、枝の不整合は境界の `{`。枝内で入口以前の要素を閉じる終了要素はその `<` で template.html_branch。属性をまたぐ if は開始の `{` で template.html_branch。属性内で完結する制御タグの構文誤りは既存 template.syntax。閉じない開始要素と文字の `<` は template.html_lex。

| code（新設） | 日本語の理由案 | English reason | 位置 |
|---|---|---|---|
| template.html_lex | HTML の書き方が正しくありません | Invalid HTML syntax | 違反の先頭 |
| template.html_comment | コメントを正しく閉じてください | Invalid or unclosed comment | 開始 `<`（不正終端はその先頭） |
| template.html_style_unclosed | style 要素を閉じてください | Unclosed style element | 開始 `<` |
| template.html_nul | NUL は使えません | NUL is not allowed | NUL / `&` |
| template.html_element | この要素は使えません | Element is not allowed | 開始/終了 `<` |
| template.html_attribute | この属性は使えません | Attribute is not allowed | 属性名 |
| template.html_duplicate | 同じ属性が重なっています | Duplicate attribute | 2個目の属性名 |
| template.html_quote | 属性値を二重引用符で囲んでください | Use double-quoted attribute values | 値先頭 |
| template.html_nesting | 要素の入れ子または閉じが正しくありません | Invalid element nesting or closing | 閉じ/開始 `<` |
| template.html_branch | 枝の中で要素を閉じてください | Close elements within each branch | 枝境界 `{` / 入口以前を閉じる `<` |
| template.html_template_position | この位置に差し込みや条件は書けません | Template tags are not allowed here | `{` |
| template.html_style_type | style には数値だけ差し込めます | Only number substitutions are allowed in style | `{` |
| template.html_url | この URL は使えません | URL is not allowed | URL 先頭 |
| template.css_escape | CSS の逆斜線は使えません | CSS backslashes are not allowed | `\` |
| template.css_import | CSS の import は使えません | CSS imports are not allowed | `@` |
| template.css_function | この CSS の URL 関数は使えません | CSS URL function is not allowed | 関数名 |
| template.css_syntax | CSS の文字列・URL・コメントの書き方が正しくありません | Invalid CSS string, URL or comment | 開始位置 |

既存の `template.syntax`・`unknown`・`format`・`comparison` は維持。文頭には既存翻訳と同様 `{line}` 行 `{column}` 文字目 / line {line}, column {column} を加える。API は code、翻訳済み error、line、column。保存 POST/PUT は422、preview は200の template_error（HTML は返さない）。

## 3. 文法・型

| 項目 | 規則 |
|---|---|
| 要素 | スタックで一致する閉じを要求。要素の深さはルート直下を1として64まで（void も1段と数える）。65段目の開始 `<` で template.html_nesting。br/hr/img は積まず、`/>` も許す。ほかの自己終了、void の終了要素、省略閉じは拒否 |
| if | 入口の要素スタックを枝ごとに複製。elif/else/end で入口と同一でなければ html_branch。入口以前に開いた要素も枝で閉じられない。else がない空枝も検査。全枝を検査し実行時の真偽で省かない |
| 属性の if | 許可された1つの引用符値の内部で完結。属性や要素をまたぐ制御を拒否。枝ごとに HTML 状態も入口と一致させる |
| 差し込み | 本文と JSON の許可属性のみ。id/width/height/lang/dir/src、名前、無引用値には禁止 |
| style 属性 | Number の差し込みのみ。Seconds/Clock/String/Bool と if/elif/else/end を拒否。形は既存 Number の0〜6。未定義は空。url 内・CSS 文字列内・CSS コメント内は html_template_position。`{` の直前の識別子文字 `[A-Za-z0-9_-]`（`-` を含む）と @ と # はすべて拒否。`}` の直後は ASCII 英字だけの並びか `%` で、その次が識別子文字でも `(` でもないときだけ単位として許す。それ以外の識別子文字との隣接は拒否。違反位置は `{`。{{・}} も html_template_position。続いた差し込みは1つと見て、取り除くと前後が `/` と `*` または `*` と `/` になる位置も html_template_position。片側だけの `/`（`calc(100%/{cam1.kbps})`）は許す。保存時は空と数値の2通りで CSS 検査を通し、描画後も再検査。負の値は `calc(-1 * {x}px)` と書く |
| style 要素と HTML コメント | 中の `{}` は CSS/コメントの文字。既知の値らしい記述は style の注意にするだけで参照に数えない |

入れ子の唯一の表は第1節 JSON の nesting。parents と children は両方を適用する。p の禁止は全子孫、li は ul/ol に達する前の li 先祖、見出しは全子孫の見出しを拒否する。要素の入れ子の深さは最上位を1として64までとし、65個目の開始 `<` で template.html_nesting。void も要素として深さを数える。restrictedTextParents の直下は許可された子要素・HTML 空白・コメント・制御タグのみ。文字参照の空白は第2節7の限定規則を使う。空白は直書きの TAB・LF・FF・CR・SP と、それらになるセミコロンつき数値参照のみ。ほかの `&`（名前つき参照も含む）は非空白文字。非空白文字と差し込みは枝の中でも html_nesting（文字/参照の先頭または `{`）。table 直下の style も拒否。直接 tr の tbody 補完だけは許し、走査器の比較用の木にも反映する。補完 tbody は最初の直下の tr で始まり、次の thead・tbody の開始か table の終わりまでとし、間の空白とコメントを含む。これ以上の木の一致の保証は S2-78 の試験で確認する。

## 4. CSS と URL

1. style 属性は復号後、style 要素は生テキストで検査。CSS 入力は CRLF を1つの LF、単独 CR と FF を LF に前処理し、元位置の対応を保つ。文字列内の LF・CR・FF（文字参照由来も含む）は開始引用符で template.css_syntax。全域の逆斜線はコメント・文字列内も css_escape。閉じない文字列・コメント・url は css_syntax。
2. コメントはトークン間の区切りで、前後の識別子を連結しない。関数名は直前が ASCII 識別子文字 `[A-Za-z0-9_-]` でない位置から始まる最長の識別子。ASCII 以外は区切り。直後が `(` のとき関数とし、大文字小文字を無視し、JSON の vendor 接頭辞を外して禁止関数と比較する。@import は拒否。通常の文字列・コメント内は関数/at-rule としない。src: 記述子と src( 関数は区別する。
3. `url(` の直後は空白だけ読み飛ばす。次が引用符なら「文字列 → 空白かコメント → `)`」。文字列前のコメントは拒否。引用符つき URL の文字列の後に空白・コメント・`)` 以外があれば template.css_syntax（開始引用符）。引用符つき URL の URL 違反も開始引用符を指す。引用符でなければ `)` までを生 URL とし、前後の空白だけ除く。内部の空白・引用符・`(`・`/*` は値の文字としてそのまま全体一致の URL 検査にかけ、失敗は template.html_url。空の値も html_url。閉じの `)` 自体がなければ css_syntax。url (...) は関数ではない。
4. URL は構文を除いた値で照合し、src は trim しない。URL 解決・percent decode・パス正規化はしない。素材は JSON の asset と CheckPath の予約名・末尾ドット規則を適用し、さらに //・..・?・#・% を拒否。拡張子のみ大小無視。存在確認は合否から分ける。
5. img.src は素材または data:image/ のみ。CSS url は素材、data:image/、data:font/、fragment。data URL の条件は「data:image/ または data:font/（ASCII 大小無視）で始まり、`,` を含む」。空データも許す。データ部には //・.. の拒否を適用せず、クエリ/fragment を外部 URL と解釈しない。fragment は JSON の fragmentPattern に全体一致。img.src の fragment と data:font/ は不可。別 MIME・相対/外部 URL・javascript: は拒否。
6. CSS の全文パーサーや関数の許可一覧は作らない。calc/rgb/var/local/format 等は許す。URL を持つ関数は既知の一覧の更新で追う。未知の関数を網羅して拒否する保証はせず、最後の守りは CSP と管理ページのプレビューの iframe の CSP。

CSP（第6章）は `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:; base-uri 'none'; form-action 'none'`。インライン script は許さず、style の許可は本文と OBS カスタム CSS のため。CSP を本文検査の代用にしない。

## 5. 組み立て後の再検査

保存検査 → 全枝の AST と参照集合 → 値をエスケープして組み立て → S2-85 の素材ハッシュ付与 → SSE/preview → 共通 JS 再検査 → 写し込み/差分更新、の順。

管理ページと source.js は、まず組み立てた文字列のまま、保存検査と同じ字句・URL・CSS の検査を JS の rendered モードで行う。差は本体が付ける素材末尾の `?v=<12文字の小文字の16進>` を1つだけ許すこと。任意クエリ・重複 v・fragment は拒否し、保存モードで利用者の v を断る。テンプレートは再解釈しない。

通った文字列だけを `DOMParser.parseFromString('<!doctype html><html><head></head><body>' + html, 'text/html')` に渡す。その後、木を検査する。要素は localName と namespaceURI（`http://www.w3.org/1999/xhtml`）で照合、属性は名前空間なしだけ。head は空、body は属性なしを要求し、body.childNodes 以下に許可表を適用する。先頭/途中の style は body 内。補完 tbody は許す。属性値は復号済みで二重復号せず、style.textContent は生テキスト。DOM の style 属性の値・style 要素の textContent・img の src に URL と CSS の検査をもう一度かける。元の引用符や重複属性の保証は前段の文字列検査が担う。rendered の誤りは code だけ比較し、元の行・文字目を要求しない。

木をすべて検査してから iframe 文書へ importNode する。管理ページ自身には innerHTML で入れない。iframe は sandbox="allow-same-origin"、外枠は認証つき /api/sources/frame、OBS と同じ CSP。source.js も同じ関数で全木の検査成功後に id/位置/要素名の差分を適用する。不合格なら受信した節を写さず誤りを表示/記録し、危険な節を部分挿入しない。初回外枠は空。

## 6. 素材の使用判定（S2-85）

保存検査で得た全枝 AST の img.src と style 属性・style 要素の url トークンから、復号後の `/source/files/…` を集め、完全一致の集合にする。実行しない枝も含め、data/fragment/コメント/普通の文字/通常属性に書いた URL 文字列は数えない。部分一致しない。組み立て後から逆算せずクエリなしの保存パスを持つ。欠損素材も集合に残し、保存は許して注意だけ返す。索引のパスは大小を含め文字どおり比較し、削除の409にソース表示名を使う。hash は SHA-256 の先頭12桁、同名更新でも URL の変更で HTML を送る。参照の書換えはトークン位置で行い文字列全置換はしない。

## 7. Go/Node 共通入力表

`internal/sources/html_cases.json` 案。defs は共通、mode は save / rendered。入力の重複は mode・input・values の組で判定する（save/rendered の対と値違いは別試験）。save の error は code/line/column、rendered は code のみ。refs の省略は空集合。任意欄 values は差し込む値、html は期待する出力、warnings は注意の code の文字列の配列（省略は比較しない、空配列は注意なし）。code は `template.style_literal`（style 要素内の既知の値らしい `{…}`）、`template.asset_missing`（ない素材）とする。ない素材の試験は refs のパスが索引にない fixture を使う。html のある行は検査後に組み立てて html と比較する。

試験は3層: (1) Go と Node が字句・URL・CSS の関数を直接呼ぶ（rendered モードも含む）。(2) は共通入力表に置かず JS の試験に手で書く。Node は小さな偽の節で木の検査を呼び、名前空間、head/body、属性、DOM 値の URL/CSS 再検査等を確認。(3) 本物の DOMParser は S2-78 の chromium-devtools で試す。Node 標準に DOMParser はない。ok の行を DOMParser に通し、tbody 補完を反映した走査器の木と同じことを確認する。比較は文字列でなく木で行う（innerHTML は `--!>` や本文の `"` の出し方が異なる）。本文の CR と CRLF は DOM では LF になる。値エスケープ行は追加 img が0件、title/本文が元値と一致、外向き要求0件を確認。外部 URL の rendered 行は DOMParser 呼出0件・挿入0件・外向き要求0件、更新失敗時は前回表示を保持することも確認する。

```json
{
  "version": 3,
  "defs": {"cam1.kbps": {"type": "number", "format": "0"}, "cam1.name": {"type": "string"}, "cam1.state": {"type": "string"}, "cam1.online": {"type": "bool"}, "stream.elapsed": {"type": "seconds"}, "obs.streaming": {"type": "bool"}, "now": {"type": "clock"}},
  "cases": [
    {"id": "H001", "mode": "save", "input": "<div></div>", "expect": "ok", "reason": "許可要素 div"},
    {"id": "H002", "mode": "save", "input": "<span></span>", "expect": "ok", "reason": "許可要素 span"},
    {"id": "H003", "mode": "save", "input": "<p></p>", "expect": "ok", "reason": "許可要素 p"},
    {"id": "H004", "mode": "save", "input": "<br>", "expect": "ok", "reason": "許可要素 br"},
    {"id": "H005", "mode": "save", "input": "<hr>", "expect": "ok", "reason": "許可要素 hr"},
    {"id": "H006", "mode": "save", "input": "<img>", "expect": "ok", "reason": "許可要素 img"},
    {"id": "H007", "mode": "save", "input": "<style></style>", "expect": "ok", "reason": "許可要素 style"},
    {"id": "H008", "mode": "save", "input": "<b></b>", "expect": "ok", "reason": "許可要素 b"},
    {"id": "H009", "mode": "save", "input": "<i></i>", "expect": "ok", "reason": "許可要素 i"},
    {"id": "H010", "mode": "save", "input": "<u></u>", "expect": "ok", "reason": "許可要素 u"},
    {"id": "H011", "mode": "save", "input": "<s></s>", "expect": "ok", "reason": "許可要素 s"},
    {"id": "H012", "mode": "save", "input": "<strong></strong>", "expect": "ok", "reason": "許可要素 strong"},
    {"id": "H013", "mode": "save", "input": "<em></em>", "expect": "ok", "reason": "許可要素 em"},
    {"id": "H014", "mode": "save", "input": "<small></small>", "expect": "ok", "reason": "許可要素 small"},
    {"id": "H015", "mode": "save", "input": "<sub></sub>", "expect": "ok", "reason": "許可要素 sub"},
    {"id": "H016", "mode": "save", "input": "<sup></sup>", "expect": "ok", "reason": "許可要素 sup"},
    {"id": "H017", "mode": "save", "input": "<ul></ul>", "expect": "ok", "reason": "許可要素 ul"},
    {"id": "H018", "mode": "save", "input": "<ol></ol>", "expect": "ok", "reason": "許可要素 ol"},
    {"id": "H019", "mode": "save", "input": "<li></li>", "expect": "ok", "reason": "許可要素 li"},
    {"id": "H020", "mode": "save", "input": "<table></table>", "expect": "ok", "reason": "許可要素 table"},
    {"id": "H021", "mode": "save", "input": "<table><thead><tr><td>x</td></tr></thead></table>", "expect": "ok", "reason": "許可要素 thead"},
    {"id": "H022", "mode": "save", "input": "<table><tbody><tr><td>x</td></tr></tbody></table>", "expect": "ok", "reason": "許可要素 tbody"},
    {"id": "H023", "mode": "save", "input": "<table><tr><td>x</td></tr></table>", "expect": "ok", "reason": "許可要素 tr"},
    {"id": "H024", "mode": "save", "input": "<table><tr><th>x</th></tr></table>", "expect": "ok", "reason": "許可要素 th"},
    {"id": "H025", "mode": "save", "input": "<table><thead><tr><td>x</td></tr></thead><tbody><tr><td>y</td></tr></tbody></table>", "expect": "ok", "reason": "許可要素 td"},
    {"id": "H026", "mode": "save", "input": "<h1></h1>", "expect": "ok", "reason": "許可要素 h1"},
    {"id": "H027", "mode": "save", "input": "<h2></h2>", "expect": "ok", "reason": "許可要素 h2"},
    {"id": "H028", "mode": "save", "input": "<h3></h3>", "expect": "ok", "reason": "許可要素 h3"},
    {"id": "H029", "mode": "save", "input": "<h4></h4>", "expect": "ok", "reason": "許可要素 h4"},
    {"id": "H030", "mode": "save", "input": "<h5></h5>", "expect": "ok", "reason": "許可要素 h5"},
    {"id": "H031", "mode": "save", "input": "<h6></h6>", "expect": "ok", "reason": "許可要素 h6"},
    {"id": "H032", "mode": "save", "input": "<section></section>", "expect": "ok", "reason": "許可要素 section"},
    {"id": "H033", "mode": "save", "input": "<header></header>", "expect": "ok", "reason": "許可要素 header"},
    {"id": "H034", "mode": "save", "input": "<footer></footer>", "expect": "ok", "reason": "許可要素 footer"},
    {"id": "H035", "mode": "save", "input": "<span>{cam1.kbps} kbps</span>", "expect": "ok", "reason": "ひな形"},
    {"id": "H036", "mode": "save", "input": "<style>.text { color: white; -webkit-text-stroke: 1px black; }</style><span class=\"text\">{cam1.kbps} kbps</span>", "expect": "ok", "reason": "ひな形"},
    {"id": "H037", "mode": "save", "input": "<style>.box { background: #000a; color: white; padding: 8px; }</style><div class=\"box\">{cam1.name}</div>", "expect": "ok", "reason": "ひな形"},
    {"id": "H038", "mode": "save", "input": "<style>.OK { color: lime; } .LOW { color: orange; } .DOWN { color: red; }</style><span class=\"{cam1.state}\">{cam1.state}</span>", "expect": "ok", "reason": "ひな形"},
    {"id": "H039", "mode": "save", "input": "ふつうの本文 日本語😀", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H040", "mode": "save", "input": "<div><p>本文<br>次</p></div>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H041", "mode": "save", "input": "<style>body {color:red} :root {--x:1}</style>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H042", "mode": "save", "input": "<span class=\"{if cam1.state == \"LOW\"}low{else}ok{end}\">x</span>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H043", "mode": "save", "input": "<span style=\"width:{cam1.kbps:0}px\">x</span>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H044", "mode": "save", "input": "<span style=\"fill:url(#grad)\"></span>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H045", "mode": "save", "input": "<img src=\"data:image/png;base64,AA==\">", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H046", "mode": "save", "input": "<!-- {bad} <img onerror=\"x\"> -->", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H047", "mode": "save", "input": "<style>.x{content:\"{cam1.name}\"}</style>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H048", "mode": "save", "input": "<STYLE>.x{color:red}</STYLE >", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H049", "mode": "save", "input": "<br/><hr /><img/>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H050", "mode": "save", "input": "{{a}}", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H051", "mode": "save", "input": "{if cam1.online}<b>x</b>{else}<i>y</i>{end}", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H052", "mode": "save", "input": "<div>{if cam1.online}<b>x</b>{end}</div>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H053", "mode": "save", "input": "<style>.x{width:calc(1px + 2px);color:rgb(1,2,3)}</style>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H054", "mode": "save", "input": "<span title=\"&#123;cam1.state&#125;\"></span>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H055", "mode": "save", "input": "<style>@font-face{src:url(data:font/woff2;base64,AA==)}</style>", "expect": "ok", "reason": "許可される文脈"},
    {"id": "H056", "mode": "save", "input": "<span class=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H057", "mode": "save", "input": "<span id=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H058", "mode": "save", "input": "<span title=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H059", "mode": "save", "input": "<span alt=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H060", "mode": "save", "input": "<span width=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H061", "mode": "save", "input": "<span height=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H062", "mode": "save", "input": "<span lang=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H063", "mode": "save", "input": "<span dir=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H064", "mode": "save", "input": "<span data-x=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H065", "mode": "save", "input": "<span aria-label=\"x\"></span>", "expect": "ok", "reason": "共通属性"},
    {"id": "H066", "mode": "save", "input": "<span title=\"{cam1.name}\"></span>", "expect": "ok", "reason": "文字列の差し込み"},
    {"id": "H067", "mode": "save", "input": "<span alt=\"{cam1.name}\"></span>", "expect": "ok", "reason": "文字列の差し込み"},
    {"id": "H068", "mode": "save", "input": "<span data-x=\"{cam1.name}\"></span>", "expect": "ok", "reason": "文字列の差し込み"},
    {"id": "H069", "mode": "save", "input": "<span aria-label=\"{cam1.name}\"></span>", "expect": "ok", "reason": "文字列の差し込み"},
    {"id": "H070", "mode": "save", "input": "<img src=\"/source/files/a.png\">", "expect": "ok", "reason": "単一素材の完全一致", "refs": ["/source/files/a.png"]},
    {"id": "H071", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a.png\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a.png"]},
    {"id": "H072", "mode": "save", "input": "<img src=\"/source/files/a/b/c/x.png\">", "expect": "ok", "reason": "素材パスの最大4段", "refs": ["/source/files/a/b/c/x.png"]},
    {"id": "H073", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a/b/c/x.png\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a/b/c/x.png"]},
    {"id": "H074", "mode": "save", "input": "<img src=\"/source/files/a.b/x.SVG\">", "expect": "ok", "reason": "名前のドットと拡張子の大小無視", "refs": ["/source/files/a.b/x.SVG"]},
    {"id": "H075", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a.b/x.SVG\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a.b/x.SVG"]},
    {"id": "H076", "mode": "save", "input": "<span style=\"background:url( /*a*/ #grad /*b*/ )\"></span>", "expect": "error", "reason": "url の中にコメントはない", "error": {"code": "template.html_url", "line": 1, "column": 30}},
    {"id": "H077", "mode": "save", "input": "<span style=\"background:URL('#grad')\"></span>", "expect": "ok", "reason": "文字列・コメント・区切り"},
    {"id": "H078", "mode": "save", "input": "<style>.x{content:\"@import url(https://evil)\"}</style>", "expect": "ok", "reason": "文字列・コメント・区切り"},
    {"id": "H079", "mode": "save", "input": "<!-- --!x -->", "expect": "ok", "reason": "文字列・コメント・区切り"},
    {"id": "H080", "mode": "save", "input": "--!>", "expect": "ok", "reason": "文字列・コメント・区切り"},
    {"id": "H081", "mode": "save", "input": "<svg></svg>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H082", "mode": "save", "input": "<math></math>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H083", "mode": "save", "input": "<script></script>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H084", "mode": "save", "input": "<noscript></noscript>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H085", "mode": "save", "input": "<template></template>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H086", "mode": "save", "input": "<frame></frame>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H087", "mode": "save", "input": "<frameset></frameset>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H088", "mode": "save", "input": "<iframe></iframe>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H089", "mode": "save", "input": "<video></video>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H090", "mode": "save", "input": "<audio></audio>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H091", "mode": "save", "input": "<a></a>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H092", "mode": "save", "input": "<link></link>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H093", "mode": "save", "input": "<base></base>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H094", "mode": "save", "input": "<object></object>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H095", "mode": "save", "input": "<embed></embed>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H096", "mode": "save", "input": "<input></input>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H097", "mode": "save", "input": "<form></form>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H098", "mode": "save", "input": "<textarea></textarea>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H099", "mode": "save", "input": "<xmp></xmp>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H100", "mode": "save", "input": "<plaintext></plaintext>", "expect": "error", "reason": "許可表外", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H101", "mode": "save", "input": "<svg><style><img onerror=\"x\">", "expect": "error", "reason": "外来要素の抜け道", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H102", "mode": "save", "input": "<!doctype html>", "expect": "error", "reason": "宣言を拒否", "error": {"code": "template.html_lex", "line": 1, "column": 1}},
    {"id": "H103", "mode": "save", "input": "<![CDATA[x]]>", "expect": "error", "reason": "宣言を拒否", "error": {"code": "template.html_lex", "line": 1, "column": 1}},
    {"id": "H104", "mode": "save", "input": "<?xml?>", "expect": "error", "reason": "宣言を拒否", "error": {"code": "template.html_lex", "line": 1, "column": 1}},
    {"id": "H105", "mode": "save", "input": "<!x>", "expect": "error", "reason": "宣言を拒否", "error": {"code": "template.html_lex", "line": 1, "column": 1}},
    {"id": "H106", "mode": "save", "input": "<!-->", "expect": "error", "reason": "不正コメント", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H107", "mode": "save", "input": "<!--->", "expect": "error", "reason": "不正コメント", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H108", "mode": "save", "input": "<!-- x", "expect": "error", "reason": "不正コメント", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H109", "mode": "save", "input": "<!-- --!> -->", "expect": "error", "reason": "不正コメント", "error": {"code": "template.html_comment", "line": 1, "column": 6}},
    {"id": "H110", "mode": "save", "input": "<img/src=\"/source/files/a.png\"/onerror=\"x\">", "expect": "error", "reason": "slash は属性区切りでない", "error": {"code": "template.html_lex", "line": 1, "column": 5}},
    {"id": "H111", "mode": "save", "input": "<div / >", "expect": "error", "reason": "slash は属性区切りでない", "error": {"code": "template.html_lex", "line": 1, "column": 6}},
    {"id": "H112", "mode": "save", "input": "</div/>", "expect": "error", "reason": "slash は属性区切りでない", "error": {"code": "template.html_lex", "line": 1, "column": 6}},
    {"id": "H113", "mode": "save", "input": "<img onerror=\"x\">", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_attribute", "line": 1, "column": 6}},
    {"id": "H114", "mode": "save", "input": "<a href=\"x\">", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_element", "line": 1, "column": 1}},
    {"id": "H115", "mode": "save", "input": "<img srcset=\"x\">", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_attribute", "line": 1, "column": 6}},
    {"id": "H116", "mode": "save", "input": "<span onclick=\"x\"></span>", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_attribute", "line": 1, "column": 7}},
    {"id": "H117", "mode": "save", "input": "<div data-=\"x\"></div>", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_attribute", "line": 1, "column": 6}},
    {"id": "H118", "mode": "save", "input": "<span aria-=\"x\"></span>", "expect": "error", "reason": "許可表外の属性/要素", "error": {"code": "template.html_attribute", "line": 1, "column": 7}},
    {"id": "H119", "mode": "save", "input": "<div id=\"a\" id=\"b\"></div>", "expect": "error", "reason": "大小を無視して重複", "error": {"code": "template.html_duplicate", "line": 1, "column": 13}},
    {"id": "H120", "mode": "save", "input": "<div CLASS=\"a\" class=\"b\"></div>", "expect": "error", "reason": "大小を無視して重複", "error": {"code": "template.html_duplicate", "line": 1, "column": 16}},
    {"id": "H121", "mode": "save", "input": "<img src='x'>", "expect": "error", "reason": "引用符/区切り", "error": {"code": "template.html_quote", "line": 1, "column": 10}},
    {"id": "H122", "mode": "save", "input": "<img src=x>", "expect": "error", "reason": "引用符/区切り", "error": {"code": "template.html_quote", "line": 1, "column": 10}},
    {"id": "H123", "mode": "save", "input": "<div title></div>", "expect": "error", "reason": "引用符/区切り", "error": {"code": "template.html_quote", "line": 1, "column": 11}},
    {"id": "H124", "mode": "save", "input": "<div title=\"x\"id=\"y\"></div>", "expect": "error", "reason": "引用符/区切り", "error": {"code": "template.html_lex", "line": 1, "column": 15}},
    {"id": "H125", "mode": "save", "input": "<{cam1.state}></x>", "expect": "error", "reason": "禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 2}},
    {"id": "H126", "mode": "save", "input": "<div {cam1.state}=\"x\"></div>", "expect": "error", "reason": "禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 6}},
    {"id": "H127", "mode": "save", "input": "<div title={cam1.state}></div>", "expect": "error", "reason": "禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 12}},
    {"id": "H128", "mode": "save", "input": "<img src=\"{cam1.name}\">", "expect": "error", "reason": "禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 11}},
    {"id": "H129", "mode": "save", "input": "<div id=\"{cam1.name}\"></div>", "expect": "error", "reason": "禁止属性の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 10}},
    {"id": "H130", "mode": "save", "input": "<div width=\"{cam1.name}\"></div>", "expect": "error", "reason": "禁止属性の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 13}},
    {"id": "H131", "mode": "save", "input": "<div height=\"{cam1.name}\"></div>", "expect": "error", "reason": "禁止属性の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 14}},
    {"id": "H132", "mode": "save", "input": "<div lang=\"{cam1.name}\"></div>", "expect": "error", "reason": "禁止属性の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 12}},
    {"id": "H133", "mode": "save", "input": "<div dir=\"{cam1.name}\"></div>", "expect": "error", "reason": "禁止属性の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 11}},
    {"id": "H134", "mode": "save", "input": "<span style=\"width:{cam1.name}\"></span>", "expect": "error", "reason": "style は数値だけ", "error": {"code": "template.html_style_type", "line": 1, "column": 20}},
    {"id": "H135", "mode": "save", "input": "<span style=\"width:{stream.elapsed:s}\"></span>", "expect": "error", "reason": "style は数値だけ", "error": {"code": "template.html_style_type", "line": 1, "column": 20}},
    {"id": "H136", "mode": "save", "input": "<span style=\"width:{obs.streaming}\"></span>", "expect": "error", "reason": "style は数値だけ", "error": {"code": "template.html_style_type", "line": 1, "column": 20}},
    {"id": "H137", "mode": "save", "input": "<span style=\"width:{now:hm}\"></span>", "expect": "error", "reason": "style は数値だけ", "error": {"code": "template.html_style_type", "line": 1, "column": 20}},
    {"id": "H138", "mode": "save", "input": "<span style=\"width:{if cam1.online}\"></span>", "expect": "error", "reason": "style は数値だけ", "error": {"code": "template.html_style_type", "line": 1, "column": 20}},
    {"id": "H139", "mode": "save", "input": "<div><span></div></span>", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 12}},
    {"id": "H140", "mode": "save", "input": "<p>x", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 1}},
    {"id": "H141", "mode": "save", "input": "<div/>", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 1}},
    {"id": "H142", "mode": "save", "input": "<br></br>", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 5}},
    {"id": "H143", "mode": "save", "input": "<p><div></div></p>", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 4}},
    {"id": "H144", "mode": "save", "input": "<table><td>x</td></table>", "expect": "error", "reason": "入れ子・閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H145", "mode": "save", "input": "{if cam1.online}<div>{else}</div>{end}", "expect": "error", "reason": "枝をまたぐ要素", "error": {"code": "template.html_branch", "line": 1, "column": 22}},
    {"id": "H146", "mode": "save", "input": "<div>{if cam1.online}</div>{end}", "expect": "error", "reason": "枝をまたぐ要素", "error": {"code": "template.html_branch", "line": 1, "column": 22}},
    {"id": "H147", "mode": "save", "input": "{if cam1.online}<b>{end}", "expect": "error", "reason": "枝をまたぐ要素", "error": {"code": "template.html_branch", "line": 1, "column": 20}},
    {"id": "H148", "mode": "save", "input": "<style>.x{}", "expect": "error", "reason": "style の閉じ不足", "error": {"code": "template.html_style_unclosed", "line": 1, "column": 1}},
    {"id": "H149", "mode": "save", "input": "<style>.x{}</stylesheet>", "expect": "error", "reason": "style の閉じ不足", "error": {"code": "template.html_style_unclosed", "line": 1, "column": 1}},
    {"id": "H150", "mode": "save", "input": "<style>.x{}</style/>", "expect": "error", "reason": "終了 slash 不正", "error": {"code": "template.html_lex", "line": 1, "column": 19}},
    {"id": "H151", "mode": "save", "input": "あ😀\n<span>\u0000</span>", "expect": "error", "reason": "NUL とコードポイント位置", "error": {"code": "template.html_nul", "line": 2, "column": 7}},
    {"id": "H152", "mode": "save", "input": "<div title=\"&#0;\"></div>", "expect": "error", "reason": "NUL とコードポイント位置", "error": {"code": "template.html_nul", "line": 1, "column": 13}},
    {"id": "H153", "mode": "save", "input": "<!--\u0000-->", "expect": "error", "reason": "NUL とコードポイント位置", "error": {"code": "template.html_nul", "line": 1, "column": 5}},
    {"id": "H154", "mode": "save", "input": "<img src=\"//evil\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H155", "mode": "save", "input": "<img src=\"https://evil/a.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H156", "mode": "save", "input": "<img src=\"javascript:alert(1)\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H157", "mode": "save", "input": "<img src=\"/source/files/../x\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H158", "mode": "save", "input": "<img src=\"/source/files/a.png?x\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H159", "mode": "save", "input": "<img src=\"/source/files/a.png?v=abcdef123456\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H160", "mode": "save", "input": "<img src=\"/source/files/a.png#x\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H161", "mode": "save", "input": "<img src=\"a.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H162", "mode": "save", "input": "<img src=\"/source/files/%61.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H163", "mode": "save", "input": "<img src=\"/source/files/a//x.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H164", "mode": "save", "input": "<img src=\"/source/files/a/b/c/d/x.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H165", "mode": "save", "input": "<img src=\"/source/files/CON.png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H166", "mode": "save", "input": "<img src=\"/source/files/a..png\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H167", "mode": "save", "input": "<img src=\"data:text/html,x\">", "expect": "error", "reason": "URL 制約", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H168", "mode": "save", "input": "<span style=\"background:u\\72l(//evil)\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.css_escape", "line": 1, "column": 26}},
    {"id": "H169", "mode": "save", "input": "<style>@\\69mport \"x\"</style>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.css_escape", "line": 1, "column": 9}},
    {"id": "H170", "mode": "save", "input": "<style>@import \"https://evil\"</style>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.css_import", "line": 1, "column": 8}},
    {"id": "H171", "mode": "save", "input": "<span style=\"background:&#117;rl(//evil)\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.html_url", "line": 1, "column": 34}},
    {"id": "H172", "mode": "save", "input": "<span style=\"background:url(//evil)\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.html_url", "line": 1, "column": 29}},
    {"id": "H173", "mode": "save", "input": "<span style=\"background:url(/source/files/a.png?x)\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.html_url", "line": 1, "column": 29}},
    {"id": "H174", "mode": "save", "input": "<span style=\"background:url(#)\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.html_url", "line": 1, "column": 29}},
    {"id": "H175", "mode": "save", "input": "<span style=\"background:url(\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.css_syntax", "line": 1, "column": 25}},
    {"id": "H176", "mode": "save", "input": "<span style=\"/* x\"></span>", "expect": "error", "reason": "CSS の抜け道", "error": {"code": "template.css_syntax", "line": 1, "column": 14}},
    {"id": "H177", "mode": "save", "input": "<style>.x{background:image-set(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H178", "mode": "save", "input": "<style>.x{background:-webkit-image-set(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H179", "mode": "save", "input": "<style>.x{background:image(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H180", "mode": "save", "input": "<style>.x{background:cross-fade(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H181", "mode": "save", "input": "<style>.x{background:element(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H182", "mode": "save", "input": "<style>.x{background:src(\"https://evil\")}</style>", "expect": "error", "reason": "url 以外の URL 関数", "error": {"code": "template.css_function", "line": 1, "column": 22}},
    {"id": "H183", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abcdef123456\">", "expect": "ok", "reason": "生成クエリの再検査"},
    {"id": "H184", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abc\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url"}},
    {"id": "H185", "mode": "rendered", "input": "<img src=\"/source/files/a.png?x=abcdef123456\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url"}},
    {"id": "H186", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abcdef123456#x\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url"}},
    {"id": "H187", "mode": "save", "input": "{if cam1.online}<img src=\"/source/files/a.png\">{else}<style>.x{background:url(/source/files/b.png)}</style>{end}", "expect": "ok", "reason": "全枝の集合", "refs": ["/source/files/a.png", "/source/files/b.png"]},
    {"id": "H188", "mode": "save", "input": "<img src=\"/source/files/a&#46;png\">", "expect": "ok", "reason": "復号して素材参照", "refs": ["/source/files/a.png"]},
    {"id": "H189", "mode": "save", "input": "<style>.x{background:url(/source/files/a&#46;png)}</style>", "expect": "error", "reason": "raw CSS は復号しない", "error": {"code": "template.html_url", "line": 1, "column": 26}},
    {"id": "H190", "mode": "save", "input": "<span style=\"background:url(/*/../../api/x?*//source/files/a.png)\"></span>", "expect": "error", "reason": "url の中にコメントはない", "error": {"code": "template.html_url", "line": 1, "column": 29}},
    {"id": "H191", "mode": "save", "input": "<style>.x{background:url(/source/files/a.png/**/)}</style>", "expect": "error", "reason": "url の中にコメントはない", "error": {"code": "template.html_url", "line": 1, "column": 26}},
    {"id": "H192", "mode": "save", "input": "<style>.x{background:url(/**/\"/source/files/a.png\")}</style>", "expect": "error", "reason": "url の中にコメントはない", "error": {"code": "template.html_url", "line": 1, "column": 26}},
    {"id": "H193", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a.png\" /*c*/)}</style>", "expect": "ok", "reason": "文字列の後ろのコメント", "refs": ["/source/files/a.png"]},
    {"id": "H194", "mode": "save", "input": "<style>.x{content:\"abc\n} .y{background:url(/api/status)} .z{content:\"}</style>", "expect": "error", "reason": "文字列の中の改行", "error": {"code": "template.css_syntax", "line": 1, "column": 19}},
    {"id": "H195", "mode": "save", "input": "<span style=\"content:'a&#10;;background:url(/api/status)\"></span>", "expect": "error", "reason": "文字列の中の改行（文字参照）", "error": {"code": "template.css_syntax", "line": 1, "column": 22}},
    {"id": "H196", "mode": "save", "input": "<style>.x{background:50%url(//evil)}</style>", "expect": "error", "reason": "関数名の切り出し", "error": {"code": "template.html_url", "line": 1, "column": 29}},
    {"id": "H197", "mode": "save", "input": "<span style=\"background:url&lpar;//evil)\"></span>", "expect": "error", "reason": "名前つき参照", "error": {"code": "template.html_lex", "line": 1, "column": 28}},
    {"id": "H198", "mode": "save", "input": "<span style=\"background:u&bsol;72l(//evil)\"></span>", "expect": "error", "reason": "名前つき参照", "error": {"code": "template.html_lex", "line": 1, "column": 26}},
    {"id": "H199", "mode": "save", "input": "<span style=\"background:u{cam1.kbps}rl(//evil)\"></span>", "expect": "error", "reason": "識別子に隣り合う差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 26}},
    {"id": "H200", "mode": "save", "input": "<span style=\"background:url(/source/files/a{cam1.kbps}.png)\"></span>", "expect": "error", "reason": "url の中の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 44}},
    {"id": "H201", "mode": "save", "input": "<p><li>x</li></p>", "expect": "error", "reason": "入れ子", "error": {"code": "template.html_nesting", "line": 1, "column": 4}},
    {"id": "H202", "mode": "save", "input": "<h1><h2>x</h2></h1>", "expect": "error", "reason": "入れ子", "error": {"code": "template.html_nesting", "line": 1, "column": 5}},
    {"id": "H203", "mode": "save", "input": "<p><span><div>x</div></span></p>", "expect": "error", "reason": "入れ子", "error": {"code": "template.html_nesting", "line": 1, "column": 10}},
    {"id": "H204", "mode": "save", "input": "<li><span><li>x</li></span></li>", "expect": "error", "reason": "入れ子", "error": {"code": "template.html_nesting", "line": 1, "column": 11}},
    {"id": "H205", "mode": "save", "input": "<li><ul><li>x</li></ul></li>", "expect": "ok", "reason": "入れ子"},
    {"id": "H206", "mode": "save", "input": "<td>x</td>", "expect": "error", "reason": "table の外", "error": {"code": "template.html_nesting", "line": 1, "column": 1}},
    {"id": "H207", "mode": "save", "input": "<div><tr><td>x</td></tr></div>", "expect": "error", "reason": "table の外", "error": {"code": "template.html_nesting", "line": 1, "column": 6}},
    {"id": "H208", "mode": "save", "input": "<table><tr><td><tr><td>y</td></tr></td></tr></table>", "expect": "error", "reason": "table の外", "error": {"code": "template.html_nesting", "line": 1, "column": 16}},
    {"id": "H209", "mode": "save", "input": "<table>x<tr><td>y</td></tr></table>", "expect": "error", "reason": "table の中の文字", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H210", "mode": "save", "input": "<table><tr>{cam1.name}<td>y</td></tr></table>", "expect": "error", "reason": "table の中の差し込み", "error": {"code": "template.html_nesting", "line": 1, "column": 12}},
    {"id": "H211", "mode": "save", "input": "<table>{if cam1.online}x{end}<tr><td>y</td></tr></table>", "expect": "error", "reason": "枝の中の文字", "error": {"code": "template.html_nesting", "line": 1, "column": 24}},
    {"id": "H212", "mode": "save", "input": "<table><style>.a{}</style><tr><td>y</td></tr></table>", "expect": "error", "reason": "table の直下", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H213", "mode": "save", "input": "<style/>.x{}</style>", "expect": "error", "reason": "style の自己終了", "error": {"code": "template.html_nesting", "line": 1, "column": 1}},
    {"id": "H214", "mode": "save", "input": "😀<svg></svg>", "expect": "error", "reason": "同じ行の絵文字", "error": {"code": "template.html_element", "line": 1, "column": 2}},
    {"id": "H215", "mode": "save", "input": "<div>\r\n<svg></svg></div>", "expect": "error", "reason": "CRLF", "error": {"code": "template.html_element", "line": 2, "column": 1}},
    {"id": "H216", "mode": "save", "input": "a\r<svg></svg>", "expect": "error", "reason": "単独の CR", "error": {"code": "template.html_element", "line": 1, "column": 3}},
    {"id": "H217", "mode": "save", "input": "1 < 2", "expect": "error", "reason": "文字の <", "error": {"code": "template.html_lex", "line": 1, "column": 3}},
    {"id": "H218", "mode": "save", "input": "<div title=\"x", "expect": "error", "reason": "閉じない開始要素", "error": {"code": "template.html_lex", "line": 1, "column": 1}},
    {"id": "H219", "mode": "save", "input": "</div>", "expect": "error", "reason": "対のない閉じ", "error": {"code": "template.html_nesting", "line": 1, "column": 1}},
    {"id": "H220", "mode": "save", "input": "<DIV></div>", "expect": "ok", "reason": "大文字小文字"},
    {"id": "H221", "mode": "save", "input": "<img src=\" /source/files/a.png\">", "expect": "error", "reason": "trim しない", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H222", "mode": "save", "input": "<img src=\"#x\">", "expect": "error", "reason": "img に fragment", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H223", "mode": "save", "input": "<img src=\"data:font/woff2;base64,AA==\">", "expect": "error", "reason": "img に font", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H224", "mode": "save", "input": "<style>@font-face{font-family:\"X\";src:local(\"Yu Gothic\")}</style>", "expect": "ok", "reason": "PC のフォント"},
    {"id": "H225", "mode": "save", "input": "<style>@keyframes blink{50%{opacity:0}} .x{animation:blink 1s infinite}</style>", "expect": "ok", "reason": "動き"},
    {"id": "H226", "mode": "save", "input": "<div style=\"width:calc({cam1.kbps} / 6000 * 100%)\"></div>", "expect": "ok", "reason": "ビットレートの棒"},
    {"id": "H227", "mode": "save", "input": "{if cam1.online}<img src=\"/source/files/on.png\">{else}<img src=\"/source/files/off.png\">{end}", "expect": "ok", "reason": "画像の出し分け", "refs": ["/source/files/on.png", "/source/files/off.png"]},
    {"id": "H228", "mode": "rendered", "input": "<style>.x{color:red}</style><span class=\"x\">a</span>", "expect": "ok", "reason": "先頭の style"},
    {"id": "H229", "mode": "rendered", "input": "<span>a</span><style>.x{background:url(/source/files/a.png?v=abcdef123456)}</style>", "expect": "ok", "reason": "途中の style と生成クエリ"},
    {"id": "H230", "mode": "rendered", "input": "<table><tr><td>x</td></tr></table>", "expect": "ok", "reason": "補われる tbody"},
    {"id": "H231", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=ABCDEF123456\">", "expect": "error", "reason": "大文字の16進", "error": {"code": "template.html_url"}},
    {"id": "H232", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abcdef123456&amp;v=abcdef123456\">", "expect": "error", "reason": "重なった v", "error": {"code": "template.html_url"}},
    {"id": "H233", "mode": "rendered", "input": "<img src=\"/source/files/a.png\" onerror=\"x\">", "expect": "error", "reason": "再検査の属性", "error": {"code": "template.html_attribute"}},
    {"id": "H234", "mode": "rendered", "input": "<svg></svg>", "expect": "error", "reason": "再検査の要素", "error": {"code": "template.html_element"}},
    {"id": "H235", "mode": "rendered", "input": "<style>@import \"https://evil\"</style>", "expect": "error", "reason": "再検査の CSS", "error": {"code": "template.css_import"}},
    {"id": "H236", "mode": "rendered", "input": "<span style=\"background:url(//evil)\"></span>", "expect": "error", "reason": "再検査の CSS", "error": {"code": "template.html_url"}},
    {"id": "H237", "mode": "save", "input": "<style>.x{background:url(/**/#x)}</style><div class=\"x\"></div>", "expect": "error", "reason": "無引用 URL のコメント風文字", "error": {"code": "template.html_url", "line": 1, "column": 26}},
    {"id": "H238", "mode": "rendered", "input": "<style>.x{background:url(/**/#x)}</style><div class=\"x\"></div>", "expect": "error", "reason": "無引用 URL のコメント風文字", "error": {"code": "template.html_url"}},
    {"id": "H239", "mode": "save", "input": "<span style=\"background:url(/source/files/a{cam1.kbps:0}.png)\"></span>", "expect": "error", "reason": "固定素材 URL 内の差し込み", "error": {"code": "template.html_template_position", "line": 1, "column": 44}},
    {"id": "H240", "mode": "save", "input": "<style>.x{content:\"\n;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の LF", "error": {"code": "template.css_syntax", "line": 1, "column": 19}},
    {"id": "H241", "mode": "rendered", "input": "<style>.x{content:\"\n;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の LF", "error": {"code": "template.css_syntax"}},
    {"id": "H242", "mode": "save", "input": "<style>.x{content:\"\r;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の CR", "error": {"code": "template.css_syntax", "line": 1, "column": 19}},
    {"id": "H243", "mode": "rendered", "input": "<style>.x{content:\"\r;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の CR", "error": {"code": "template.css_syntax"}},
    {"id": "H244", "mode": "save", "input": "<style>.x{content:\"\r\n;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の CRLF", "error": {"code": "template.css_syntax", "line": 1, "column": 19}},
    {"id": "H245", "mode": "rendered", "input": "<style>.x{content:\"\r\n;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の CRLF", "error": {"code": "template.css_syntax"}},
    {"id": "H246", "mode": "save", "input": "<style>.x{content:\"\f;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の FF", "error": {"code": "template.css_syntax", "line": 1, "column": 19}},
    {"id": "H247", "mode": "rendered", "input": "<style>.x{content:\"\f;background:url(https://evil.example/x);x:\"}</style>", "expect": "error", "reason": "CSS 文字列の FF", "error": {"code": "template.css_syntax"}},
    {"id": "H248", "mode": "save", "input": "<span style=\"content:'a&#xA;;background:url(/api/status)\"></span>", "expect": "error", "reason": "文字参照からの CSS 改行", "error": {"code": "template.css_syntax", "line": 1, "column": 22}},
    {"id": "H249", "mode": "save", "input": "<span style=\"content:'a&#13;;background:url(/api/status)\"></span>", "expect": "error", "reason": "文字参照からの CSS 改行", "error": {"code": "template.css_syntax", "line": 1, "column": 22}},
    {"id": "H250", "mode": "save", "input": "<span style=\"content:'a&#12;;background:url(/api/status)\"></span>", "expect": "error", "reason": "文字参照からの CSS 改行", "error": {"code": "template.css_syntax", "line": 1, "column": 22}},
    {"id": "H251", "mode": "save", "input": "<div><tbody><tr><td>x</td></tr></tbody></div>", "expect": "error", "reason": "table 系の親", "error": {"code": "template.html_nesting", "line": 1, "column": 6}},
    {"id": "H252", "mode": "save", "input": "<table>&#32;</table>", "expect": "ok", "reason": "復号後は空白、原文を保持", "html": "<table>&#32;</table>"},
    {"id": "H253", "mode": "save", "input": "<table>&#120;</table>", "expect": "error", "reason": "復号後の非空白文字", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H254", "mode": "save", "input": "<div style=\"width:calc({cam1.kbps:0}px / 10)\"></div>", "expect": "ok", "reason": "calc の棒"},
    {"id": "H255", "mode": "save", "input": "<style>@keyframes pulse{from{opacity:0}to{opacity:1}}.x{animation:pulse 1s infinite}</style><div class=\"x\"></div>", "expect": "ok", "reason": "アニメーション"},
    {"id": "H256", "mode": "save", "input": "<style>@font-face{font-family:Overlay;src:local(\"Arial\")}.x{font-family:Overlay}</style>", "expect": "ok", "reason": "local は関数、src は記述子"},
    {"id": "H257", "mode": "save", "input": "<style>:root{--fg:lime}.x{color:var(--fg)}</style><span class=\"x\">x</span>", "expect": "ok", "reason": ":root の変数"},
    {"id": "H258", "mode": "save", "input": "<img src=\"data:image/png;base64,//8=\">", "expect": "ok", "reason": "data の // はデータ"},
    {"id": "H259", "mode": "save", "input": "<img src=\"data:image/,\">", "expect": "ok", "reason": "空データも最小条件を満たす"},
    {"id": "H260", "mode": "save", "input": "<style>.x{background:url(#x)}</style>", "expect": "ok", "reason": "同文書 fragment"},
    {"id": "H261", "mode": "save", "input": "<div class=\"{if cam1.online}a\" title=\"{end}\"></div>", "expect": "error", "reason": "属性をまたぐ if", "error": {"code": "template.html_branch", "line": 1, "column": 13}},
    {"id": "H262", "mode": "save", "input": "<style title=\"x\">.x{}</style>", "expect": "error", "reason": "style 要素に属性は不可", "error": {"code": "template.html_attribute", "line": 1, "column": 8}},
    {"id": "H263", "mode": "save", "input": "<span style=\"content:'{cam1.kbps}'\"></span>", "expect": "error", "reason": "style の禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 23}},
    {"id": "H264", "mode": "save", "input": "<span style=\"color:#{cam1.kbps}\"></span>", "expect": "error", "reason": "style の禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 21}},
    {"id": "H265", "mode": "save", "input": "<span style=\"x:@{cam1.kbps}\"></span>", "expect": "error", "reason": "style の禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 17}},
    {"id": "H266", "mode": "save", "input": "<span style=\"width:{{\"></span>", "expect": "error", "reason": "style の禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 20}},
    {"id": "H267", "mode": "save", "input": "<span style=\"width:}}\"></span>", "expect": "error", "reason": "style の禁止位置", "error": {"code": "template.html_template_position", "line": 1, "column": 20}},
    {"id": "H268", "mode": "save", "input": "<img src=\"&#0;\">", "expect": "error", "reason": "src の不許可数値参照", "error": {"code": "template.html_lex", "line": 1, "column": 11}},
    {"id": "H269", "mode": "save", "input": "<img src=\"&#128;\">", "expect": "error", "reason": "src の不許可数値参照", "error": {"code": "template.html_lex", "line": 1, "column": 11}},
    {"id": "H270", "mode": "save", "input": "<img src=\"&#xD800;\">", "expect": "error", "reason": "src の不許可数値参照", "error": {"code": "template.html_lex", "line": 1, "column": 11}},
    {"id": "H271", "mode": "save", "input": "<img src=\"&#999999999999999999999;\">", "expect": "error", "reason": "src の不許可数値参照", "error": {"code": "template.html_lex", "line": 1, "column": 11}},
    {"id": "H272", "mode": "save", "input": "<span title=\"{cam1.name}\">{cam1.name}</span>", "expect": "ok", "reason": "属性と本文の値をエスケープ", "values": {"cam1.name": "\"><img src=\"https://evil.example/x\" onerror=\"alert(1)\">"}, "html": "<span title=\"&#34;&gt;&lt;img src=&#34;https://evil.example/x&#34; onerror=&#34;alert(1)&#34;&gt;\">&#34;&gt;&lt;img src=&#34;https://evil.example/x&#34; onerror=&#34;alert(1)&#34;&gt;</span>", "warnings": []},
    {"id": "H273", "mode": "save", "input": "<span title=\"{cam1.name}\">{cam1.name}</span>", "expect": "ok", "reason": "値の NUL を除去", "values": {"cam1.name": "a\u0000b"}, "html": "<span title=\"ab\">ab</span>", "warnings": []},
    {"id": "H274", "mode": "rendered", "input": "<img src=\"https://evil.example/probe\">", "expect": "error", "reason": "DOMParser 前に外部 URL を拒否", "error": {"code": "template.html_url"}},
    {"id": "H275", "mode": "save", "input": "<span style=\"background:&#117rl(//evil)\"></span>", "expect": "error", "reason": "セミコロンのない数値参照", "error": {"code": "template.html_lex", "line": 1, "column": 25}},
    {"id": "H276", "mode": "rendered", "input": "<span style=\"background:&#117rl(//evil)\"></span>", "expect": "error", "reason": "セミコロンのない数値参照", "error": {"code": "template.html_lex"}},
    {"id": "H277", "mode": "save", "input": "<span style=\"background:url&#x28//evil)\"></span>", "expect": "error", "reason": "セミコロンのない16進参照", "error": {"code": "template.html_lex", "line": 1, "column": 28}},
    {"id": "H278", "mode": "save", "input": "<span style=\"a:b&c\"></span>", "expect": "error", "reason": "裸の &", "error": {"code": "template.html_lex", "line": 1, "column": 17}},
    {"id": "H279", "mode": "save", "input": "<span style=\"/* *{cam1.kbps}/ x:/{cam1.kbps}* ;background:url(//evil.example/a); /* */\"></span>", "expect": "error", "reason": "同じ変数2回、CSS コメント内", "error": {"code": "template.html_template_position", "line": 1, "column": 18}},
    {"id": "H280", "mode": "save", "input": "<span style=\"width:calc(100%/{cam1.kbps})\"></span>", "expect": "ok", "reason": "片側の / は許可"},
    {"id": "H281", "mode": "save", "input": "<span style=\"background:{cam1.kbps}url(//evil)\"></span>", "expect": "error", "reason": "関数名は単位でない", "error": {"code": "template.html_template_position", "line": 1, "column": 25}},
    {"id": "H282", "mode": "save", "input": "<span style=\"width:{cam1.kbps}%\"></span>", "expect": "ok", "reason": "% 単位"},
    {"id": "H283", "mode": "save", "input": "<span style=\"margin:-{cam1.kbps}px\"></span>", "expect": "error", "reason": "直前の - は識別子文字", "error": {"code": "template.html_template_position", "line": 1, "column": 22}},
    {"id": "H284", "mode": "save", "input": "<span style=\"width:{cam1.kbps}e3\"></span>", "expect": "error", "reason": "英数字は単位でない", "error": {"code": "template.html_template_position", "line": 1, "column": 20}},
    {"id": "H285", "mode": "save", "input": "<table>&Tab;</table>", "expect": "error", "reason": "名前つき参照は table の空白でない", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H286", "mode": "save", "input": "<table>&nbsp;</table>", "expect": "error", "reason": "名前つき参照は table の空白でない", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H287", "mode": "save", "input": "<span title=\"{cam1.state}\">{cam1.state}</span>", "expect": "ok", "reason": "値の ' と & をエスケープ", "values": {"cam1.state": "'&"}, "html": "<span title=\"&#39;&amp;\">&#39;&amp;</span>", "warnings": []},
    {"id": "H288", "mode": "save", "input": "<style>.x{background:url(\"//evil\")}</style>", "expect": "error", "reason": "引用符つき URL は開始引用符", "error": {"code": "template.html_url", "line": 1, "column": 26}},
    {"id": "H289", "mode": "save", "input": "<span style=\"background:url(//evil);x:&foo;\"></span>", "expect": "error", "reason": "文字参照をすべて先に戻す", "error": {"code": "template.html_lex", "line": 1, "column": 39}},
    {"id": "H290", "mode": "save", "input": "<style>.x{background:url(\"#x\" x)}</style>", "expect": "error", "reason": "引用符の後の余分な文字", "error": {"code": "template.css_syntax", "line": 1, "column": 26}},
    {"id": "H291", "mode": "save", "input": "<style>.x{width:{cam1.kbps}}</style>", "expect": "ok", "reason": "style 要素の既知の値は注意", "warnings": ["template.style_literal"]},
    {"id": "H292", "mode": "save", "input": "<img src=\"/source/files/missing.png\">", "expect": "ok", "reason": "索引にない素材も保存可", "refs": ["/source/files/missing.png"], "warnings": ["template.asset_missing"]},
    {"id": "H293", "mode": "save", "input": "<div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div>", "expect": "error", "reason": "深さ65は拒否", "error": {"code": "template.html_nesting", "line": 1, "column": 321}},
    {"id": "H294", "mode": "save", "input": "<div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div><div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div></div>", "expect": "ok", "reason": "深さ64は許可"},
    {"id": "H295", "mode": "save", "input": "<span style=\"x:/{cam1.kbps}*\"></span>", "expect": "error", "reason": "除去でコメント開始", "error": {"code": "template.html_template_position", "line": 1, "column": 17}},
    {"id": "H296", "mode": "save", "input": "<span style=\"x:*{cam1.kbps}/\"></span>", "expect": "error", "reason": "除去でコメント終了", "error": {"code": "template.html_template_position", "line": 1, "column": 17}},
    {"id": "H297", "mode": "save", "input": "<span style=\"x:/{cam1.kbps}{cam1.kbps}*\"></span>", "expect": "error", "reason": "連続差し込みを一つと見る", "error": {"code": "template.html_template_position", "line": 1, "column": 17}},
    {"id": "H298", "mode": "save", "input": "<!--><b>x</b>-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H299", "mode": "rendered", "input": "<!--><b>x</b>-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H300", "mode": "save", "input": "<!---><b>x</b>-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H301", "mode": "rendered", "input": "<!---><b>x</b>-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H302", "mode": "save", "input": "<!--><img src=\"https://evil.example/x\" onerror=\"alert(1)\">-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H303", "mode": "rendered", "input": "<!--><img src=\"https://evil.example/x\" onerror=\"alert(1)\">-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H304", "mode": "save", "input": "<!---><img src=\"/api/status\">-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H305", "mode": "rendered", "input": "<!---><img src=\"/api/status\">-->", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_comment", "line": 1, "column": 1}},
    {"id": "H306", "mode": "save", "input": "<div style=\"{if(cam1.online)}color:red\">x{end}</div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_branch", "line": 1, "column": 13}},
    {"id": "H307", "mode": "save", "input": "<div style=\"{if(cam1.online)}\">x{end}{cam1.name}\" title=\"x\">y</div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_branch", "line": 1, "column": 13}},
    {"id": "H308", "mode": "save", "input": "{if(cam1.online)}a{end}", "expect": "ok", "reason": "S2-77 レビュー回帰"},
    {"id": "H309", "mode": "save", "input": "{if cam1.online}a{elif(obs.streaming)}b{end}", "expect": "ok", "reason": "S2-77 レビュー回帰"},
    {"id": "H310", "mode": "save", "input": "{if\"a\"==cam1.name}a{end}", "expect": "ok", "reason": "S2-77 レビュー回帰"},
    {"id": "H311", "mode": "save", "input": "<table>&#4294967328;</table>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H312", "mode": "rendered", "input": "<table>&#4294967328;</table>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H313", "mode": "save", "input": "<table>&#x100000020;</table>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H314", "mode": "rendered", "input": "<table>&#x100000020;</table>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_nesting", "line": 1, "column": 8}},
    {"id": "H315", "mode": "save", "input": "<span style=\"x:/&#42;{cam1.kbps}*/\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 22}},
    {"id": "H316", "mode": "save", "input": "<span style=\"content:&quot;{cam1.kbps}&quot;\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 28}},
    {"id": "H317", "mode": "save", "input": "<span style=\"x:&#117;{cam1.kbps}\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 22}},
    {"id": "H318", "mode": "save", "input": "<span style=\"x:{cam1.kbps}&#40;1)\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 16}},
    {"id": "H319", "mode": "save", "input": "<span style=\"x:{cam1.kbps}&#101;3\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 16}},
    {"id": "H320", "mode": "save", "input": "<span style=\"x:&#64;{cam1.kbps}\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 21}},
    {"id": "H321", "mode": "save", "input": "<span style=\"x:&#35;{cam1.kbps}\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 21}},
    {"id": "H322", "mode": "save", "input": "<span style=\"content:&apos;{cam1.kbps}&apos;\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 28}},
    {"id": "H323", "mode": "save", "input": "<span style=\"&#47;&#42;{cam1.kbps}&#42;&#47;color:red\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 24}},
    {"id": "H324", "mode": "save", "input": "<img src=\"/source/files/png\">", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H325", "mode": "rendered", "input": "<img src=\"/source/files/png\">", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H326", "mode": "save", "input": "<img src=\"/source/files/a/svg\">", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H327", "mode": "rendered", "input": "<img src=\"/source/files/a/svg\">", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H328", "mode": "save", "input": "<span style=\"x:url(/source/files/x.y/png)\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 20}},
    {"id": "H329", "mode": "rendered", "input": "<span style=\"x:url(/source/files/x.y/png)\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_url", "line": 1, "column": 20}},
    {"id": "H330", "mode": "save", "input": "<{cam1.name}>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 2}},
    {"id": "H331", "mode": "rendered", "input": "<{cam1.name}>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 2}},
    {"id": "H332", "mode": "save", "input": "<div {cam1.name}=\"x\"></div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 6}},
    {"id": "H333", "mode": "rendered", "input": "<div {cam1.name}=\"x\"></div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 6}},
    {"id": "H334", "mode": "save", "input": "<div title={cam1.name}></div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 12}},
    {"id": "H335", "mode": "rendered", "input": "<div title={cam1.name}></div>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.html_template_position", "line": 1, "column": 12}},
    {"id": "H336", "mode": "save", "input": "<span style=\"x:url(\\\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.css_escape", "line": 1, "column": 20}},
    {"id": "H337", "mode": "rendered", "input": "<span style=\"x:url(\\\"></span>", "expect": "error", "reason": "S2-77 レビュー回帰", "error": {"code": "template.css_escape", "line": 1, "column": 20}},
    {"id": "H338", "mode": "rendered", "input": "<div style=\"{if(cam1.online)}color:red\">x{end}</div>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H339", "mode": "rendered", "input": "<div style=\"{if(cam1.online)}\">x{end}{cam1.name}\" title=\"x\">y</div>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H340", "mode": "rendered", "input": "{if(cam1.online)}a{end}", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H341", "mode": "rendered", "input": "{if cam1.online}a{elif(obs.streaming)}b{end}", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H342", "mode": "rendered", "input": "{if\"a\"==cam1.name}a{end}", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H343", "mode": "rendered", "input": "<span style=\"x:/&#42;{cam1.kbps}*/\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H344", "mode": "rendered", "input": "<span style=\"content:&quot;{cam1.kbps}&quot;\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H345", "mode": "rendered", "input": "<span style=\"x:&#117;{cam1.kbps}\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H346", "mode": "rendered", "input": "<span style=\"x:{cam1.kbps}&#40;1)\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H347", "mode": "rendered", "input": "<span style=\"x:{cam1.kbps}&#101;3\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H348", "mode": "rendered", "input": "<span style=\"x:&#64;{cam1.kbps}\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H349", "mode": "rendered", "input": "<span style=\"x:&#35;{cam1.kbps}\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H350", "mode": "rendered", "input": "<span style=\"content:&apos;{cam1.kbps}&apos;\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H351", "mode": "rendered", "input": "<span style=\"&#47;&#42;{cam1.kbps}&#42;&#47;color:red\"></span>", "expect": "ok", "reason": "描画後の波括弧はリテラル"},
    {"id": "H352", "mode": "save", "input": "<span style=\"{if(cam1.online)}\">x{ end }</span>", "expect": "error", "reason": "字句による枝の検査", "error": {"code": "template.html_branch", "line": 1, "column": 14}}
  ]
}

```

## 8. 現行との差・レビューで決める所

- （spec と食い違い: 現行 `internal/sources/template/template.go` は全文で `{}` を読み style/コメントも除外しない。Render は平文として値を返し HTML エスケープしない。既存の呼出側の扱いをそのまま HTML 本文へ転用しない。）
- （spec と食い違い: 現行 `internal/web/assets/app/template-lexer.js` は style/コメントの状態を持たず UTF-16 offset を使う。エディターのトークン位置と API のコードポイント位置を区別する必要がある。）
- （spec と食い違い: CheckPath は `a..png` を名前の正規表現上許すが、第9章の URL 規則は `..` を含むものを拒否する。本文の URL では追加拒否し、索引の名前を勝手に変更しない。）
- CheckPath の末尾ドット拒否・Windows 予約名・拡張子の大小無視を URL にも適用する。存在・内容・リンクの検査は素材索引の責務として保存検査から分ける。
- この設計で選んだ細部: data/aria の接尾辞の正規表現、字句誤りの優先順位と位置、CSS の区切り、DOM 不合格時の不挿入。spec の明文ではないのでレビュー対象。
- v3 で閉鎖確認 N1〜N8、v2 で sol・Fable のレビューと指揮者の決めを反映した。JSON の独立ファイル化と実装は別作業。この変更では spec・task 表・コードを変更しない。

## 9. S2-78 への申し送り

- chromium-devtools で全 ok 行の DOMParser の木と走査器の木の一致を調べる（tbody 補完は比較前に反映）。文字列でなく木（要素・属性・テキスト・コメントの構造と値）で比較する。innerHTML はコメント外の `--!>` や本文の `"` の出し方が異なるため比較に使わない。本文の CR と CRLF は DOM では LF になるので比較用の木にも反映する。明示した制約より広いブラウザの木構築の保証はここで確認する。
- 同じ id の重なりを含む本文で更新対象を取り違えないことを確認する。
- 出力に残るコメントの扱いを試験で固定する。保存時はコメント内を検査/展開せず、出力は原文保持とする。
- source.js が本文の id/class によって外枠を取り違えないよう、所有する外枠への参照を使う試験を設ける。
- 偽の節だけでなく本物の DOMParser・プレビュー iframe で CSP、要求と挿入の有無、前回表示の保持を確認する。
