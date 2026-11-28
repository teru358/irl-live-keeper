# S2-87 素材の日本語の名前 — 設計 v2.1

2026-11-28。対象は s2-87-design、基点 HEAD 2469a8c。指揮者の決定と Fable・codex sol の v1 レビューを反映した設計であり、実装・実機検証の完了報告ではない。この作業では本書だけを変更する。

## v2・v2.1 の要約とレビュー対応

v2.1：閉鎖確認の4件を反映し、親の未確定時の配信停止、filesMu による更新の直列化、ハングルの生成検証、ReadUpload の part 選択・上限・応答を確定する。

日本語の実名を本文でも使えるようにする目的は維持する。v1 の「L/N/M と NFC ライブラリ」を「必ず NFC になる Unicode 15.0 の L/N の部分集合」に変更する。Go/JS の正規化差と依存をなくすためで、管理ページだけが送信前にブラウザの NFC を使う。長さは相対パスの UTF-8 512バイトに統一する。

| 指摘 | 対応 | v2 での決定・理由 |
|---|---|---|
| Fable I-1、sol Minor 4 | 採った | Default_Ignorable_Code_Point を L/N 内も全除外 |
| Fable I-2、sol Minor 5 | 採った／一部消えた | 15.0 の生成元・SHA-256・再生成を固定。サーバー NFC・norm.Version・CEF に応じた版引下げは不要。folding は Go だけで実施 |
| Fable I-3 | 採った | 拡張子・予約名は自前の ASCII A〜Z folding |
| Fable I-4、sol I-2 | 採った | 実名完全一致、Delete の索引必須、親も含む衝突状態を内容検査から分離して維持 |
| Fable I-5、sol Minor 6 | 採った | Go/JS とも相対 UTF-8 長。既存 H 行の調査結果と新境界行、bodyAssetInsertion を明記 |
| Fable I-6、sol I-3 | Fable 案を採った | 標準 MIME パーサーの結果を検査。sol の独自構造解析・重複等の一律400は採らない。結果を同じ完成形検査に通せば安全境界を保てる |
| Fable I-7 | 採った | 補完・本文差し込み2箇所・html.js の予約名を列挙 |
| Fable I-8 | 採った | 衝突相手・順序、アップロードは直下全項目、NFD/NFC 同居を意図した仕様とする |
| Fable I-9 | 採った | M、非 NFC の L/N、前の文字と合成する L/N を除外。x/text は追加しない |
| Fable I-10、sol の追加行 | 採った／期待変更 | 第7節。M を含む名前は全て html_url。sol の q́ 可という期待は採らない |
| sol I-1 | 消えた | M とサーバー正規化をなくすため stream-safe 制限・CGJ 挿入問題は存在しない |
| Fable M-1 | 採った | unicode.SimpleFold の軌道の最小値。共有 folding 表なし |
| Fable M-2 | 採った | 不正 UTF-8／孤立 surrogate は言語別試験 |
| Fable M-3 | 一部採った／消えた | 直せる案内と互換漢字の注意を残す。not_nfc の Reason は不要 |
| Fable M-4 | 消えた | M の位置判定・出力列に依存する置換順は不要 |
| Fable M-5〜M-8 | 採った | 長名専用文言、raw path 検査は任意、RTL の注意、version 4・表の一度だけの構築・Reason の i18n |
| sol の統合試験4項目 | 採った／期待調整 | 衝突更新、親衝突はそのまま。multipart と encoded slash は第7節の標準パーサー・復号後完全一致の期待へ変更 |

参照したレビューは scratchpad/out/s2-87-review-fable.md と s2-87-review-sol.md。Fable の Go/Node/Chromium 実測と Windows/OBS 未確認を区別する。Unicode 15.0 の除外数は17.0 の実測値を転記せず、実装時の生成結果で確定する。

## 1. 目的と変更箇所

S2-82 F-7 の `__-203121.png` への置換を解消し、画像と親フォルダに日本語の名前を許す。S2-77 v3.2 のスクリプト禁止、外部要求禁止、文字列検査前の DOMParser 禁止、木の一致、全枝検査、CSS escape 禁止、テンプレート位置制限は維持する。画像型・拡張子の集合、容量・件数、symlink 禁止、os.Root、S2-85 の使用判定と削除確認も維持する。

| 場所 | 変更／維持 | 理由 |
|---|---|---|
| internal/sources/files.go | SafeFileName、DetectImage の拡張子比較、Delete、ReadUpload の MultipartReader 移行 | 日本語保存、ASCII folding、索引完全一致 |
| internal/sources/filesindex.go | checkSegment・CheckPath・予約名、再構築・Refresh・OpenWithHash・put | 名前と内容と衝突を分離 |
| internal/sources/html_rules.json | version 3 → 4、固定許可表と長さ・予約名 | Go/JS の共通契約 |
| internal/sources/html.go、html_template.go | checkURL 相当、参照抽出と位置での書換え | 相対バイト数、日本語の生パスを全枝から抽出 |
| internal/web/assets/app/html.js | checkURL、19行付近の予約名正規表現と末尾ドット判定を共通規則へ | COM¹ 等と Go の合否を揃える |
| internal/web/assets/app/source-body.js | bodyAssetInsertion の本文用 encodeURIComponent を除く | 差し込みを生パスへ |
| internal/web/assets/app/sources-state.js | assetText の本文用 encodeURIComponent を除く | 同上。削除要求等の通信の符号化は維持 |
| internal/web/assets/app/template-lexer.js | 97〜98行付近の補完開始判定・validFor | 日本語を打っても補完が閉じない。共通名規則で候補を絞る |
| internal/web/source_files.go、管理ページの送信箇所 | ReadUpload の呼出しとエラー応答、明示した送信 filename に NFC、エラー文言 | 取り込み本体は files.go。API は応答を担当 |
| i18n、Go/Node 試験 | 第7・8節を実装時に追加 | 新 Reason と長名の表示 |

