# S2-87 素材の日本語の名前 — 設計 v1

2026-11-28。Fable と codex sol のレビュー待ち。以下の細部は設計者の推奨案であり、レビュー済み・実装済みではない。

目的は、S2-82 実機結果 F-7・気になった点6の `__-203121.png` への置換を解消し、ユーザーが決めた「日本語の名前を許す」を実現すること。対象は画像ファイルとその親フォルダの名前。本文の安全な文字集合を広げるだけで、S2-77 v3.2 のスクリプト禁止、外部要求禁止、文字列検査前の DOMParser 禁止、木の一致、枝の検査、CSS escape 禁止、テンプレート位置制限は変えない。

この文書だけを追加する。コード、試験、spec、task 表、結果、チケットは変更しない。依存追加・試験追加・仕様改訂はレビュー後の実装役と指揮者への提案である。

## 1. 現状と変更の境界

| 現在の入力・処理 | 期待する変更／維持 | 理由 |
|---|---|---|
| `SafeFileName("夕焼け.png")` | `夕焼け.png` を返す | 今は stem の非 ASCII を `_` に置換する |
| `CheckPath`、索引、`Delete` が別々に名前を検査 | 共通の名前検査へ揃える | Delete にも古い `fileNamePattern` が残っている |
| `html_rules.json` version 3 の ASCII segmentPattern | 第2節の名前規則に置換 | Go と JS で同じ結果を得る |
| `html.go`・`html_template.go` の参照抽出と書換え | 日本語の生パスを全枝から抽出し、同じ位置情報で書換え | S2-85 の完全一致と欠損注意を維持 |
| `sources-state.js` 等の `encodeURIComponent` による本文差し込み | 本文へは生パスを差し込む | 現行は ASCII なので表面化しなかった不一致 |
| HTTP 配信・削除 API の URL 組立て | 通信時は各 segment を符号化してよい | 本文の構文と HTTP の表現を分ける |
| 画像型、拡張子、容量・件数、symlink、`os.Root` | 維持 | 名前を広げてもファイルアクセスの境界は広げない |

参照元は spec v0.32 第14章の素材、S2-77 v3.2 第1・2・4〜7節、`internal/sources/files.go`・`filesindex.go`、`internal/web/source_files.go`、`internal/web/assets/app/html.js`・`sources-state.js`・`editor.js` 等。現行 API は重複時409で、自動連番ではない。

## 2. 許す文字と長さ

**推奨: NFC の Unicode L・N・M と ASCII `_` `-` `.` の限定許可。** 分類表は Unicode 15.0 に固定する。実行環境の Unicode 更新で Go と JS の合否が変わらないよう第7節の共通範囲表を使う。L は Lu/Ll/Lt/Lm/Lo、N は Nd/Nl/No、M は Mn/Mc/Me。次の除外・位置制約が分類より優先する。

| 入力（各部分） | 期待 | 理由 |
|---|---|---|
| `夕焼け.png`、`カメラー１.png`、`ＡＢＣ１２.png` | 可 | 漢字・かな、長音 U+30FC（Lm）、全角英数字も含む |
| `é.png`、`q\u0301.png`（実際の結合文字） | NFC なら可 | 合成されず残る M も必要。結合文字を一律禁止しない |
| `\u0301a.png`、`_\u0301.png`、`a.\u0301png` | 不可 | M はその部分の L/N に始まる連続した L/N/M 列の中だけ。先頭、`_-.` の直後は禁止 |
| `_画像-2.v1.png` | 可 | 既存の ASCII の有効名を維持 |
| `.画像.png`、`画像.`、空、`.`、`..`、`a..png` | 不可 | 先頭・末尾ドットと連続ドットを禁止。URL 側の既存 `..` 禁止に揃える |
| `{ }`、二重/単一引用符、バッククォート、`< > & ( )` | 不可 | HTML・テンプレート・CSS の構文文字を入れない |
| `% ? # / \ : * \|`、全角 `／` `＼` | 不可 | URL・パス・Windows の特殊解釈を避ける。`/` は部分間の区切りだけ |
| SP、TAB、CR/LF、NBSP、全角空白 U+3000、その他 Z | 不可 | 空白類はすべて拒否、trim で救済しない |
| C0/C1/DEL、Cf（双方向制御・ゼロ幅・BOM）、Co/Cn/Cs | 不可 | 制御・書式・私用・未割当・サロゲートを除外。無効 UTF-8／孤立 UTF-16 surrogate も拒否 |
| U+034F、U+180B〜180D、U+180F、U+FE00〜FE0F、U+E0100〜E01EF | 不可 | M の中でも不可視の結合制御・異体字選択を除く |
| 絵文字、`★`、`・`、全角記号 `：` `．` | 不可 | L/N/M と ASCII `_-.` 以外の S/P は許可しない |

