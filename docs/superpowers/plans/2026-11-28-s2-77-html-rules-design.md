# S2-77 HTML 本文検査の設計案

2026-11-28。対象: task S2-77、spec v0.31 第6・9・14章。Fable・codex sol のレビューと指揮者の承認待ち。実装は行わない。以下の JSON は将来のファイルの内容案であり、この変更では文書にだけ置く。

## 1. 許すものの表

表にない要素・属性は拒否。要素名・属性名・CSS の関数名と at-rule 名は ASCII 小文字にして比較する。属性値・素材パスは小文字化しない。`data-*`・`aria-*` の正規表現はこの設計の提案（接尾辞は1文字以上）。`style` は全要素の属性として許すが、差し込みは別規則。

`internal/sources/html_rules.json` 案:

```json
{
  "version": 1,
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
  "templateForbiddenAttributes": [
    "id",
    "width",
    "height",
    "lang",
    "dir",
    "src"
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
      "-webkit-image-set",
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
    ]
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
7. 属性値の固定文字部分だけを HTML の属性値文脈で1回文字参照復号する（数値・名前つき参照、セミコロン省略時の属性文脈も HTML と同じ）。戻した文字を HTML やテンプレートとして再字句化しない。`&#123;cam1.state&#125;` は差し込みではない。復号された NUL・置換前の数値参照ゼロも拒否。`<style>` の生テキストでは文字参照を戻さない。本文の文字参照は文字として保持し、URL 検査には使わない。
8. 復号後の CSS/URL を検査する。復号文字から元入力への対応を保持し、参照から生じた違反は `&` の位置へ戻す。組み立ては固定文字の意味を保ち、差し込む値を必ず HTML エスケープする。属性は二重引用符用に `& < > " '` を逃がす。

位置は元入力の Unicode コードポイント、行・文字目とも1始まり。LF で行を進め文字目を1へ戻し、CR は通常の1文字（CRLF は LF で改行）。Go の byte offset と JS の UTF-16 offset をそのまま返さない。EOF は末尾の次。制御タグだけの行の除去や復号で元位置を失わない。