validateTree は既に attr.value を読んでおり、生の属性値を使う境界を維持する。auth.go の配信入口は既に独自振分けで clean/redirect しない。このためだけの変更は不要。

## 2. 許可集合・長さ・予約名

許可集合は Unicode 15.0 の L（Lu/Ll/Lt/Lm/Lo）と N（Nd/Nl/No）から、(1) Default_Ignorable_Code_Point 全体、(2) 単独で NFC でない文字、(3) 前の文字と合成しうる文字を引き、ASCII `_` `-` `.` を加えたもの。M（Mn/Mc/Me）は一切許さない。Go の unicode.IsLetter 等、JS の `\p{…}`、Unicode 大小変換を合否に使わない。

各部分は UTF-8 **1〜128バイト**、相対パスは `/` を含め **512バイト以下**。`/source/files/` と生成 `?v=` は数えない。最大4部分（親3段）。拡張子も部分長に含む。先頭・末尾ドット、空部分、連続 `..`、先頭 `/`、バックスラッシュは禁止。末尾部分だけは既存の画像拡張子を必須とし、フォルダ名には拡張子を要求しない。

| 入力 → 期待 | 理由 |
|---|---|
| `夕焼け.png`、`カメラー１.png`、`ＡＢＣ１２.png`、`é.png`、`한.png`、`ｶﾞ.png` → 可 | 合成済み文字、半角濁点（Lm）は集合内 |
| `_画像-2.v1.png` → 可 | ASCII の有効な途中ドットを維持 |
| `q́.png`、`か゚.png`、`が.png`、`한.png` → 不可 | M 又は合成に参加する字母 |
| `神.png`（U+FA19）、`Å.png` → 不可 | L でも単独 NFC でない |
| `﨑.png`、`髙.png` → 可 | これらには正規分解がない |
| `夕ㅤ.png`、U+115F/U+1160/U+FFA0 を含む名前 → 不可 | L の不可視文字も除外 |
| `.画像.png`、`画像.`、空、`.`、`..`、`a..png` → 不可 | 位置・構造制限 |
| SP/TAB/改行/NBSP/U+3000、Z/C/S/P（ASCII `_-.` を除く） → 不可 | 空白・制御・書式・私用・未割当・絵文字・構文記号を許さない |
| `{}`、引用符、バッククォート、`<>&()`、`%?#/:*|`、`\\`、`／＼`、`・（）〜` → 不可 | HTML/CSS/URL/Windows の特殊文字を入れない |
| 不正 UTF-8、孤立 UTF-16 surrogate → 不可 | 長さ計算前に拒否 |
| `あ`×41＋`.png` → 可、×42 → 不可 | 127／130バイト |
| `a`×124＋`.png` → 可、×125 → 不可 | 128／129バイト |
| 128バイトの部分4つ → 不可 | 区切り込み515バイト |
| `a/b/c/夕.png` → 可、`a/b/c/d/夕.png` → too_deep | 段数の境界 |

拡張子と予約名の比較には **ASCII A〜Z だけを a〜z に写す自前の関数**を Go/JS 両方で使う。strings.ToLower/ToUpper、JS toLowerCase、正規表現の Unicode 大小無視は使わない。`a.PNG` は可、`a.gİf` と `夕.ｐｎｇ` は不可。

予約名は全部分の最初の ASCII `.` より前で比較する。CON/PRN/AUX/NUL、COM1〜9/LPT1〜9、COM¹/²/³・LPT¹/²/³ は reserved_name。`con.夕.png`、`夕/NUL.png` も不可。COM0/COM10/ＣＯＮ は可。幅変換・NFKC はしない。`GAZOU~1.PNG` は集合外で、8.3 alias を素材 ID として扱わない。

## 3. NFC を保証する表と、変えないこと・失うもの

依存は追加しない。サーバーと本文の Go/JS 検査は正規化も NFC 判定も実行しない。管理ページだけが FormData に設定する filename を `file.name.normalize('NFC')` にする（File 自体の name が変わると仮定しない）。サーバーは受信した名前の不許可文字を置換する。本文や直接配置は置換しない。

| 入口 → 期待 | 理由 |
|---|---|
| 管理ページで `か\u3099.png` → 送信 `が.png`、保存も同名 | ブラウザ NFC で通常の NFD を救済 |
| 他クライアントが同じ NFD を送信 → `か_.png` | サーバーは正規化しない |
| 直接配置 NFD と NFC `が.png` が同居 → NFD は bad_name、NFC は可 | 同じ見た目の2行は意図した仕様。改名・統合しない |
| 本文 NFD → template.html_url | 自動修正しない |
| 管理ページで `神.png` → `神.png` | NFC で互換漢字の一部は標準の字へ変わる。﨑・髙は変わらない |
| `Ａ.png` と `A.png` → 別名 | NFC は幅を畳まない |