各部分は **UTF-8 で1〜128バイト**、相対パス全体は `/` を含め **512バイト以下**。接頭辞 `/source/files/` と生成 query は数えない。拡張子も部分の長さに含める。最大4部分（親フォルダ3段＋ファイル）。末尾は既存の画像拡張子必須、拡張子の比較のみ ASCII 大小無視。M の個数制限を別に設けずバイト上限で抑える。

| 入力 | 期待 |
|---|---|
| `あ`×41 + `.png`（127バイト） | 可 |
| `あ`×42 + `.png`（130バイト） | 長すぎるため不可 |
| ASCII `a`×124 + `.png`／×125 + `.png` | 128バイトで可／129バイトで不可 |
| 128バイトの部分を4つ、区切り3つ | 515バイトで不可。各部分が通っても全体を検査 |
| `a/b/c/夕.png`／`a/b/c/d/夕.png` | 可／`too_deep` |

比較案は「128コードポイント・512コードポイント」まで。日本語の長名には便利だが、一般的なファイルシステムの部分長の制約を超え得る。バイト案は現行 Go の `len(rel)` の意味と ASCII の境界を維持し、最大128 UTF-16単位以内にも収まるので推す。spec の「文字」は明確に改訂する必要がある。Windows の絶対パス長や保存先の制約による I/O 失敗までは、この論理上限だけで保証しない。

## 3. 正規化

**推奨: Go に `golang.org/x/text/unicode/norm` を追加して NFC を使う。** バージョンは実装時に Go 1.27 と Unicode 15.0 の規則との互換性を確認して固定する。JS は `String.prototype.normalize('NFC')` を使うが、第7節の範囲制限が先に働く。NFKC・幅変換・かな変換はしない。

| 入口の入力 | 期待 | 理由 |
|---|---|---|
| アップロード `か\u3099.png` | NFC `が.png` で保存し、実保存名を返す | macOS 由来の NFD を取り込める |
| 直接配置 `か\u3099.png` | 改名せず一覧に残し `not_nfc` で使えない | 実ファイル名と索引キーを違わせない |
| 直接配置 `が.png` | 可 | Explorer 由来でも出自を推測せず同じ検査 |
| 本文 `か\u3099.png` の参照 | 保存・rendered とも `template.html_url` | 本文を黙って書換えず NFC を要求 |
| `Ａ.png` と `A.png` | 別名 | NFC は全角半角を統合しない |