誤りは左から走査して最初に確定したもの1件。禁止要素/属性は名前が確定した時点で報告。位置は要素なら `<`、属性なら属性名先頭、属性値の引用符誤りなら値先頭、CSS の違反なら関数名・at-rule の `@`・逆斜線、URL なら値の先頭。閉じ不足は開始 `<`、枝の不整合は境界の `{`。属性内のテンプレートの制御タグ不足などは既存 `template.syntax` を使う。

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
| template.html_branch | 枝の中で要素を閉じてください | Close elements within each branch | 枝境界 `{` |
| template.html_template_position | この位置に差し込みや条件は書けません | Template tags are not allowed here | `{` |
| template.html_style_type | style には数値だけ差し込めます | Only number substitutions are allowed in style | `{` |
| template.html_url | この URL は使えません | URL is not allowed | URL 先頭 |
| template.css_escape | CSS の逆斜線は使えません | CSS backslashes are not allowed | `\` |
| template.css_import | CSS の import は使えません | CSS imports are not allowed | `@` |
| template.css_function | この CSS の URL 関数は使えません | CSS URL function is not allowed | 関数名 |
| template.css_syntax | CSS の URL またはコメントを正しく閉じてください | Invalid CSS URL or comment | 開始位置 |

既存の `template.syntax`・`unknown`・`format`・`comparison` は維持。文の末尾には既存同様 `{line}` 行 `{column}` 文字目 / line {line}, column {column} を加える。API は code、翻訳済み error、line、column。保存 POST/PUT は422、preview は200の template_error（HTML は返さない）。

## 3. 文法・型

| 項目 | 規則 |
|---|---|
| 要素 | スタックで一致する閉じを要求。br/hr/img は積まず、`/>` も許す。ほかの自己終了、void の終了要素、省略閉じは拒否 |
| if | 入口の要素スタックを枝ごとに複製。elif/else/end で入口と同一でなければ html_branch。入口以前に開いた要素も枝で閉じられない。else がない空枝も検査。全枝を検査し実行時の真偽で省かない |
| 属性の if | 許可された1つの引用符値の内部で完結。属性や要素をまたぐ制御を拒否。枝ごとに HTML 状態も入口と一致させる |
| 差し込み | 本文と JSON の許可属性のみ。id/width/height/lang/dir/src、名前、無引用値には禁止 |
| style 属性 | Number の差し込みのみ。Seconds/Clock/String/Bool と if/elif/else/end を拒否。形は既存 Number の0〜6。未定義は空。数値の仮トークンで CSS の検査を通し、描画後も再検査 |
| style とコメント | 中の `{}` は CSS/コメントの文字。既知の値らしい記述は style の注意にするだけで参照に数えない |

入れ子の「誤り」は単なるスタック一致だけでなく、ブラウザが閉じを補う/移動する配置も対象にする設計案。p の中に div/p/section/header/footer/h1〜h6/ul/ol/table/hr を置く、li の中に親のリストを挟まず li を置く、table 系の順序違いは拒否する。table の直下は thead/tbody/tr、thead/tbody の直下は tr、tr の直下は th/td（空白・コメント・制御タグを除く）。table 内の非空白文字の移動も拒否。直接 tr はブラウザの tbody 補完を許す。一般の HTML 適合性検査全体を追加するかは承認事項（第8節）。

## 4. CSS と URL

1. style 属性は復号後、style 要素は生テキストで検査。全域の `\` は文字列・コメントの中も拒否。コメント `/*…*/` と引用符（単/二重とも CSS 内では可）を状態で読む。閉じないコメント・文字列・url は css_syntax。
2. CSS コメントをトークン間の空白として扱う。識別子の文字をコメントの前後で連結しない。ASCII 大小を無視して `@import` と JSON の禁止関数を拒否。通常文字列やコメント内の同じ文字は関数・at-rule とみなさない（逆斜線は例外）。`@ import` は import トークンではなくブラウザでも import にならない。`URL(...)` は url と同じ。
3. 関数トークンは識別子直後の `(`。`url (...)` は url 関数にならない。`url(` の前後にあるコメント/空白は CSS の区切りとして扱い、中身はトークンとして単一の引用符文字列か無引用 URL、前後の空白/コメントだけを許す。無引用の内部空白・引用符・`(`、値の後の余分なトークンは拒否。空 url は拒否。通常の calc/rgb/var 等を一律拒否しない。URL を持つ新しい関数は許可せず、一覧更新のレビュー対象とする。
4. URL の値は HTML/CSS の構文を除いた文字列そのもの。trim や URL 解決、percent decode、パス正規化で危険入力を救済しない。`//`・`http:`・`..` を含む値を拒否。素材は `/source/files/` の接尾辞に CheckPath と同じ段数/長さ/名前/予約名/末尾ドット/拡張子を適用し、さらに `..` 部分文字列、`?`、`#`、`%` を拒否。拡張子だけ大小を区別しない。存在確認は保存の合否から分ける。
5. img.src は素材か `data:image/…`。CSS url は素材、`data:image/…`、`data:font/…`、`#名前`。data は該当 MIME 接頭辞でデータ部を持つもの（MIME の比較は ASCII 大小を無視）、クエリ/fragment を外部 URL として解釈しない。`javascript:`、別 MIME、相対 URL、外部 URL は拒否。data の内容の画像検証は本文検査の対象外。CSS の `#名前` は空でない同文書 fragment、素材に付く `#` は拒否。
6. CSS を全文書き直すパーサーは作らず、文字列/コメント/識別子/関数/URL を走査する。意味のない CSS 宣言自体はブラウザに任せる。CSS エスケープを禁止するため `u\72l`・`@\69mport` の難読化は通らない。`image-set("https://…")` は URL 関数の禁止で止める。

CSP（第6章）は `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:; base-uri 'none'; form-action 'none'`。インライン script は許さず、style の許可は本文と OBS カスタム CSS のため。CSP を本文検査の代用にしない。

## 5. 組み立て後の再検査

保存検査 → 全枝の AST と参照集合 → 値をエスケープして組み立て → S2-85 の素材ハッシュ付与 → SSE/preview → 共通 JS 再検査 → 写し込み/差分更新、の順。

管理ページと source.js は `DOMParser.parseFromString('<!doctype html><html><head></head><body>' + html, 'text/html')` を使い、body.childNodes から下だけ検査。自動補完された html/head/body は検査対象外、tbody は許可表にある。先頭と途中の style は body 内で検査する。DOM の属性値は既に復号済みなので二重復号しない。style.textContent は生テキストのまま検査。テンプレートの再解釈はしない。

保存時の厳密な字句/枝検査と違い、DOM 再検査では元の引用符・重複属性・省略閉じを復元できない。表と CSS/URL の同じ関数を rendered モードで適用する。差は素材パス末尾の `?v=[0-9a-f]{12}` を1つだけ許すこと。任意クエリ・重複 v・fragment は拒否。本体が生成した文字列であること自体をブラウザで証明はできず、保存モードで利用者の v を拒否することと組み合わせる。

木をすべて検査してから iframe 文書へ importNode する。管理ページ自身には innerHTML で入れない。iframe は sandbox="allow-same-origin"、外枠は認証つき /api/sources/frame、OBS と同じ CSP。source.js も同じ関数で全木の検査成功後に id/位置/要素名の差分を適用する。不合格なら受信した節を写さず誤りを表示/記録し、危険な節を部分挿入しない。初回外枠は空。

## 6. 素材の使用判定（S2-85）

保存検査で得た全枝 AST の img.src と style 属性・style 要素の url トークンから、復号後の `/source/files/…` を集め、完全一致の集合にする。実行しない枝も含め、data/fragment/コメント/普通の文字/通常属性に書いた URL 文字列は数えない。部分一致しない。組み立て後から逆算せずクエリなしの保存パスを持つ。欠損素材も集合に残し、保存は許して注意だけ返す。索引のパスは大小を含め文字どおり比較し、削除の409にソース表示名を使う。hash は SHA-256 の先頭12桁、同名更新でも URL の変更で HTML を送る。参照の書換えはトークン位置で行い文字列全置換はしない。

## 7. Go/Node 共通入力表

`internal/sources/html_cases.json` 案。以下は JSON の表で1行1件。defs は全件共通、mode は save / rendered。ok には error を付けず、失敗には code/line/column。refs は期待する素材パス（プレフィックス込み、重複なし）で、省略は空集合。rendered 件は Go でも URL/CSS の rendered モードを試し、Node では DOMParser 相当のブラウザ試験も併用する（Node 標準には DOMParser がない）。生字句の試験を DOM の自動修復で置き換えない。

```json
{
  "version": 1,
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
    {"id": "H025", "mode": "save", "input": "<table><tr><td>x</td></tr></table>", "expect": "ok", "reason": "許可要素 td"},
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
    {"id": "H070", "mode": "save", "input": "<img src=\"/source/files/a.png\">", "expect": "ok", "reason": "入れ子も完全一致", "refs": ["/source/files/a.png"]},
    {"id": "H071", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a.png\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a.png"]},
    {"id": "H072", "mode": "save", "input": "<img src=\"/source/files/a/b/c/x.png\">", "expect": "ok", "reason": "入れ子も完全一致", "refs": ["/source/files/a/b/c/x.png"]},
    {"id": "H073", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a/b/c/x.png\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a/b/c/x.png"]},
    {"id": "H074", "mode": "save", "input": "<img src=\"/source/files/a.b/x.SVG\">", "expect": "ok", "reason": "入れ子も完全一致", "refs": ["/source/files/a.b/x.SVG"]},
    {"id": "H075", "mode": "save", "input": "<style>.x{background:url(\"/source/files/a.b/x.SVG\")}</style>", "expect": "ok", "reason": "CSS 素材", "refs": ["/source/files/a.b/x.SVG"]},
    {"id": "H076", "mode": "save", "input": "<span style=\"background:url( /*a*/ #grad /*b*/ )\"></span>", "expect": "ok", "reason": "文字列・コメント・区切り"},
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
    {"id": "H184", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abc\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H185", "mode": "rendered", "input": "<img src=\"/source/files/a.png?x=abcdef123456\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H186", "mode": "rendered", "input": "<img src=\"/source/files/a.png?v=abcdef123456#x\">", "expect": "error", "reason": "生成クエリの再検査", "error": {"code": "template.html_url", "line": 1, "column": 11}},
    {"id": "H187", "mode": "save", "input": "{if cam1.online}<img src=\"/source/files/a.png\">{else}<style>.x{background:url(/source/files/b.png)}</style>{end}", "expect": "ok", "reason": "全枝の集合", "refs": ["/source/files/a.png", "/source/files/b.png"]},
    {"id": "H188", "mode": "save", "input": "<img src=\"/source/files/a&#46;png\">", "expect": "ok", "reason": "復号して素材参照", "refs": ["/source/files/a.png"]},
    {"id": "H189", "mode": "save", "input": "<style>.x{background:url(/source/files/a&#46;png)}</style>", "expect": "error", "reason": "raw CSS は復号しない", "error": {"code": "template.html_url", "line": 1, "column": 26}}
  ]
}

```

## 8. 現行との差・レビューで決める所

- （spec と食い違い: 現行 `internal/sources/template/template.go` は全文で `{}` を読み style/コメントも除外しない。Render は平文として値を返し HTML エスケープしない。既存の呼出側の扱いをそのまま HTML 本文へ転用しない。）
- （spec と食い違い: 現行 `internal/web/assets/app/template-lexer.js` は style/コメントの状態を持たず UTF-16 offset を使う。エディターのトークン位置と API のコードポイント位置を区別する必要がある。）
- （spec と食い違い: CheckPath は `a..png` を名前の正規表現上許すが、第9章の URL 規則は `..` を含むものを拒否する。本文の URL では追加拒否し、索引の名前を勝手に変更しない。）
- CheckPath の末尾ドット拒否・Windows 予約名・拡張子の大小無視を URL にも適用する。存在・内容・リンクの検査は素材索引の責務として保存検査から分ける。
- この設計で選んだ細部: data/aria の接尾辞の正規表現、字句誤りの優先順位と位置、CSS の区切り、DOM 不合格時の不挿入。spec の明文ではないのでレビュー対象。
- 未確定: 入れ子の検査をブラウザの補完で木が変わる全配置まで広げるか、上記の明示的な制約で足りるか。追加するなら共通表に追記してから実装する。data URL の MIME/データの細かな文法と同文書 fragment の名前の許容文字も spec に詳細がないため、第4節の最小条件を案とする。新しい CSS URL 関数への追随方針は一覧更新でよいか確認する。
- Fable・sol の今回のレビューは未実施。依頼で列挙された抜け道は入力表に収録した。未提供のレビュー原文まで網羅したという主張はしない。指揮者承認後に JSON を独立ファイル化し、Go/Node の同一入力・同一 code/位置/参照集合を合格条件にする。