失うものは M が必要なタイ語・インド系・ヘブライ語の母音記号、合成できないアクセント `q́`、鼻濁音 `か゚`、異体字選択子等。送信前 NFC でも残る M はアップロードで `_`、直接配置・本文では不可となる。通常の日本語・中国語、合成済み韓国語・ベトナム語・西欧語は扱えるが、その言語の全表記を保証する意味ではない。サーバー側 NFD→NFC も失う。

v1 の M の位置規則、M 専用除外表、not_nfc、置換出力列に基づく M 判定は集合から M をなくしたため削除する。sol I-1 の stream-safe 制限も x/text を使わないため不要。M 禁止だけでは足りないという sol の懸念は、非 NFC の L/N と合成後続字母も除外することで解決する。

### 表の生成・保存・再現性（実装時）

生成スクリプトは `scripts/generate-asset-name-ranges.py`、元ファイルは `scripts/unicode/15.0.0/UnicodeData.txt` と `DerivedCoreProperties.txt`、単独非 NFC の正確な判定には同版の `DerivedNormalizationProps.txt` の NFC_QC=N も生成専用の補助入力として保存する。é のような正規分解を持つ NFC 文字を誤って落とさないためである。3ファイルの実バイトの SHA-256 は同ディレクトリの `SHA256SUMS` にコミットする。取得元は `https://www.unicode.org/Public/15.0.0/ucd/` の各同名ファイル。実装時に取得・照合し、ダイジェストを推測で本書へ記入しない。

スクリプトは元ファイルのみを入力にし、First/Last 範囲を展開して L/N と ccc を読む。DCP の Default_Ignorable_Code_Point を引く。DerivedNormalizationProps の NFC_QC=N と L/N の交差を単独非 NFC として除く（正規分解があるだけでは除外しない）。残る L/N が全て ccc=0、単独 NFC であることを検査する。UnicodeData の正規分解（互換タグつきは除く）から合成の後続に現れる ccc=0 の候補も列挙・除外し、アルゴリズムによるハングル合成の V U+1161〜1175 と T U+11A8〜11C2 も除外する。この既知2範囲は21＋27文字だが、集合との交差数と除外総数は15.0データで生成時に確定する。追加候補が出たら黙って無視しない。

残る文字は単独 NFC、ccc=0、かつ前からの合成の後続にならない。ハングル以外の正規分解を持つ合成済み文字については、再帰的な正規分解の末尾が non-starter になる条件を生成時に検証する。後続が ccc=0 ならこの境界を越える合成は起きない。

ハングル音節 U+AC00〜D7A3 は UnicodeData の分解欄ではなく、Unicode のアルゴリズムによる L/V/T 分解・合成として別に扱う。その分解末尾は starter なので上の non-starter の説明を適用しない。許可された音節の後に合成が進む LV＋T は T の除外で、L＋V は V の除外で防ぐ。LVT の後には追加合成がなく、合成済み音節自身も前の文字の合成後続にはならない。この区別と ASCII `_-.` を含む連結境界から NFC を保証する。生成試験には `한.png` の許可、音節の最初・最後（U+AC00/U+D7A3）、LV 音節 `하`＋T U+11AB の拒否、L U+1112＋V U+1161 の拒否、`한한.png` と `한A.png` の許可を入れる。実行環境の Python unicodedata や Go/JS の Unicode 表から許可表を作らない。

出力は `internal/sources/html_rules.json` version **4** の asset.allowedRanges（ソート・結合したコードポイント範囲）、unicodeVersion=15.0.0、lengthUnit=utf8Bytes、maxSegmentLength=128、maxPathLength=512、maxSegments=4、予約名。除外理由別の数・範囲と入力 SHA-256 を `scripts/unicode/15.0.0/asset-name-generation.json` に残す。実装時の再生成は `python3 scripts/generate-asset-name-ranges.py` でネットワークなしに同じ出力となり、差分ゼロを確認する。

Go と JS は同じ完成済み表を使う。二分探索等の範囲検索構造はルール読込時に一度だけ組み立てる。checkURL の呼出しごとに RegExp を作らない。CEF が15.0より古くても合否は表で決まり、正規化実装の版に依存しない。管理ページの送信前 NFC は利用上の救済であり、合否の信頼境界にはしない。

## 4. 衝突・索引・実名完全一致

参照、used_by、配信、削除は綴りと大小を含む完全一致。衝突検出だけは Go の unicode.SimpleFold の軌道の最小コードポイントで各文字を代表させる。共有 JSON や CaseFolding.txt は不要（Fable M-1）。この使用に限って Go unicode を許す。使用した Go と unicode.Version を実装検証で記録し、ツールチェーン更新時に衝突回帰を確認する。Windows 内部の比較の再実装ではなく、最終保存は os.Root と O_EXCL で既存扱いなら409にする。

再構築では、各親ディレクトリの実名集合と衝突グループを保持する。**部分の名前検査（拡張子は問わない）に通った兄弟項目同士**を、ファイル・フォルダ・画像以外・内容不良も含め中身検査の前に比較する。名前自体が不許可の項目はこのグループに入れない。親フォルダが衝突すれば配下全体も name_collision。索引走査順で勝者を決めない。

名前状態・親の衝突状態と内容検査の結果は別々に保持する。usable は全条件の積で導出し、checkContent が成功しても衝突を消さない。Refresh・OpenWithHash・put の全経路でこの導出を使う。理由の優先は構造／深さ／全体長 → 不正文字／部分長 → 予約名 → 衝突 → 拡張子／画像型等の既存内容検査（symlink 等のアクセス禁止は理由にかかわらず常に適用）。