もう1案は依存を足さず M を禁止し、NFD を理由つきで拒否する方式。しかし Go 標準には NFC 判定・変換がなく、M 禁止だけでは分解ハングルや正規分解を持つ L の非 NFC を検出しきれない。独自正規化表を持つより依存追加を推す。`norm` の説明は [Go norm](https://pkg.go.dev/golang.org/x/text/unicode/norm) を参照。

## 4. Windows の名前と同名衝突

名前の綴りを保存し、参照と索引は今どおり大小も含む完全一致。**衝突の検出だけ**大小無視にする。これは `a.png` と書けば `A.png` を配るという意味ではない。

| 入力・状態 | 期待 | 理由 |
|---|---|---|
| `A.png` があり `a.png` をアップロード | 409、上書き・自動改名なし | Windows で共存できない |
| `Ａ.png` があり `A.png` をアップロード | 別名として可 | 幅を畳まない |
| Linux で大小違いを直接配置 | 両方 `name_collision` で使えない | 索引順に勝者を決めない。親フォルダ同士の衝突も配下全体に適用 |
| `CON`、`con.png`、`PRN.x.png`、`AUX`、`NUL`、`COM1`〜`COM9`、`LPT1`〜`LPT9`（拡張子つき含む） | 全部分で `reserved_name` | 最初の ASCII `.` より前を大小無視で比較 |
| `COM¹.png`、`COM².png`、`COM³.png`、対応する `LPT` | 同じく予約名 | No を許すため上付き1〜3の Windows 予約も除く |
| `COM0.png`、`COM10.png`、`ＣＯＮ.png` | 可 | 予約名との幅変換はしない |
| `画像.`、`画像 ` | 直接配置は不可。黙って削らない | Win32 の末尾処理で名前が変わることを避ける |
| `GAZOU~1.PNG` | 不可 | `~` は許可集合外。8.3 alias を素材 ID として扱わない |

衝突キーは NFC の各部分を Unicode 15.0 の simple case folding の同値類で揃えたものとし、幅を畳まない。参照検査にはキーを使わない。OS の大小比較と Unicode folding は完全には一致しないので、最終保存も `os.Root` と排他的作成 `O_EXCL` を維持し、OS が既存扱いしたら409。8.3 alias の検索や実名への自動解決はしない。配信・削除は索引に完全一致する実名だけを開く。

根拠は [Microsoft の名前規則](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file)。Unicode folding は Windows の内部比較の再実装ではなく、移植時に曖昧な同居を避けるための保守的なアプリ規則である。

## 5. 本文の参照と HTTP

**推奨: 保存本文と組立て HTML は日本語を生の文字で書く。percent encoding は本文では引き続き拒否する。** 比較案は両方を許して1回復号し同じキーにする方式だが、S2-77 第4節の percent decode 禁止を変え、区切りや二重復号への入口を増やすため採らない。

| 入力 | 期待 | 理由 |
|---|---|---|
| `src="/source/files/夕焼け.png"` | 可、refs は `/source/files/夕焼け.png` | 生パスが唯一のキー |
| `src="/source/files/&#22805;焼け.png"` | 可、同じ refs | 第2節の文字参照を1回戻した値で検査 |
| `src="/source/files/%E5%A4%95焼け.png"` | `template.html_url`、refs に入れない | 生形式と符号化形式を両方受理しない |
| `%25E5...`、`%2f`、`%5c`、`%2e%2e` | 本文ではすべて不可 | URL 解決・復号・clean はしない |
| CSS `url(/source/files/夕焼け.png)`、`url("/source/files/夕焼け.png")`、単一引用符版 | 可、同じ refs | 名に空白・括弧・引用符を許さないので引用符なしも安全 |
| style 属性の `url(&quot;/source/files/夕焼け.png&quot;)` | 可 | 属性文字参照は1回復号 |
| style 要素内の `url(/source/files/&#22805;焼け.png)` | 不可 | style 生テキストでは文字参照を戻さない |
| 保存本文の `夕焼け.png?v=0123456789ab` | 不可 | query は本体だけが付ける |
| rendered の同パス＋`?v=0123456789ab` | 可 | 既存の12桁小文字 hex、重複 query 不可 |

使用判定は全枝の正規の生パスの集合。文字参照版と生文字版は同じ素材になり、欠損も refs に保持して `template.asset_missing`。大小違い、全角半角違いは別キー。生成 hash は既存 SHA-256 先頭12桁を維持する。トークン位置による `?v=` の追記も維持し、文字列全置換はしない。固定文字参照の原文を残してもよいが、生成される名前自体を percent encoding へ変換しない。

ブラウザは日本語を含むパスを HTTP 要求時に UTF-8 percent encoding する（[URL Standard](https://url.spec.whatwg.org/)）。Go の [URL.Path](https://pkg.go.dev/net/url#URL) は復号された表現を持つ。配信・削除 API は `r.URL.Path` を再度 PathUnescape せず検査・索引完全一致へ渡す。CEF でもこの動作を期待するが、OBS 実機での確認は未実施であり実装後の受入項目とする。

| HTTP の入力 | 期待 |
|---|---|
| `/source/files/%E5%A4%95%E7%84%BC%E3%81%91.png` | `r.URL.Path` の日本語が有効な索引キーなら200 |
| `%252f` | 1回の復号後に `%2f` が残り、名前検査で404 |
| encoded slash/backslash、dot segment、空部分 | `EscapedPath()` の検査と復号後検査で拒否。clean/redirect で有効パスへ救済しない |
| `/source/files/GAZOU~1.PNG` | 索引名として不許可、404 |

HTTP の raw path では `%2f`・`%5c`（大小無視）を拒否し、他の escape も UTF-8 と復号後の名前の両方を検査する。API の削除に使う segment ごとの符号化は継続する。DOM 再検査は URL 解決済みの `img.src` ではなく `getAttribute('src')` を読む。そうしないと生文字の正規の参照をブラウザの絶対 URL／符号化へ変換して誤拒否する。

## 6. 取り込みと直接配置

取り込みは「ヘッダーの名前取得 → 妥当な UTF-8・パス混入検査 → NFC → 最終拡張子の確認・ASCII 小文字化 → stem の不許可文字だけ置換 → 全規則の再検査 → 衝突検査 → 排他的保存」の順。置換は不許可コードポイント1つにつき `_` 1つとし、連続 `_` を圧縮しない。既存の有効な途中の `.` も残す。先頭の `.`、位置に合わない M、末尾の `.` は `_` にするが、`..` を含む元名は従来どおり入口で拒否する。長すぎる名前・予約名は切り詰めや接頭辞追加で救済せず拒否する。

| 入力 | 期待 | 理由 |
|---|---|---|
| `夕 焼け（夜）.PNG` | `夕_焼け_夜_.png` | 空白・全角括弧だけ置換 |
| `夕焼け.v1.png` | そのまま | 有効な途中ドットを失わない |
| `.夕焼け.png` | `_夕焼け.png` | 先頭ドットだけ置換 |
| `CON.png` | 400 | 予約名は明示的に拒否 |
| `a/b.png`、`a\b.png`、`a..png` | 400 | basename を取って通さない |
| `a／b.png` | 400 | 全角区切りもパス混入として拒否。通常置換より優先 |
| 既存 `夕_焼け.png` に `夕 焼け.png` | 409 | 置換後の衝突も同名として扱う |
| 既存 `が.png` に NFD の同名 | 409 | NFC 後の名前で判定 |
| 不正 UTF-8 の filename | 400 | 文字化けを置換して保存しない |

ブラウザの通常の multipart `filename` の日本語 UTF-8 を受け付ける。URL と違い `filename` に現れた `%E5...` は復号しない（`%` を `_` にする）。`filename*` は Go の MIME パラメーター解釈で UTF-8 の拡張パラメーターを1回だけ復号した結果を使い、通常 filename と併記なら有効な拡張値を優先する。ただし不正な拡張値、未対応 charset、重複、連結パラメーターは400として曖昧な fallback を作らない。実装時に現在の Go の受理動作を固定する表試験が必要。

[Go multipart.FileName](https://pkg.go.dev/mime/multipart#Part.FileName) は basename 処理を行うため、今の `FormFile` 後の `h.Filename` だけでは元の `/` 混入を検出できない。生の Content-Disposition も構造として検査する必要がある。文字列の手分割や独自 percent decoder は使わず、標準 MIME パーサーの結果と元パラメーターの有無を確認する。CR/LF 等の壊れたヘッダーは名前置換より前に拒否する。

直接置いたファイルと親フォルダは第2〜4節の同じ完成形規則に通す。変更・NFC 化・置換は一切しない。`not_nfc` と `name_collision` は新しい Reason 案であり、名前の文字・位置違反は既存 `bad_name`、予約名は `reserved_name`、全体長超過は `path_too_long`。理由の優先は構造・深さ・全体長 → 不正文字/部分長 → 非 NFC → 予約名 → 衝突 → 画像型等の既存検査。使えない素材は補完・差し込み・配信の対象外とし、一覧では実名と理由を示す。

削除も古い正規表現を残さず共通検査へ移す。非 NFC など不許可名の削除を新たに有効化することは本件に含めず、Explorer での改名・削除を案内する。既存の使用中素材削除409・確認後削除は維持する。

## 7. Go と JS の一致、試験の追加案

JSON の `asset` に normalization=NFC、lengthUnit=utf8Bytes、maxSegmentLength=128、maxPathLength=512（相対パス）、unicodeVersion=15.0、許可 L/N/M のコードポイント範囲、除外範囲、予約名を持たせる案を推す。単一 `segmentPattern` にすべてを押し込まない。範囲表は固定した Unicode データから作り、Go/JS が同じ JSON を読む。ブラウザ固有の `\p{L}` 判定だけに依存しない。衝突キー用 simple folding の表も同じ版で固定するが、名前検査と参照の完全一致には混ぜない。

| 処理 | Go／JS の期待 | 理由 |
|---|---|---|
| UTF-8 の長さ | Go `len`／JS `TextEncoder` のバイト長 | JS `.length` は UTF-16 単位で不一致になる |
| Unicode の走査 | rune／コードポイント走査 | 補助平面の漢字も1文字として検査 |
| NFC | `norm.NFC`／`normalize('NFC')` | 許可範囲を先に固定し、未割当を後から合成で救わない |
| 字句エラー位置 | 元入力の1始まりコードポイント | S2-77 第2節を維持。バイト長制限とは別 |
| URL の合否 | 生パスのみ、百分号は拒否 | 保存・rendered・DOM 後の検査で一致 |

### 共通入力表の行案

現行の最後は **H359**。追加は H360 から。表の `\uXXXX` は JSON で実文字に展開する記法（本文に逆斜線を入れる意味ではない）。S/R は save/rendered。error の位置は S で必須、R は現行契約どおり code のみ比較する。ok の code・位置はなし。refs は save の期待値で、rendered は現行ハーネスに合わせ検査のみ。存在する fixture は `夕焼け.png` と `が.png` を追加し、欠損試験以外の注意は空にする。

| id / mode | input | 期待（code、行・文字目）／refs |
|---|---|---|
| H360 S / H361 R | `<img src="/source/files/夕焼け.png">` | ok／`/source/files/夕焼け.png` |
| H362 S / H363 R | `<img src="/source/files/&#22805;焼け.png">` | ok／同上 |
| H364 S / H365 R | `<img src="/source/files/%E5%A4%95.png">` | `template.html_url`、S は1・11 |
| H366 S / H367 R | `<img src="/source/files/か\u3099.png">` | `template.html_url`、S は1・11 |
| H368 S / H369 R | `<img src="/source/files/が.png">` | ok／`/source/files/が.png` |
| H370 S / H371 R | `<img src="/source/files/夕 焼け.png">` | `template.html_url`、S は1・11 |
| H372 S / H373 R | `<img src="/source/files/夕\u3000焼け.png">` | `template.html_url`、S は1・11 |
| H374 S / H375 R | `<img src="/source/files/夕\u202E焼け.png">` | `template.html_url`、S は1・11 |
| H376 S / H377 R | `<img src="/source/files/夕／焼け.png">` | `template.html_url`、S は1・11 |
| H378 S / H379 R | `<img src="/source/files/COM¹.png">` | `template.html_url`、S は1・11 |
| H380 S / H381 R | `<style>.x{background:url(/source/files/夕焼け.png)}</style>` | ok／`/source/files/夕焼け.png` |
| H382 S / H383 R | `<style>.x{background:url("/source/files/夕焼け.png")}</style>` | ok／同上 |
| H384 S / H385 R | `<span style="background:url(&quot;/source/files/夕焼け.png&quot;)"></span>` | ok／同上 |
| H386 S | `<img src="/source/files/夕焼け.png?v=0123456789ab">` | `template.html_url`、1・11 |
| H387 R | 同上 | ok |
| H388 S / H389 R | `<img src="/source/files/%252f夕.png">` | `template.html_url`、S は1・11 |
| H390 S / H391 R | `<img src="/source/files/夕&#128;焼け.png">` | `template.html_lex`、S は1・26（`&`） |
| H392 S / H393 R | `<img src="/source/files/夕\uFE0F.png">` | `template.html_url`、S は1・11 |
| H394 S | `<img src="/source/files/欠損.png">` | ok、refs に保持、warnings は `template.asset_missing` |
| H395 R | 同上 | ok（存在は合否に使わない） |

旧行の ASCII・percent encoding 拒否の期待は緩めない。追加行のほか、下記の表試験を実装役が用意する。

| 試験対象 | 入力 → 期待 |
|---|---|
| Go 名前検査表 | 第2節の全分類・境界、全予約名大小混在、上付き数字、前後ドット、空白、`~1`、無効 UTF-8 → 指定 Reason |
| Go/JS 名前検査の共有 fixture | L/N/M 各下位分類、補助平面漢字、未割当、私用、孤立 surrogate → 同じ合否。128/129・512/513バイトも一致 |
| multipart 表 | raw UTF-8 filename、UTF-8 filename*、両方、不正 charset/escape、パス混入、二重符号化 → 第6節の結果 |
| 保存・索引 | 日本語保存→一覧→読出し→削除、NFD 取込→NFC、直接 NFD は使えない、大小衝突、親フォルダ衝突 → 第3・4節の結果 |
| S2-85 統合 | 生文字・文字参照の両方と不実行枝 → 同じ used_by。hash が同じ、削除確認409、欠損注意、同名更新時の hash 更新 |
| HTML 書換え | 第5節の img、style 属性、style 要素の引用符あり/なし → 生パスを保って既存 v だけ追記。外部要求ゼロ、木が同じ |
| HTTP | 正しい日本語 percent encoding →200。二重符号化・encoded separator・dot segment・alias →404（リダイレクト救済なし） |
| Windows/OBS 実機 | 日本語のドロップ・直接配置・補完・表示・同名更新・削除 → 実名表示、CEF の要求と再表示が成功 |

試験の実装・実行は本設計作成には含めない。

## 8. 画面

| 場所 | 入力 → 期待 | 理由 |
|---|---|---|
| 補完候補・素材一覧 | `夕焼け.png` → そのまま表示 | 符号化表示やローマ字化をしない |
| 差し込み | 日本語の候補 → 生パスの img / 引用符つき url | 本文規則と揃える。CSS の引用符は今の挿入形式を維持 |
| 削除確認 | `夕焼け.png` → 実名と使用中ソース表示名を既存の文に差し込む | S2-85 の確認内容を維持 |
| 不許可の直接配置 | 実名＋「名前が NFC ではありません」等 | 理由から改名できる |

表示は textContent・テキストノード・エディターの文字列として扱う。使えない名前は HTML 記号を含む可能性があるので innerHTML に渡さない。画面の名前と保存名・削除要求名を別に変換しない。通信だけを segment ごとに符号化する。

## 9. 残る危険とレビュー依頼

| リスク／レビューで見てほしい所 | 期待する判断・確かめ | 理由 |
|---|---|---|
| ラテン・キリルなど見た目の似た文字 | 同一扱いしない。実名表示を確認 | 文字種の全混同検出は本件に含めない |
| NFC 差の同じ見た目の2名 | NFD 直接配置を使えなくし、勝手に統合しない | 改名・置換で別ファイルを失わない |
| M と不可視文字 | 許可 M・例外範囲・先頭制約を Fable/codex sol に重点レビュー依頼 | 日本語の異体字選択も拒否する提案は利便性との判断が必要 |
| 128/512バイト | 日本語41字＋拡張子で十分かレビュー | 現行 spec の「文字」との変更を明記して採否を決める |
| Unicode 表と NFC 依存 | Unicode 15.0 固定、生成根拠、JS/Go の一致を確認 | 将来の実行環境更新だけで許可範囲を増やさない |
| Windows 大小同値 | simple folding の保守的な衝突判定と OS の差、親フォルダ衝突を確認 | 参照の完全一致を緩めない |
| 二重 URL 復号と区切りの抜け道 | 本文 `%` 禁止、HTTP は1回だけ、raw separator 拒否、索引実名一致を確認 | `os.Root` だけで論理上の別名アクセスを許さない |
| multipart の basename と filename* | Go 標準の実挙動と追加拒否規則を表で確認 | 正常なブラウザ名と偽装されたパスを区別する |
| CEF の要求 | 実機で文字参照、生文字、CSS の両引用形式を観測 | この文書の机上の期待だけで合格にしない |

設計の選択は、NFC 依存追加、生文字だけの本文参照、バイト上限、衝突時の拒否。レビュー未決事項を実装役が黙って別案へ差し替えず、指揮者がレビュー結果を反映した版を渡す。

## 10. spec 改訂案（指揮者向け、この作業では変更しない）

| 箇所 | 現在 → 改訂案 | 理由 |
|---|---|---|
| 第14章「素材」の ASCII 正規表現と128文字・512文字 | 第2節の限定 Unicode 集合・除外・NFC・各部分128 UTF-8バイト・相対パス512バイトへ置換 | 判定単位を明確にする |
| 同章の「取り込みの正規化は今どおり」 | 第6節の順序、NFC、不許可文字のみ `_`、予約名拒否、409維持、filename の規則を明記 | 日本語を消さず衝突を隠さない |
| 同章の直接配置 | 改名せず検査、非 NFC・衝突も理由つきで使えないと追記 | 取り込みとの違いを明記 |
| 第9章の固定素材 URL と S2-77 第4節相当 | 名前の集合だけ更新。percent decode・`%` 禁止は維持。文字参照は既存どおり1回 | 安全の規則を広げない |
| S2-85 の使用判定・v | NFC の生パスを大小含め完全一致、文字参照だけ同一視。通信時の符号化と区別 | 曖昧な参照と二重復号を避ける |
| 素材 UI と補完・差し込みの記述 | 候補と本文は実名の生文字、HTTP 操作だけ符号化と追記 | 利用者が実名を読める |

S2-77 の許可要素・属性・CSS 関数・字句・テンプレート・枝・DOM 再検査の安全規則を改訂対象に含めない。