Refresh が未知名を追加する前には、ルートから各親を実名完全一致で確認し、親の ReadDir に末尾の完全一致する実名があることを確かめる。親から保持した兄弟情報も更新し衝突を再計算する。既存行でも Refresh の対象親と祖先のディレクトリ情報を確認し、追加された衝突を反映する。親衝突の変化は既存の配下全行へ反映する。親の兄弟一覧を完全に取得できなければ、その親の衝突判定は「未確定」とする。ReadDir の途中エラー（部分結果が返る場合も含む）、Open の失敗、MaxScanEntries による打切り・未訪問を含み、部分集合から「衝突なし」を決めない。ReadDir の EOF は全件取得できた場合だけ正常完了として扱う。ルートも同じ親として扱う。

未確定の親の配下は、既存の usable 行・今回読めた行も含め全て usable=false にする。旧索引を残すエラー経路でも先に親状態を未確定にし、旧 usable を残さない。未訪問の親の過去の「確認済み」を引き継がない。未確定の診断には既存 unreadable を使い、上限では既存 scan_limit 通知も出す。理由順は上記の構造・名前違反の後、確定衝突／未確定、内容検査の順とし、どの理由でも配信不可。Available・Versions・OpenWithHash も同じ状態を参照する。次回の完全な兄弟走査でのみ未確定を解除し、祖先も確認済み・衝突なしで内容等の全条件を満たせば既存行も復帰する。

全走査を配信ごとに増やさない。再構築が親情報を作り、Refresh が対象親と祖先だけを更新する。OpenWithHash と put は保持した名前状態を使い、未登録名を OS の大小解決だけで足さない。直接配置の変更は既存の定期更新・再構築のタイミングで検出する。確認できた親情報の更新でのみ衝突を解除し、内容の更新では解除しない。索引の行と親情報は idxMu 下で一体更新するが、読み取りから反映までの順序は次の直列化で保証する。

**更新順は世代番号ではなく既存 filesMu による直列化を採用する。** 現行の Save・Delete・Rebuild・EnsureIndex の初期構築は既に filesMu を保持して rebuildLocked を呼ぶ。一方 Refresh と OpenWithHash はロック外で読んだ古い行を put/update できるため、この2経路も親情報・行の取得前からファイル読取り、判定、反映、戻り値の確定まで filesMu を保持する。内容が不変で早期 return する経路も対象とし、保持した古い usable だけで返さない。Save・Delete は実名確認、ディスク変更、再構築まで同じロック区間に置く。保存先の兄弟一覧が不完全なら保存は行わず I/O エラーにする。

ロック順は常に filesMu → idxMu。idxMu を保持したまま I/O や filesMu 待機をしない。OpenWithHash は EnsureIndex をロック取得前に呼んだ後、ロック内で最新の行を取り直すか、初期構築をロック保持前提の内部関数に分ける。既存のロックを取る公開メソッドをロック内から呼んで再入しない。put/update は filesMu 保持中の内部更新専用とし、全行置換で親状態を消さない。これで古い走査を始めた処理が新しい衝突判定の後から反映される順序自体をなくす。世代方式は再構築による索引交換・保存／削除を含む失効判定と再試行を全経路へ足す必要があり、既存の直列化を拡張する方が適合する。I/O 中の待ち時間増加は受け入れる。外部の直接配置は filesMu では止まらず、次の定期検出までの窓は既存どおり残る。

Delete は共通名前検査に加え、索引の実名完全一致を必須にする。使用中判定もそのキーで行う。未知名は root.Remove に渡さず404。登録済みでも削除直前に親実名を確認して OS の別名解決を防ぐ。衝突は配信不可だが、名前として有効で索引に実名がある項目の削除は既存の使用確認を守って扱う。不許可名の削除を新たに有効化せず Explorer での改名・削除を案内する。

| 状態／操作 → 期待 | 理由 |
|---|---|
| A.png があり a.png をアップロード →409 | 大小衝突 |
| Ａ.png と A.png →別名 | 幅を畳まない（OS が既存扱いなら409） |
| A.png/a.png を直接配置、片方の内容を更新して Refresh →両方衝突のまま | 内容検査からの独立 |
| A/x.png と a/y.png →両方衝突 | 親衝突も伝播 |
| NFD と NFC の同居 →NFD は bad_name、NFC は可 | 不許可名を衝突相手にしない意図した仕様 |
| Windows 実名 A.png に参照 a.png →欠損注意、非表示 | Refresh で別名行を作らない |
| Windows で A.png 使用中に DELETE a.png →404、A.png は残る | 索引完全一致の必須試験 |

アップロードの衝突検査は、保存先直下を ReadDir した**全項目**の名前と最終保存名を比較する。索引の usable ファイルだけと比べない。フォルダ、画像でない物、使えない A.png も相手にする。不許可名についても正規化せず取得した名前で同じ folding 比較を行い、同キーなら409。既存フォルダ Logo.png に保存名 logo.png をぶつける等、種別で見逃さない。完全一致も409。O_EXCL は競合と OS 固有の同一視に対する最後の防壁で、上書き・自動連番は行わない。

## 5. 本文と通信

保存本文と組立て HTML は日本語の生パス。本文の percent encoding は引き続き拒否する。属性の文字参照は既存の字句規則で一度だけ戻し、その値を検査する。style 要素の生テキストは文字参照を戻さない。全枝の正規の生パスを refs に残し、文字参照版と生版は同一キー、欠損も refs と warnings に残す。存在は本文の合否に使わない。

| 入力 → 期待 | 理由 |
|---|---|
| src の `/source/files/夕焼け.png`、`/source/files/&#22805;焼け.png` →可、同じ refs | 生キー |
| src の `/source/files/%E5%A4%95.png`、`%252f` →html_url | 本文は復号しない |
| CSS url の日本語（無引用・二重引用・単一引用）→可 | 構文文字は集合外 |
| style 属性の `url(&quot;/source/files/夕焼け.png&quot;)` →可 | 属性参照を一度戻す |
| style 要素の `url(/source/files/&#22805;焼け.png)` →html_url | 生テキスト |
| 保存本文の `?v=0123456789ab` →不可、rendered の同 query →可 | 生成だけが付ける12桁小文字hex。重複は不可 |

hash は既存 SHA-256 先頭12桁。トークン位置で v を追記し全置換しない。参照抽出・書換えで生文字と元の文字参照を扱い、日本語を percent encoding に変えない。DOM 再検査は解決済み img.src でなく現在どおり attr.value を使う。

HTTP はブラウザの UTF-8 percent encoding を許し、Go の r.URL.Path を**再復号せず**共通検査と索引完全一致へ渡す。clean・redirect・basename で救済しない。削除要求の segment ごとの encodeURIComponent は維持する。raw path の encoded separator を別に拒否する追加実装は必須にしない（Fable M-6）。

| HTTP 入力 → 期待 | 理由 |
|---|---|
| 日本語の正しい percent encoding →有効な索引実名なら GET 200／DELETE は既存使用確認 | Path は一度復号済み |
| `%252F`、`%5C`、不正 UTF-8、dot segment、空部分、8.3 alias →拒否、別名救済なし | 復号後の共通検査・実名一致 |
| `%2F` →復号後の通常の階層として検査 | `夕%2F焼け.png` は実在する `夕/焼け.png` の完全一致キーなら可。違うファイルへ解決しない |

sol の GET/DELETE 両方で encoded slash を常に拒否する期待は採らない。安全条件は raw 綴りの一意性ではなく、復号後の有効な実名キーへの完全一致である。

## 6. 取り込み

multipart の決まりは次の4つとする。独自のパラメーター字句解析は加えない。

1. `internal/sources/files.go` の ReadUpload を主に変更する。ParseMultipartForm/FormFile を経由せず MultipartReader を使い、Content-Disposition を mime.ParseMediaType に渡す。呼出しと HTTP エラー応答は internal/web/source_files.go が担当する。
2. basename 前の `params["filename"]` を使う。Part.FileName や filepath.Base で元名を消さない。
3. パーサーエラー、空、不正 UTF-8、`/`、`\\`、`／`、`＼`、`..` を含む名前は400。壊れたヘッダーも置換前に拒否する。
4. filename*、通常名との併記、重複、連結、不正 charset/escape は、標準パーサーが返した結果をそのまま同じ名前検査に通す。パーサーエラーなら400、成功なら結果の値で決める。独自 fallback や二度目の percent decode はしない。

### part の選択・上限・応答

今の FormFile("file") と同じく、Content-Disposition が form-data、name が大小を含め `file`、パーサーの filename が非空の**最初のファイル part 1個**を採用する。通常値の `file`（filename なし／空）は候補にならない。対象がなければ今どおり400 `source_files.invalid`。後続の同名ファイル、別フィールドのファイル、通常値は今どおり保存対象にせず読み捨てる。一律に複数 part を拒否しない。採用対象の不正名を後続候補で救済しない。第6節の元名検査は採用対象に適用する。

採用 part の取得だけで成功を返さず、終端まで全 part を読み、構文エラー・途中切断は今どおり400 `source_files.invalid`。余分な part も全体上限に数え、捨てるときの読取り／Close エラーを無視しない。最後まで正常なことを確かめてから Save を呼ぶ。本文全体（ヘッダー・境界・通常値・余分なファイルを含む）は今どおり MaxBytesReader の **MaxFileSize＋1MiB＝6MiB**、採用ファイルは **MaxFileSize＝5MiB** とする。全体上限を見落とさないよう multipart 終端後の残りの body も上限内で読み切る。ファイルは最大 MaxFileSize＋1 バイトまで検査用に読み、余分な part はバッファへ全量保存しない。

全体超過は MaxBytesError を保持し、今どおり413 `source_files.limit`。採用ファイルだけが5MiBを超えて全体6MiB以下なら、現行 ReadUpload の汎用 "too large" と同じ **400 `source_files.invalid`** を維持する（ErrFileTooLarge に変えて413にしない）。余分なファイルに個別5MiB制限は適用せず、全体制限で止める。ParseMultipartForm の既定上限に相当する総 part 数1000とファイル part の累積 MIME ヘッダー数10000も明示して保ち、超過は400とする。NextPart 自体のヘッダー制限も維持する。GODEBUG による旧パーサー上限の上書きは新契約には含めず、この既定値を固定する。

Save の件数・保存総量・ファイル容量超過は今どおり413 `source_files.limit`、重複は409 `source_files.duplicate`、型不正等は400 `source_files.invalid`、成功は201（ok/name/size）を維持する。予約名と長名は400のまま、第6・8節の修正可能な専用説明を返す。

受信名の UTF-8・パス混入検査 → 最後の ASCII ドットで拡張子確認と ASCII 小文字化 → stem の不許可コードポイント1つにつき `_` 1つ → 完成形規則を再検査 → 直下全項目との衝突検査 → 排他的保存、の順。サーバー NFC はない。連続 `_` は圧縮しない。先頭ドットは `_`、有効な途中ドットは維持。長名・予約名を切詰めや接頭辞で救わない。

| 入力 → 期待 | 理由 |
|---|---|
| `夕 焼け（夜）.PNG` → `夕_焼け_夜_.png` | 記号だけ置換 |
| `夕焼け.v1.png` →そのまま、`.夕焼け.png` → `_夕焼け.png` | 途中ドットを維持 |
| CON.png、長すぎる最終名 →400 | 専用の説明を返す |
| a/b.png、a\\b.png、a／b.png、a＼b.png、a..png →400 | basename で通さない |
| `%E5.png` → `_E5.png` | filename の percent encoding は復号しない |
| 既存 `夕_焼け.png` に `夕 焼け.png` →409 | 置換後の名前で衝突 |
| 既存 `が.png` に管理ページ経由 NFD →409、他クライアント NFD → `か_.png` | 送信前 NFC の有無を区別 |
| `filename="a.png"; filename="a.png"` →パーサーが a.png を返すなら通常処理 | 重複自体を一律400にしない |
| 不正 filename*＋通常名、filename*0* 連結 →標準パーサーの結果で決定 | sol の一律拒否案を縮小 |

直接配置は改名・置換・正規化しない。文字違反／部分長超過は bad_name、全体長は path_too_long、予約名は reserved_name、新しい衝突理由は name_collision。not_nfc は追加しない。

## 7. 共通入力表と試験の設計

Go は rune と len、JS はコードポイント走査と TextEncoder で UTF-8 長を計算する。JS は孤立 surrogate を TextEncoder の置換前に拒否する。エラー位置は元入力の1始まりコードポイントで、バイト長とは別。

既存 `internal/sources/html_cases.json` H001〜H359 を全行調査したところ、長さ変更で期待が変わる行は **ない**。128/512の URL 境界行自体がない。長い H293/H294 は HTML の深さ境界で無関係。従って「既存境界行を変更」とはせず、相対499〜512バイトが旧 Go/JS では接頭辞込みで落ちる不整合を新しい行で捕まえる。H357〜H359 の生成 query 受理も維持する。

### H360〜H395（v1 の行を維持）

S/R は save/rendered。表中の Unicode escape は実文字へ展開する。ok の refs は save で比較し、rendered の refs 比較はハーネスの契約に合わせる。エラーは下表の位置を両言語で検証し、H298以降の現行ハーネスに合わせ rendered 側にも必要な位置を持たせる。存在 fixture に夕焼け.png・が.pngを足し、欠損行以外の warnings は空にする。
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


### H396 以降の追加行（入力→期待と理由）

各独立入力を S/R の組で採番する。以下の URL 部分だけの行は `<img src="/source/files/…">` に入れる。`html_url` は `template.html_url` の略で、既定の位置は1・11。複数例を挙げた欄はそれぞれ別入力にする。ok は復号後の同じ生パスを refs に持つ。位置が異なる CSS 入力は字句の URL 値の開始位置を現行契約で数える。

| 入力 | 期待 | 理由 |
|---|---|---|
| U+2EBF0＋`.png`、`Ᲊ.png`（U+1C89） | html_url | 15.0の表外 |
| `夕ㅤ.png`、`ᅟ.png`、U+1160/U+FFA0 を含む名 | html_url | 不可視 L |
| `𠮷.png` | ok | 補助平面 |
| 全体 `<img src="/source/files/𠮷.png"><img src="x">` | html_url、1・42 | UTF-16／バイトで位置を数えない |
| `あ`×41／×42＋`.png` | ok／html_url | 127／130バイト |
| `a`×127 `/` `b`×127 `/` `c`×127 `/` `d`×124＋`.png` | ok | 相対512バイト、全部分128以下 |
| 上の d を×125に | html_url | 相対513、最後の部分129。全体境界の試験は次行でも独立に行う |
| `a`×128 `/` `b`×128 `/` `c`×128 `/` `d`×122＋`.png` | html_url | 相対513、全部分128以下で全体だけ超過 |
| 相対499／500バイト（上と同様に各部分128以下） | ok | 旧接頭辞込み上限の境界 |
| 相対512の URL＋生成 query | S html_url、R ok | query を長さに含めない |
| `a.gİf`、`夕．png`、`夕.ｐｎｇ` | html_url | ASCII 拡張子だけ |
| `神.png`、`Å.png`、`한.png`、U+1100 U+1161＋`.png` | html_url | 単独非 NFC とハングル合成後続を除外 |
| `한.png`、`ｶﾞ.png`、`﨑.png`、`髙.png` | ok | NFC の許可文字 |
| `q́.png`、`q̣́.png`、`q̣́.png`、`́a.png`、`か゚.png`、`夕឴.png` | 全て html_url | M は位置・順序によらず不可 |
| q＋U+0301×30／×31＋`.png` | 両方 html_url | stream-safe 境界ではなく M 禁止 |
| `か&#x3099;.png` | html_url | 属性復号後も M 禁止 |
| `夕&#x3000;.png`、`夕&#37;E5.png`、`a&#46;&#46;b.png` | html_url | 復号後の空白・percent・連続ドット |
| `ＣＯＮ.png`、`COM0.png` | ok | 幅を畳まない |
| `con.夕.png`、`夕/NUL.png`、`LPT².png` | html_url | 全部分で予約名 |
| 全体 `<style>.x{background:url(/source/files/夕　焼け.png)}</style>` | html_url | CSS では空白扱いされない U+3000 も名前検査で拒否 |
| 全体 `<style>.x{background:url(/source/files/夕焼け.png?v=0123456789ab)}</style>` | S html_url、R ok | CSS query |
| `夕&#47;焼け.png` | ok、refs `/source/files/夕/焼け.png` | 参照から生じた区切りも通常の階層規則 |
| `夕&amp;#22805;.png` | html_url | 二度目の文字参照復号なし |
| 全体 `<style>.x{background:url('/source/files/夕焼け.png')}</style>` | ok、同じ refs | 単一引用符も検証 |
| 全体 `<style>.x{background:url(/source/files/&#22805;焼け.png)}</style>` | html_url | style は参照復号なし |
| 全体 `<img srcset="/source/files/夕焼け.png 1x">` | template.html_attribute | srcset は許可しない |
| `夕焼け.png?v=0123456789ab&v=0123456789ab` | S/R とも html_url | 重複 query 禁止 |
| 全体 `😀<img src="/source/files/夕 焼け.png">` | html_url、1・12 | 補助平面後の位置 |

### 名前・統合・実機試験（実装時）

名前検査専用の共有 fixture は `internal/sources/asset_name_cases.json` に置き、Go と Node が同じ範囲・長さ・位置・予約名を検証する。不正 UTF-8 は JSON に書けず、孤立 surrogate は Go JSON 復号が U+FFFD に変えるため共有 fixture に入れない。Go は生バイト入力、JS は孤立 surrogate の文字列を各言語の個別試験にする。

| 試験 | 入力 → 期待 |
|---|---|
| 生成・名前 | ハングルのアルゴリズム分解・合成を別検証し、第3節の `한.png`・LV/T・L/V・音節端点と連結境界を確認。L/N 全下位分類、M、不可視、表外、128/129・512/513、予約名、ASCII folding →一致。再生成差分ゼロ |
| multipart | 日本語 raw UTF-8、filename*／併記／重複／連結／不正拡張値 →標準パーサー結果を完成形検査へ。パス混入は basename 前に400 |
| multipart 選択 | 通常値 file＋ファイル file、複数 file、別名ファイル →最初の対象だけ保存。空 filename のみ／対象なし→400。不正な先頭対象＋正常な後続→400。後続不正構文・途中切断→400、保存なし |
| multipart 上限・応答 | 採用ファイル5MiBちょうど／＋1（全体6MiB以下）→201／400。全体6MiBちょうど／＋1→他条件正常なら201／413。余分な part・終端後の body での全体超過も413、保存なし。1000／1001 part・累積ヘッダー10000／10001→境界内可／400。保存上限413・重複409・型不正400と JSON code も検証 |
| 親の未確定 | 既存 usable を作り、未読側に衝突相手を置いて MaxScanEntries 打切り→既存を含む配下全行不可。部分結果つき ReadDir 失敗・ルート Open 失敗・未訪問の親も不可。Available/Versions から除外、GET は404。次の完全走査で衝突なしなら復帰、衝突ありなら name_collision のまま |
| 更新の交錯 | barrier で Refresh の親読取り後を停止し、衝突を検出する Rebuild を開始→filesMu 待ちになり、Refresh 完了後の Rebuild が最終状態を決める。逆順でも古い Refresh が衝突を解除しない。OpenWithHash の不変／変更経路・put/update、Save、Delete と再構築も両順で交錯させ、後から古い行が復活しない・配下不可・デッドロックなしを検証（sleep に依存しない） |
| 保存・索引 | 日本語保存→一覧→読出し→削除、管理ページ NFC と他クライアント NFD の違い、置換後衝突、直下フォルダ／内容不良との衝突 →規定どおり |
| sol 統合1 | A.png/a.png の片方更新→定期 Refresh→OpenWithHash・put →両方衝突のまま |
| sol 統合2 | A/x.png と a/y.png →親衝突で両方不可。追加・削除の親情報更新でも整合 |
| sol 統合3（期待修正） | 同値の filename 重複、不正 filename*＋通常名、連結 →一律400にはせず標準パーサーの結果を検査する |
| sol 統合4（期待修正） | GET/DELETE の %2F/%5C/%252F、不正 UTF-8、dot segment →第5節どおり。%2F は復号後の完全一致キーなら許可、別名へ救済なし |
| S2-85 | 生文字・文字参照・不実行枝 →同じ used_by、hash、削除確認409、欠損注意。同名更新で hash 更新 |
| HTML 書換え | img・style 属性・style 要素、引用の各形 →生パスを保ち v を位置で追記、木一致、外部要求ゼロ |
| 補完・差し込み | 日本語入力中も候補維持。両差し込み箇所から保存→再表示 →生パス |
| Windows 実機1 | A.png 配置、本文 a.png →欠損注意・非表示。Refresh 後も a.png 行を作らない |
| Windows 実機2 | 使用中 A.png に DELETE a.png →404、実ファイルが残る。全角大小と8.3 alias も確認 |
| OBS/CEF 実機 | 日本語ドロップ・直接配置・CSS 各引用・文字参照・同名更新・削除 →正常表示。CEF の要求が通信時だけ符号化されることを観測 |

Linux の試験だけで Windows 実名保証を合格にしない。Fable の Chromium 実測を OBS/CEF 実機の代用にしない。本作業では試験コードや結果記録は変更しない。

## 8. 画面・文言・残る危険

| 場所／入力 → 期待 | 理由 |
|---|---|
| 素材一覧・補完・差し込み →実名の生文字 | 符号化表示・ローマ字化をしない |
| アップロードで名前が長すぎる →専用「名前が長すぎます。拡張子を含め128バイト以内にしてください」 | 汎用 source_files.invalid だけでは直せない。日本語41字＋.pngの目安も案内 |
| bad_name →「名前に使えない文字があります。管理ページから入れ直すか、名前を変更してください」 | NFC 用語だけで説明しない。M は入れ直しても `_` になるとヘルプに明記 |
| name_collision →「同じ名前とみなされる項目があります。ファイルまたは親フォルダの名前を変更してください」 | 親衝突も説明 |
| 削除確認 →実名・使用中ソース名を既存の確認文へ | S2-85 を維持 |

新 Reason name_collision を Reasons に加え、全対応言語の `source_files.reason_name_collision` を追加する。長名のアップロード専用キー `source_files.name_too_long` も各言語へ追加。既存 path_too_long／bad_name も単位と案内を揃える。Reason 全コードに文がある前提を保ち、not_nfc 用のキーは増やさない。

実名表示は textContent・テキストノード・エディター文字列を使い、innerHTML に渡さない。RTL のヘブライ・アラビア文字は許可されるので表示順が入れ替わりうる。名前表示は bdi 等で周囲と分離し、双方向制御文字そのものは禁止を維持する。ラテン／キリルなど見た目の混同検出は対象外。記号に似た Lm（ʼ U+02BC、ː U+02D0）は構文上安全なので許す。NFD/NFC 同居は勝手に統合しない。

Windows の OS 比較と SimpleFold の差、絶対パス長や保存先制限による I/O 失敗、直接配置と定期検出の間の競合は残る。os.Root・実名確認・O_EXCL と既存のファイル同一性検査を維持し、失敗を黙った別名アクセスで救済しない。

## 9. 実装の順と引継ぎの切れ目

1. 名前の検査：生成元・SHA-256・スクリプト、固定範囲表、Go/JS 共通規則、共有 fixture と言語別不正入力試験。
2. checkURL：html.go と html.js の相対 UTF-8 長、version 4、H360以降、抽出・書換えの一致。
3. 索引：衝突状態と内容の分離、親情報、Refresh/OpenWithHash/put、Delete の実名完全一致。
4. 取り込みと画面：MultipartReader、ブラウザ送信前 NFC、衝突、両差し込み・補完・i18n。
5. spec と実機：仕様へ契約を反映し、Windows 2項目・OBS/CEF を確認する。

3回に分けるなら、**第1〜2段階**（表と本文の契約が Go/Node で一致）、**第3段階**（更新後も衝突と完全一致が保たれる）、**第4〜5段階**（取り込みから実機表示・削除まで）を切れ目にする。2回なら前半を第1〜3段階、後半を第4〜5段階とする。中間の段階を製品完成とはせず、各回の入力表・境界条件を次の実装役へ渡す。コード・試験・spec はこの設計改訂では書かない。

## 10. spec の直し方の案（本作業では変更しない）

| 箇所 | 現在 → v2 の改訂案 | 理由 |
|---|---|---|
| 第14章 素材の ASCII 正規表現・128文字／512文字 | 15.0 固定 L/N の限定表、M・不可視・非 NFC／合成後続を除外、部分128・相対512 UTF-8バイト、親3段 | 必ず NFC になる集合と単位を明確化 |
| 同章 取り込みの正規化 | 管理ページのみ送信前 NFC、サーバーは正規化せず不許可文字を `_`、ASCII 拡張子、予約名400、直下全項目との衝突409、O_EXCL | クライアント別 NFD の差と非上書きを明記 |
| 同章 multipart | 第6節の4規則、標準パーサー結果を検査、basename を使わない | 独自の重複／拡張形式拒否を要求しない |
| 同章 直接配置・索引・削除 | 改名なし、bad_name と name_collision、親衝突、内容更新で解除しない、Refresh／Delete は実名完全一致 | 別名の追加・誤削除を防ぐ。not_nfc は作らない |
| 第9章 固定素材 URL と S2-77 第4節相当 | 本文は生文字、属性の文字参照は1回、本文の percent decode／% 禁止維持、通信は復号済み Path の完全一致 | raw separator の追加拒否は必須にしない |
| S2-85 使用判定・v | 生パスの綴り・大小の完全一致、文字参照だけ同一視、相対長に接頭辞・v を含めない | 衝突判定と参照一致を混ぜない |
| 素材 UI・補完・差し込み | 実名の生文字、日本語入力で補完継続、送信前 NFC と失う表記、長名・衝突の案内、通信のみ符号化 | 操作と保存名の関係を説明 |

S2-77 の許可要素・属性・CSS 関数・字句・テンプレート・全枝・DOM 再検査の安全規則は改訂対象外。依存追加、M の位置規則、stream-safe、not_nfc、共有 folding 表を仕様に残さない。
