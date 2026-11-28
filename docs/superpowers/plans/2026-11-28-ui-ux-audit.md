# 管理ページの UI/UX と文言の監査（2026-11-28。Fable）

- 状態: 直す所の一覧（まだ直していない。2026-11-28 のユーザーの指示「修正点を上げて、本体にはまだ変更を加えない」）
- 照らしたもの: spec v0.36 の5章・14章、実機の結果の指摘、ui-ux-pro-max の references/quick-reference.md（決まりの id）、baseline-ui のうち合う所
- 写しは scratchpad の ui-audit/ にあり、リポジトリには入れていない（注意の文にフォルダの場所が写るため）
- 追記（指揮者）: C-1 の「ログインが必要です」は、指揮者が 4c5fd05 で英語の unauthorized を訳したときの文で、パスワードが違うときにも出る

## 見た範囲と、見られなかった所

- **見た組み合わせ**
  - PC（1280×900）のライト: ログイン、状態、カメラ、設定の4区分、ソース（一覧と編集）、確かめの窓、スマホで開くの窓
  - PC のダーク: 状態だけ
  - スマホ（390×844）のダーク: 状態の3つのタブ、メニュー、カメラ、設定、ソース（一覧と編集）
  - スマホのライト: 英語の設定とソースの一覧だけ
  - 英語: 状態・カメラ（PC ダーク）、設定・ソース（スマホ ライト）
- **見ていない組み合わせ**: ログイン・確かめの窓・スマホで開くの窓の、ダークと 390px。PC ダークのカメラ・設定・ソース。
- **開けなかった窓**: 「管理パスワードを作り直す」「読み取りパスワードを作り直す」の確かめの窓。開く操作が自動の許可の仕組みに断られた。中身はコード（`app/settings-state.js:44-58`）から読んだ。
- **押さなかったもの**: スマホで開くの「作り直す」（QR を作る）、状態の「配信を始める / 止める」「待機」「すべて表示に戻す」、素材の取り込み、カメラの「送り先」を開くこと、目のアイコン。
- **確かめられなかった状態**: OBS がつながっているときのシーンの一覧と選ぶ欄、配信中の赤い注意、待機中の帯、「パスワードを決める」の画面、再起動中の表示。
- **試しにしたこと**
  - ソース「監査の試し」（名前 audit-tmp）を作って消した。消したあと、一覧が「生きた確かめ」だけなのを見た。
  - カメラ2の「削除」を下書きにして確かめの窓を読み、キャンセルして「読み直す」で戻した（2台のまま）。
  - 「本体を再起動する」の窓を開いて読み、キャンセルした。
  - ログインの誤りを見るために、違うパスワードを1回だけ入れた。
- **片づけ**: 言語の覚え（localStorage の `language`）を消し、下のタブの覚えを「回線」に戻した。テーマは「自動」のまま。emulate を戻し、ページを閉じた。
- **「？.」について**: 確かめの文の終わりの「.」は chromium-devtools の表示が足すもので、実物の文にはない（s2-54 の8の1つ目は誤りではない）。

**写しの置き場**: `/tmp/claude-1000/-home-teru358-project-irl-live-keeper/9ba12fa0-d1e3-40a3-86be-c9d0f6c7da54/scratchpad/ui-audit/`（下の「写し」はこの中のファイル名）。
- 状態とカメラの写しには、注意の文の中のフォルダの場所（ホームの下の試験用のフォルダ）が写っている。人に渡すなら隠す。
- LAN の番地は置き換えてから撮った（`04` `06` `09` `12` `17` `23` `28` は目で見た。`16` は縦に長く、目では見ていないが、同じ画面の `17` で置き換えを見た）。

**場所の略**
- `app/` は `/home/teru358/project/irl-live-keeper/internal/web/assets/app/`
- `style.css` は `/home/teru358/project/irl-live-keeper/internal/web/assets/style.css`
- 文の鍵は `/home/teru358/project/irl-live-keeper/internal/i18n/locales/ja.json`（英語は `en.json`）
- JS は1行が長いので、行と関数名か部品名で示す。

---

## A: 使いやすさ・読み上げ・押しやすさ

### A-1 スマホのメニューに今の画面の印が出ない（高・小）
- 場所: `style.css:177`（`.sheet-nav a[aria-current="page"]`）、`app/shell.js:20`
- 写し: `15-menu-sheet-mobile-dark.png`（「状態」を開いているのに、4行とも同じ見た目）
- よくない点: メニューの行は `button` なのに、印の CSS は `a` にだけ当たっている。`nav-state-active`。spec 14章「今の画面に印」と食い違う。
- 直し方: `.sheet-nav button[aria-current="page"]` に太字と左の線を付ける。

### A-2 スマホのメニューが dialog として働かない（高・中）
- 場所: `app/shell.js:17`（メニューのボタン）、`:20`（`#sheet`）
- 写し: `15-menu-sheet-mobile-dark.png`
- 実測: `role="dialog"` はあるが、名前も `aria-modal` もない。開いてもフォーカスは「メニュー」のまま。Escape で閉じない。後ろの `main` は Tab と読み上げの対象のまま。ボタンの字は開いても「メニュー」。
- 決まり: `dialog-focus`、`modal-escape`、`escape-routes`。前のレビューで回したものと同じ。
- 直し方: `<dialog>` の `showModal()` に替える（確かめの窓と同じ仕組みで、Escape・フォーカス・後ろを触れなくする動きがそろう）。`aria-label` は「メニュー」。開いている間はボタンを「閉じる」にする。

### A-3 確かめの窓に名前がなく、最初のフォーカスが「反映する」に乗る（高・小）
- 場所: `app/settings.js:50`、`app/shell.js:23,24`（3つの `<dialog>`）、`app/phone.js:11`
- 写し: `05-cameras-confirm-delete-pc-light.png`、`06-phone-dialog-pc-light.png`
- 実測
  - 3つの dialog とも `aria-labelledby` も `aria-label` もない。
  - カメラの削除（再起動して反映）の窓でも、開いた直後のフォーカスは「反映する」。Enter だけで確定する。
  - キャンセルのあと、フォーカスが `body` に落ちた（1回見た。押した時に「保存」が `disabled` になるためと見られる）。
  - スマホで開くの窓は、Escape で閉じたあと「スマホで開く」に戻った。
- 決まり: `dialog-focus`、`confirmation-dialogs`、`destructive-emphasis`。
- 直し方: 見出しに id を付けて `aria-labelledby`。削除・再起動・作り直しを含むときは「キャンセル」に `autofocus`。閉じたら元のボタンへ `focus()`。

### A-4 カメラの削除の窓が、全カメラの受信が切れることを言わない（高・小）
- 場所: `app/settings-state.js:44-58`（`confirmationLines`）。49行目は配信中だけ `settings_stream_warning` を足す。
- 写し: `05-cameras-confirm-delete-pc-light.png`（出るのは「再起動して反映」と「カメラ 2（サブカメラ） — 削除」の2行だけ）
- よくない点: spec 5章「追加・削除は再起動して反映し、全カメラの受信が一度切れることを確かめの窓で示す」と食い違う。配信していないときは、何が起きるかの文がない。
- 直し方: カメラの追加・削除を含むときは、配信中でなくても「全カメラの受信が一度切れます」を足す（近い文は `publish_reset_warning` にある）。

### A-5 ブラウザの `confirm()` と、画面の確かめの窓が混ざっている（中・中）
- 場所: `app/main.js:20`（配信の停止、すべて表示に戻す）、`app/settings.js:45`（LAN から開く）、`:49`（読み直す）、`app/sources.js:74,77,84,91`（破棄、ソースと素材の削除）
- 写し: なし（ブラウザの窓は写らない。文は道具の表示で読んだ）
- よくない点: 同じ「確かめる」が2つの見た目になる。ブラウザの窓はボタンが「OK / キャンセル」で、ダークにもならない。LAN の注意は長い文が1つの段になる。baseline-ui「元に戻せない操作は確かめの窓」、`confirmation-dialogs`。
- 直し方: 確かめの窓を1つの部品にして、文・確定のボタンの名前・危険かどうかを渡す形にそろえる。

### A-6 ログインの誤りが、入力欄から離れた左上に出る（中・小）
- 場所: `app/shell.js:19`（`#error`）、`style.css:194`
- 写し: `02-login-wrong-password.png`
- よくない点: 誤りの文がヘッダーのすぐ下の左端に出て、まん中のカードから離れる。入力欄と結び付いていない。`error-placement`。
- 直し方: `#login` の中、ボタンの上に出す。入力欄に `aria-describedby` と `aria-invalid`。文言は C-1。

### A-7 操作の誤りと知らせが、押した所から遠い（中・中）
- 場所: `app/shell.js:19`（`#app-error`）、`app/settings.js:49`（`#cameras-result` `#settings-result`）、`app/sources.js:156`
- 写し: `13-source-editor-error-free-pc-light.png`（ほかはコードと実測）
- 実測
  - 状態の操作の誤りは、ページのいちばん上の左（`#app-error`。色も枠もない素の文。コードから）。
  - カメラの「読み直しました。」は、長いページのいちばん下（PC で高さ 4290px の下端）。
  - ソースの編集の誤りは、ヘッダーとテストの帯の間で、エディターから離れる。
- 決まり: `error-placement`、`success-feedback`、baseline-ui「誤りは操作の隣に」。
- 直し方: 状態の誤りは「注意」の欄か押したボタンの下。設定とカメラは保存の行の上。エディターの誤りは「HTML 本文」の見出しのそば。

### A-8 カメラの保存のボタンが、長いページの底にしかない（中・中）
- 場所: `app/settings.js:49`（`.row`）
- 写し: `04-cameras-pc-light.png`（PC で約3600px、スマホで約3800px）
- よくない点: 下書きを作っても、今の画面に「未保存」が出ない（印はメニューの中だけで、スマホではメニューを開かないと見えない）。`submit-feedback`、`content-priority`。
- 直し方: 下書きがある間だけ、画面の下に「保存していない変更があります　保存　読み直す」の帯を貼り付ける（`position: sticky`）。

### A-9 カメラの「削除」が、押した瞬間にカードごと消える（中・小）
- 場所: `app/settings.js:28`（`remove`）、`:35`
- 写し: なし（実測。押すとカードが消え、メニューが「カメラ未保存」になる）
- よくない点: 「消す予定」の表示がない。戻すには「読み直す」でほかの下書きごと捨てるしかない。読み上げの名前はどのカードも「削除」。色もほかと同じ青。`undo-support`、`destructive-emphasis`、`voiceover-sr`。
- 直し方: カードを残して「保存すると削除します」と出し、「取り消す」を置く。ボタンは赤（`.btn.stop`）、`aria-label` は「カメラ 2（サブカメラ）を削除」。

### A-10 details の見出しが 24px（中・小）
- 場所: `style.css:201,213,271`、`app/settings.js:38`、`app/main.js:36`、`app/sources.js:147,159,160`
- 写し: `18-cameras-card-mobile-dark.png`、`24-source-editor-mobile-dark.png`
- 実測: 「切替」「送り先」「値の一覧」「素材」「OBS への取り込み」の押せる高さが 24px。
- 決まり: `touch-target-size`。前のレビューで回したもの。
- 直し方: `summary{min-height:44px;display:flex;align-items:center}`。

### A-11 切り替えのボタンとスイッチが小さい（中・小）
- 場所: `style.css:55`（`.seg button`）、`:242`（テストの帯）、`:210`（スイッチ）、`:50`（メニュー）
- 写し: `09-sources-list-pc-light.png`、`23-sources-list-mobile-dark.png`
- 実測
  - ソースのテストの3つの軸: 高さ 36px、字 12px（「OK」は幅 33px）。
  - 「表示」「分割の向き」: 高さ 36px。
  - スイッチ: 48×28。
  - スマホの「メニュー」: 90×40。目のアイコン: 40×43。
- 決まり: `touch-target-size`、`readable-font-size`。
- 直し方: `.seg button` を 44px・14px に。スイッチは高さ 44px の押せる所を持たせる。

### A-12 押せないスイッチが、押せるものと同じに見える（中・小）
- 場所: `style.css:210`、`app/sources.js:139`（「OBS にも反映」）
- 写し: `09-sources-list-pc-light.png`
- 実測: 3つの軸が「実際」の間は `disabled` だが、`opacity` は 1。理由の文もない。
- 決まり: `disabled-states`。
- 直し方: `input[role=switch]:disabled{opacity:.5;cursor:not-allowed}`。下に「テストの状況を選ぶと、OBS にも映せます」。

### A-13 ページを読み直すと、いつも「状態」に戻る（中・中）
- 場所: `app/shell.js:8,11`（`page:'status'`、`navigate`）
- 写し: なし
- 実測: カメラやソースを開いていて読み直すと「状態」に戻る（言語の切り替えも読み直すので同じ）。URL はどのページでも `/`。
- コードから: 履歴に何も積まないので、ブラウザの「戻る」ではページを戻れない（押して確かめてはいない）。
- 決まり: `deep-linking`、`back-behavior`、`state-preservation`。
- 直し方: `location.hash`（`#cameras` など）に今のページを書き、開く時と `hashchange` で読む。

### A-14 ページを移ったあと、フォーカスが動かない（低・小）
- 場所: `app/shell.js:11`、`app/sources.js:75`（`open`）
- 写し: なし（実測）
- 実測: 「新しく作る」を押すと、押したボタンが消えてフォーカスが `body` に落ちる。本文へ飛ぶリンクもない。
- 決まり: `focus-on-route-change`、`skip-links`。
- 直し方: 移った先の `h2` に `tabindex="-1"` を付けて `focus()`。

### A-15 送り先へ飛ぶと、カードの見出しがヘッダーの下に隠れる（低・小）
- 場所: `app/main.js:25,37`（`scrollIntoView({block:'start'})`）
- 写し: `18-cameras-card-mobile-dark.png`（同じ呼び方で動かした。「カメラ 2」の見出しが貼り付いたヘッダーの裏）
- 決まり: `fixed-element-offset`、`focus-not-obscured`。
- 直し方: `.settings-card{scroll-margin-top:72px}`。

### A-16 数の欄が `role="textbox"`（低・小）
- 場所: `app/settings.js:46`（`role=${bool?'switch':'textbox'}`）
- 写し: なし（コードと a11y の木から）
- よくない点: `type=number` の読み上げの役割を消している（s2-33b の17 の `valuemax="0"` を避けた跡）。「保存日数」は `min` だけで `max` がない。
- 直し方: `role` は switch のときだけ付ける。数の欄には `min` と `max` をそろえて書く。

### A-17 空の id が3つある（低・小）
- 場所: `app/main.js:26`（`id=${...||''}`）
- 写し: なし（実測）
- 実測: 「すべて表示に戻す」「配信を始める」「配信を止める」が `id=""`。
- 直し方: lit-html の `nothing` を返す。

### A-18 入力の細かい所（低・小）
- 写し: なし（コードから）
- `app/phone.js:11`: 「直接入力」の欄に `autocomplete` も `inputmode="url"` もない。`input-type-keyboard`。
- `app/sources.js:128`: ソースの「名前」に、使える文字の案内がない（C-2）。`input-helper-text`。

---

## B: 見た目（余白・階層・色・ダーク）

### B-1 ダークのとき、ソースのプレビューの地が白くなる（高・小）
- 場所: `app/sources.js:28-29`（iframe）、枠の文書を返す `/home/teru358/project/irl-live-keeper/internal/web/sources.go:20,36`、`style.css:268`
- 写し: `23-sources-list-mobile-dark.png`、`24-source-editor-mobile-dark.png`（ライトは `09-sources-list-pc-light.png` で市松が見える）
- よくない点: ダークでは iframe が白く塗られ、透けを示す市松が隠れる。白い字のソース（OBS に重ねる字で多い）は見えない。`color-dark-mode`。
- 原因の見立て（確かめていない）: 親と iframe の `color-scheme` が違うと、ブラウザが iframe を不透明にする。`internal/web/*.go` と `source.js` に `color-scheme` の指定はなかった。
- 直し方: 枠の文書に `<meta name="color-scheme" content="light dark">` を入れるか、iframe に `style="color-scheme:light dark"`。直したあと、白い字と黒い字の両方で見る。

### B-2 色の差が足りない所（中・小）
- 写し: `03-status-pc-light.png`、`14-status-line-mobile-dark.png`、`25-status-pc-dark.png`
- 比は CSS の値からの計算（字は 4.5 以上が要る）

| 所 | 場所 | ライト | ダーク |
|---|---|---|---|
| 帯の目盛りの数字（800 / 1500、11px） | `style.css:81-82`。`.tick` の `opacity:.55` が数字にもかかる | 2.2 | 2.9 |
| 「配信中」の札（白い字と `--tally`） | `style.css:41-46` | 5.0 | 3.2 |
| LOW の字（`--low`、14px の太字） | `style.css:3` | 地 4.1、面 4.4 | 9.5 |
| 「カメラなし」の字（`--k-none`、12px。タイルは見ていない） | `style.css:148` | 面 4.3 | 5.3 |
| 使えない素材の理由（補助の色に `opacity:.65`） | `style.css:277` | 2.8 | 3.3 |
| 入力欄の枠（`--line`）と面 | — | 1.5 | 1.3 |

- 決まり: `color-contrast`、`color-accessible-pairs`。屋外で読む画面なので、目盛りの数字と「配信中」を先に。
- 直し方
  - 目盛り: 線だけ薄くし、数字は `opacity` の外に出す。
  - ダークの札: 字を `#131B24` にする（ダークの `#FF4D5E` は spec に値がない）。
  - 素材の理由: `opacity` をやめて `--muted` のまま。
  - `--low #A86A00`: spec 14章でユーザーが認めた色。変えるかは判断がいる（まとめの「決めてほしいこと」）。

### B-3 ページの見出しが、中の見出しより小さい（中・小）
- 場所: `style.css:61`（`h2` は 15px の補助の色）。`h3` は既定の 18.72px の太字。
- 写し: `04-cameras-pc-light.png`、`07-settings-obs-pc-light.png`、`05-cameras-confirm-delete-pc-light.png`
- よくない点: 「カメラ」「設定」「ソース」（h2）が 15px の灰色で、下の「切替」「OBS」（h3）が 18.7px の黒。確かめの窓も、見出しが本文より小さい。カメラのページは「カメラ」と「カメラ 1（…）」が同じ h3。`visual-hierarchy`、`font-scale`。
- 直し方: ページの題は 20px ほどの太字（`--ink`）。区分の見出しは 17px。状態の小見出しは今のまま別のクラスに。カメラのカードは h4。

### B-4 スマホで開くの窓が、素のボタンのまま（中・小）
- 場所: `style.css:215`、`app/phone.js:11`
- 写し: `06-phone-dialog-pc-light.png`（ライトだけ見た）
- 実測: 「作り直す」「キャンセル」は地が `#EFEFEF`、枠が `2px outset`、角なし（`.btn` が付いていない）。選ぶ欄は高さ 28px。名前と欄の間に余白がない。
- 直し方: ボタンに `.btn` と `.btn.primary`。選ぶ欄と入力は `.settings-field` と同じ形。名前は上の行に。

### B-5 カメラの「送り先の住所」が、素の選ぶ欄のまま（中・小）
- 場所: `app/main.js:37`（`#camera-address`）
- 写し: `17-cameras-top-mobile-dark.png`、`04-cameras-pc-light.png`
- 実測: 高さ 28px、名前と欄の間に余白なし。ダークでは灰色の素の欄。下の注意の文は、枠も色もない素の段（状態の画面では `.warn` の箱）。
- 決まり: `touch-friendly-input`、`whitespace-balance`。
- 直し方: `.settings-field` の形にする。注意は `.warn` を付ける。

### B-6 保存の行の並びが、区分で変わる（中・小）
- 場所: `app/settings.js:49`（`.row`）、`style.css:103`
- 写し: `07-settings-obs-pc-light.png`（保存が左上）、`08-settings-admin-pc-light.png`（保存が右上、左上は「管理パスワードを作り直す」）
- よくない点: 「保存」の場所が区分で左右に動く。主のボタンの前に「作り直す」が来る。行は区分の一覧の下から始まり、右の欄とそろわない。`navigation-consistency`、`destructive-nav-separation`。
- 直し方: 「保存」「読み直す」を1行に固める。「パスワードを作り直す」はその欄の下。「本体を再起動する」は線で区切って下に。

### B-7 状態の「送り先」のボタンが重い（中・小）
- 場所: `app/main.js:27`（`.btn.to-send`）、`style.css:171`
- 写し: `03-status-pc-light.png`、`14-status-line-mobile-dark.png`
- よくない点: 52px の枠つきのボタンが、帯の下に左へずれて、カメラごとに繰り返す。配信中に見る画面で、状態より目立つ。s2-33c の7（見た目が違う）は直ったが、今度は重い。`visual-hierarchy`。
- 直し方: カメラの名前の行の右に、字だけのリンク「送り先 ›」（押せる高さは 44px を保つ）。

### B-8 状態の「操作」の並び（中・小）
- 場所: `app/main.js:33`、`style.css:117,193,195`
- 写し: `21-status-ops-mobile-dark.png`
- よくない点
  - 「カメラのソースをすべて表示に戻す」が区切りの線にくっつく（すき間 0）。どの節にも入っていない。
  - 「配信を始める」「配信を止める」が 10px のすき間で並ぶ。配信していなくても、OBS がつながっていなくても、両方とも押せる。
  - `.stop{margin-top:40px}` は `.row .stop` で消してある。
- 決まり: `touch-spacing`、`destructive-emphasis`、`disabled-states`。
- 直し方: 配信の状態で、出すボタンを1つにする。OBS がつながっていなければ押せなくして、理由を書く。「すべて表示に戻す」の置き場は `status-warning-heading-empty` の裁定に合わせる。

### B-9 分割のプレビューが縦に伸び、中身が読めない（中・中）
- 場所: `style.css:268`、`app/sources.js:32-33`（`paint`）
- 写し: `12-source-editor-open-pc-light.png`
- よくない点: 左右の分割で、プレビューの箱が左の列（値の一覧を開くと 800px を超える）と同じ高さに伸びる。中身は幅に合わせて縮むので、「0 kbps」が 7px ほどの字になる。`whitespace-balance`。
- 直し方: `.source-preview{align-self:start;position:sticky;top:72px}` とし、高さは幅と高さの比から決める。

### B-10 エディターの下の空き（低・小）
- 場所: `style.css:265`（`.source-body{min-height:260px;resize:vertical}`）、`:266`（`.cm-editor{min-height:180px}`）
- 写し: `10-source-editor-new-pc-light.png`、`24-source-editor-mobile-dark.png`
- よくない点: エディターの枠の下に 80px の空きと、大きさを変えるつまみだけが見える。
- 直し方: `.cm-editor{height:100%}`。

### B-11 設定の区分のボタンが、スマホで3つと1つに割れる（低・小）
- 場所: `style.css:211`（`flex:1 0 30%`）
- 写し: `19-settings-top-mobile-dark.png`、`27-settings-en-mobile-light.png`（英語は高さ 90px）
- 直し方: 2列×2段（`flex:1 0 45%`）。

### B-12 数の欄が広すぎる（低・小）
- 場所: `style.css:319-320`（S2-86 の「幅いっぱい」）
- 写し: `04-cameras-pc-light.png`（「2」「800」の欄が幅 890px）
- よくない点: s2-82 の12（空欄の字が切れる）は直ったが、数の欄まで広がった。`line-length-control`。
- 直し方: 数の欄は `max-width:12em`、字の欄は `max-width:40em`。

### B-13 細かい余白（低・小）
- カメラのカード: 「切替」の直下に「削除」がくっつく。「切替」は枠なし、「送り先」は枠つき（`04-cameras-pc-light.png`、`18-cameras-card-mobile-dark.png`）。
- カメラのカード: 状態が「DOWN / 0 kbps」の素の字。状態の画面の札と形が違う。
- メニューの「カメラ未保存」: 字の間にすき間がなく、色も同じ（`app/shell.js:20,22` の `<small>`。実測）。
- スマホの状態: 上の要約の札が横に切れる（`14-status-line-mobile-dark.png`）。「回線」のタブでは、すぐ下に同じ中身がある。
- ログイン: カードに見出しがない（`01-login-pc-light.png`）。
- ソースの一覧: テストの帯に題がなく、「新しく作る」だけ次の行に落ちる（`09-sources-list-pc-light.png`）。スマホでは一覧の前に4段を使う（`23-sources-list-mobile-dark.png`）。
- 共通の切替: 「自動切替」のスイッチが、しきい値の下にある（`04-cameras-pc-light.png`）。上に置くほうが自然。

---

## C: 文言（日本語）

### C-1 パスワードが違うとき「ログインが必要です」（高・小）
- 鍵: `messages` の `api.unauthorized`（ログインは `/home/teru358/project/irl-live-keeper/internal/web/auth.go:379`）
- 写し: `02-login-wrong-password.png`
- よくない点: 何が起きたかを言っていない。`error-clarity`。
- 案: ログインの失敗には別の鍵を作り「パスワードが違います」。`api.too_many_attempts` は「パスワードの誤りが続きました。しばらく待ってから、もう一度試してください」。

### C-2 ソースの保存の誤り「ソースの値の範囲が正しくありません。」（高・小〜中）
- 鍵: `sources.invalid`（`/home/teru358/project/irl-live-keeper/internal/web/sources.go:105,115,120`）
- 写し: `11-source-save-empty-name-error.png`
- 実測: 名前が空でも、名前に日本語や空白を入れても、この文だけ。画面にも「表示名」と「名前」の違いや、使える文字の案内がない。`error-clarity`、`error-recovery`。
- 案
  - 理由ごとに分ける: 「名前を入れてください」「名前に使えるのは、英数字と - と _ だけです」（規則は本体の検査に合わせる）。
  - 欄の名前: 「表示名」と「URL 用の名前」。
  - 欄の下: 「URL に使う名前。作ったあとは変えられません」。

### C-3 MediaMTX の状態が英語の「down」のまま出る（高・小）
- 場所: `app/main.js:32`（`t('mediamtx_'+…)||data.mediamtx`）。本体の値は `down`（`/home/teru358/project/irl-live-keeper/internal/supervisor/supervisor.go:29`）。`mediamtx_down` の鍵がない。
- 写し: `03-status-pc-light.png`、`14-status-line-mobile-dark.png`、`26-status-en-pc-dark.png`（英語でも「down」）
- 案: `mediamtx_down` を足す（「止まっている」。下の注意と合う言い方）。`mediamtx_stopped`「止まっている」と `mediamtx_failed`「停止している」も、違いがわかる文に（例: 「起動に失敗した」）。

### C-4 確かめの窓の題とボタンが、中身と合わない（高・小）
- 鍵: `settings_confirm`「変更の確認」、`settings_accept`「反映する」、`settings_restart_only`、`settings_restart_mode`、`settings_immediate`
- 写し: `05-cameras-confirm-delete-pc-light.png`（再起動の窓は a11y の木で読んだ）
- よくない点
  - 「本体を再起動する」の窓も、題が「変更の確認」、ボタンが「反映する」。中の文は「設定は変わりません」。
  - 「保存」を押して「反映する」で確定する。同じ操作が2つの名前。
  - 「再起動して反映」「すぐ反映」が、主語のない1行だけ。
  - 「カメラ 2（サブカメラ） — 削除」は、棒でつないだ形。
- 案

| 操作 | 題 | 確定のボタン |
|---|---|---|
| 保存 | 次の変更を保存します | 保存する |
| 再起動 | 本体を再起動します | 再起動する |
| 作り直し | パスワードを作り直します | 作り直す |

  - 反映の仕方: 「保存すると、本体が再起動します」「保存すると、すぐに反映します」。
  - 項目: 「カメラ 2（サブカメラ）を削除」。

### C-5 「＊ 反映にはアプリの再起動が必要」（中・小。裁定の文）
- 鍵: `settings_restart_note`。spec 14章の裁定の文そのもの。
- 写し: `04-cameras-pc-light.png`（同じ注記が2回）、`07-settings-obs-pc-light.png`
- よくない点: 利用者が自分で再起動するように読める（実際は、保存すると本体が自分で再起動する）。下のボタンは「本体を再起動する」で、「アプリ」と「本体」が混ざる。
- 案: 「＊ 保存すると本体が再起動します」。1ページに1回だけ。裁定の文を変えるので、ユーザーの判断がいる。

### C-6 同じものを別の名前で呼んでいる所（中・中）
写し: なし（文の表と画面から）

| もの | 今の呼び方（鍵） | そろえる案 |
|---|---|---|
| アプリ | 「本体」（`settings_restart` `disconnected`）、「アプリ」（`settings_restart_note`） | 「本体」 |
| 切り替え | 「切替」（`setting_switch` `setting_auto_enabled`、`messages` の多く）、「切り替え」（説明の文） | 「切り替え」 |
| 相手の場所 | 「住所」（`sender_address`、説明の文）、「ホスト」（`setting_host` `setting_obs_read_host` `setting_publish_host`）、「アドレス」（`sender_endpoint`） | 欄の名前は「住所」 |
| 利用者の名前 | 「ユーザー」（`sender_user` `setting_read_user`）、「ユーザー名」（`sender_copy_all` `sender_rtmp_hint`）、「認証名」（説明の文） | 「ユーザー名」 |
| 捨てる | 「破棄しますか？」（`settings_discard` `src_discard`）、「捨てて」（`settings_restart_dirty`）、「戻してください」（`settings_regenerate_dirty`） | 「捨てる」 |
| 確かめる | 「確認」（`settings_confirm` `settings_failed` `settings_pending_urls`）、「確かめ」（`settings_camera_sources_unreadable`） | どちらか（決めてほしい） |
| 回線が悪い | 「不調」（`setting_fail_kbps` `setting_on_bad`）、「LOW」 | 「LOW にするしきい値 (kbps)」のように、状態の名前に合わせる |
| Studio Mode | 「Studio Mode」（状態、注意の文）、「スタジオモード」（`setting_studio_mode_hide`） | 「スタジオモード」 |
| フォルダ | 「フォルダ」（`settings_default_binary`）、「フォルダー」（`setting_dir_help`） | 「フォルダー」 |
| 送る・読む | 設定の区分「送信」の中身は OBS が読む設定。カメラのページの「送信」はカメラが送る設定 | 設定の区分は「OBS の読み取り」 |
| ソース | 管理ページの「ソース」（ブラウザソース）と、`show_all`「カメラのソースをすべて表示に戻す」（OBS のメディアソース） | 状態のボタンは「隠したカメラの映像をすべて戻す」 |
| 素材を入れる | `src_upload`「素材を上げる」。spec と誤りの文は「取り込み」。`src_obs_import`「OBS への取り込み」は別のこと | 「素材を足す」と「OBS に入れる手順」 |
| 表示 | `sender_show`「表示」、`src_title`「表示名」、`src_view`「表示」（画面の分け方） | 分け方は「画面の分け方」 |
| 待機なし | `standby_inactive`「していない」、`src_standby_none`「なし」 | 「待機していない」 |

### C-7 ボタンの動詞の形がそろっていない（中・小）
- 写し: `04-cameras-pc-light.png`、`10-source-editor-new-pc-light.png`
- 名詞だけ: 「保存」「削除」「追加」「編集」「コピー」「条件」（`settings_save` `settings_remove` `settings_add` `src_edit` `sender_copy` `src_condition`）
- 動詞: 「読み直す」「反映する」「本体を再起動する」「新しく作る」「差し込む」「一覧へ戻る」「配信を始める」
- よくない点: 同じ行に「保存 / 読み直す / 本体を再起動する」と並ぶ。spec 5章はカメラの追加を「新しく作る」と書くが、画面は「追加」。「条件」は、押すと何が起きるかがわからない。
- 案: 動詞にそろえる（「保存する」「削除する」「編集する」「コピーする」「条件を差し込む」）。名詞にそろえるのでもよい。

### C-8 スマホで開くの窓の文（中・小）
- 鍵: `sender_address`（使い回し）、`phone_create`、`settings_cancel`（使い回し）
- 写し: `06-phone-dialog-pc-light.png`
- よくない点: 欄の名前が「送り先の住所」（スマホが開く住所のこと）。まだ何も作っていないのに「作り直す」。閉じるボタンが「キャンセル」。
- 案: 「スマホから開く住所」「QR を作る」（作ったあとは「QR を作り直す」）、「閉じる」。

### C-9 設定の「有効」と、使い回しの説明（中・小）
- 鍵: `setting_enabled`、`setting_port_help`、`setting_password_help`
- 写し: `07-settings-obs-pc-light.png`、`08-settings-admin-pc-light.png`
- よくない点
  - OBS の区分に、見出しなしで「有効」だけのスイッチ（`on_stream_start.enabled`）。
  - OBS の「ポート」の説明が「接続を受け付けるポート番号。」（本体が OBS へつなぐ先のポート）。管理ページのポートと同じ鍵。
  - 管理ページの「パスワード」の説明が「接続時の認証に使うパスワード。」（OBS と同じ鍵）。
- 案: 鍵を分ける。
  - スイッチ: 「配信を始めたらシーンを切り替える」
  - OBS: 「OBS の WebSocket のポート」「OBS の WebSocket のパスワード」
  - 管理ページ: 「管理ページを開くポート」「管理ページのログインのパスワード（8文字以上）」

### C-10 文の調子と、終わりの「。」（中・小）
- 写し: `12-source-editor-open-pc-light.png`（「保存しました」）ほか
- 知らせ: `src_saved`「保存しました」と `sender_copied`「コピーしました」は「。」なし。`settings_reloaded`「読み直しました。」と `settings_no_changes`「変更はありません。」は「。」あり。
- 説明の文: 体言止め（「接続を受け付けるポート番号。」）と、です・ます（「カメラから RTMP で映像を受信します。」）が1つの区分に混ざる。
- 誤りの文（`messages`）: 常体（「シーン {name} が OBS にない」「{purpose} が失敗した」「OBS に未接続」）と、です・ます（「シーンが見つかりません」）が混ざる。
- 案: 知らせと誤りは、です・ますで「。」あり。説明の文は「〜します。」。状態の値は短い体言止め（「未接続」「待機していない」）。

### C-11 漢語が固い所（低・小）
写し: なし（文の表から）

| 鍵 | 今 | 案 |
|---|---|---|
| `settings_lan`、`settings_lan_notice` | 通信は平文で、LAN 内で盗み見られるおそれがあります。接続できる機器から… | 通信は暗号にならないので、同じ LAN の人に中身を見られることがあります。つなげる機器からは… |
| `setting_api_port_help` | 本体が MediaMTX の状態を取得するポート。 | 本体が MediaMTX の状態を読むポートです。 |
| `setting_keep_days_help` | この日数を超えたログを削除します。 | この日数より古いログを消します。 |
| `settings_discard`、`src_discard` | 未保存の変更を破棄しますか？ | 保存していない変更を捨てますか？ |
| `settings_reverted` | 反映に失敗したので元の設定に戻しました。 | 反映できなかったので、前の設定に戻しました。 |
| `setting_on_bad` | 回線不調時 | 回線が悪いときの動き |
| `setting_studio_mode_hide` | スタジオモード中の動作 | スタジオモードのときの動き |
| `setting_close_action` | 閉じるときの動作 | 窓を閉じるときの動き |
| `api.stopped` | 中核ループが止まっている | 本体が止まっています。本体を起動し直してください。 |
| `api.internal` | 本体の中で誤りが起きました | 本体で誤りが起きました。ログを見てください。 |
| `settings.invalid` | 設定の内容を確認してください。 | 赤い字の欄を直してください。 |
| `api.invalid` | 受け付けられない: {reason} | {reason} のため、受け付けられません。（s2-33b の12 の「values」が今も出るかは未確認） |

### C-12 どうすればよいかを言っていない誤り（中・小）
写し: なし（文の表から）

| 鍵 | 今 | 足すこと |
|---|---|---|
| `api.disconnected`、`scenes_disconnected` | OBS に未接続 | OBS を起動し、「設定」の「OBS」のホスト・ポート・パスワードを確かめてください |
| `sources.write_failed` | ソースを保存できません。 | sources.yaml に書けるかを確かめてください |
| `source_files.io` | 素材を読み書きできません。 | source-files フォルダーに書けるかを確かめてください |
| `failed` | 操作に失敗しました | もう一度試してください。続くときはログを見てください |
| `disconnected` | 本体につながっていません | 自動でつなぎ直します |
| `api.deadline`、`api.canceled` | context deadline exceeded / context canceled（日本語の表に英語のまま） | 時間内に終わりませんでした。もう一度試してください |

### C-13 OBS がつながっていないときの見え方（中・小）
- 写し: `03-status-pc-light.png`、`22-status-scene-mobile-dark.png`、`07-settings-obs-pc-light.png`
- 今
  - 状態の「OBS」: 赤い字で「つながっていない、Studio Mode はオフ」
  - シーンの一覧: 「OBS に未接続」の1行だけ
  - 設定: 「OBS 未接続です。名前を直接入力してください。」
- よくない点: つながっていないのに Studio Mode の値を言う。3か所で言い方が違う。次に何をすればよいかがない。`empty-states`、baseline-ui「空の状態には次の操作を1つ」。
- 案: 状態は「つながっていない」だけ。シーンの欄は「OBS につながると、シーンがここに並びます」と「OBS の設定を開く」のボタン。

### C-14 そのほかの文（低・小）
写し: なし（文の表と画面から）
- `sender_copy_all`「ユーザー名とパスワードを含む URL を全部コピー」: 長い。「URL をコピー（パスワード入り）」。spec は「URL を全部コピー」。
- `src_custom`「自由」と `src_actual`「実際」: 何のことかがわかりにくい。「値を自分で書く」「今の値」。
- `src_test`: 帯の題として目に見えない（`aria-label` だけ）。「回線の状況のテスト」を出す。
- `settings_camera_comments`「config.yaml の cameras 区分の中のコメントは残りません」: 画面だけで設定する人には要らない。保存の確かめの窓へ移す。
- `settings_unsaved`「未保存」: メニューの読み上げが「カメラ未保存」。`aria-label` を「カメラ（保存していない変更あり）」に。
- `manual_hold_hint`「戻るときは上の「戻る」を押します。」: ボタンの名前は「<シーン> に戻る」で、スマホでは別のタブにある。「「シーン一覧」の「〜に戻る」を押します」。
- `scenes`「シーン一覧」: 下のタブ。spec 14章は「シーン」。
- `settings_group_camera`「カメラと切り替え」: spec でなくした区分の名前が残る（隠れた欄で使われ、目には見えない）。
- `setting_language_help`「窓とコンソールの言語、ブラウザが言語を選んでいないときの既定。」: 「小さな窓の言語です。管理ページは、右上で選んでいなければこの言語になります」。
- `down`「停止」: 管理ページの JS からは使われていない（小さな窓で使うかは未確認）。

---

## D: 英語

写しは D-1 が `02-login-wrong-password.png` と同じ画面（英語では撮っていない）、D-2〜D-5・D-20 が `26-status-en-pc-dark.png`、D-6 が `27-settings-en-mobile-light.png`、D-12・D-13 が `28-sources-en-mobile-light.png`。ほかは文の表から。作業の大きさは、D-9 が小〜中、ほかは小。

| 番号 | 重さ | 鍵 | 今 | よくない点 | 案 |
|---|---|---|---|---|---|
| D-1 | 高 | `api.unauthorized` | unauthorized | 違うパスワードの知らせがこの1語 | Wrong password. |
| D-2 | 中 | `links` | Links | リンク（URL）に読める。下のタブにも出る | Connection |
| D-3 | 中 | `sender_link`、`camera_destination` | Send to | ボタンと見出しとして不自然 | Publish URL |
| D-4 | 中 | `setting_stream`、`settings_group_stream` | Sending | 区分の名前として不自然 | Publishing |
| D-5 | 中 | `settings_restart`、`disconnected`、`settings_stream_warning` | Restart keeper / the application / The app | 3つの呼び方 | the app にそろえる |
| D-6 | 中 | `setting_admin` | Admin API | 区分は Admin page and MediaMTX | Admin page |
| D-7 | 中 | `setting_low_sec`、`setting_down_sec` | Low duration / Down duration (seconds) | 何の秒数かわからない | Seconds before LOW / Seconds before DOWN |
| D-8 | 中 | `setting_app`、`setting_binary`、`setting_dir` | Application / Binary / Directory | 日本語（アプリ名、MediaMTX の場所、保存先）と合わない | App name / MediaMTX location / Log folder |
| D-9 | 中 | `sources.invalid` | Invalid source values. | C-2 と同じ | 理由ごとに分ける |
| D-10 | 低 | `manual_save_hint` | Recovery will not return automatically. | 主語が変 | It will not switch back automatically when the connection recovers. |
| D-11 | 低 | `phone_create` | Generate again | C-8 と同じ | Create QR code |
| D-12 | 低 | `src_test`、`src_values_test`、`src_stopped` | Connection simulation / Test values / simulation in OBS | test と simulation が混ざる | test にそろえる |
| D-13 | 低 | `stream`、`src_stream`、`src_line` | Streaming / Stream / Line | 同じ見出しが2つの形 | Stream と Connection に |
| D-14 | 低 | `loss`、`editor_close` | loss / close | 小文字で始まる | Loss / Close |
| D-15 | 低 | `settings_restart_note` と項目の印 | ＊ | 全角の印が英語の文に出る | * |
| D-16 | 低 | `settings_saved` | Save accepted. | 不自然 | Saving… |
| D-17 | 低 | `setting_option_auto`、`theme_auto` | Automatic / Auto | 2つの形 | Auto |
| D-18 | 低 | `settings_lan_notice` | 文の終わりに空白 | ごみ | 空白を取る |
| D-19 | 低 | `api.stopped`、`api.deadline` | The core loop is stopped / context deadline exceeded | 中の言葉 | The app has stopped. Restart it. / Timed out. Try again. |
| D-20 | 低 | MediaMTX の状態 | down | C-3 と同じ | Stopped |

英語でも、390px の幅で横にはみ出す所と、切れる字はなかった（状態・カメラ・ソース・設定で実測）。

---

## 実機の結果の指摘の、直ったものと、まだのもの

**直っていた（画面かコードで確かめた）**
- s2-33: 1、2、3、6、13、14、17（ただしブラウザの窓。A-5）
- s2-33b: 6、7、8（並びはそろった）、9、10、18、19、21、22
- s2-33c: 2、3、5、6
- s2-54: 8（表示名）、9、10、15、16、17（id も name もない欄は 0 件）
- s2-54b: 4、8（待機の帯は `display:none` で読まれない）
- s2-82: 1（コード）、2（id の重なりはなし）、4、10（名前は「送り先」に）、12、13、17
- s2-88: 1、2、3（値が `aria-label` に入った）、5

**まだ**
- s2-33b の15: 窓の文の形（C-4）
- s2-33b の17: `role="textbox"` で避けた形（A-16）
- s2-33c の4: 「保存日数」は `min` だけ（A-16）
- s2-33c の7: 「送り先」のボタンの見た目（B-7）
- s2-54c の1・7・8: チケット `source-value-meanings`、`source-unknown-name-reason`、`source-values-open-state` のまま。7 は今回も見た（「1 行 8 文字目: 知らない値の名前です」で名前が出ず、赤い線は先頭の1字だけ）
- s2-82 の8: カメラの「読み直しました。」は、ページを移るまで残る（A-7）
- s2-82 の9・14・18: チケットのまま（`status-warning-heading-empty`、`regenerate-button-placement`、`phone-menu-open-on-phone`）
- s2-88 の7: そのまま

**確かめられなかった（OBS 未接続、配信なし、初回でない）**
- s2-33 の7・22・24、s2-33b の11・12・14・16・20、s2-54 の13、s2-54b の2、s2-82 の11、s2-88 の6

---

## まとめ

### 直すとよい順の上位10件
1. B-1 ダークでプレビューが白くなる（小）
2. C-1 と D-1 違うパスワードの文（小）
3. C-3 MediaMTX の「down」（小）
4. A-1 スマホのメニューの今の画面の印（小）
5. A-4 カメラの削除の窓に、受信が切れる文を出す（小。spec と食い違う）
6. C-2 ソースの保存の誤りと、名前の案内（小〜中）
7. C-4 と A-3 確かめの窓の題・ボタン・名前・最初のフォーカス（小）
8. B-2 色の差（目盛りの数字、ダークの「配信中」）（小）
9. A-2 スマホのメニューを dialog に（中）
10. B-4 と C-8 スマホで開くの窓の見た目と文（小）

その次: A-10・A-11・A-12（押せる大きさ）、A-8（保存の帯）、A-13（URL にページ）、B-3（見出しの階層）、B-6（保存の行）、C-6（呼び方をそろえる）。

### spec と食い違う所
- 14章「今の画面に印」: スマホのメニューに印が出ない（A-1）
- 5章「全カメラの受信が一度切れることを確かめの窓で示す」: 配信中でないと出ない（A-4）
- 5章「「新しく作る」で…番号を付ける」: カメラの追加のボタンは「追加」（C-7）
- 5章のソース「「回線の状況のテスト」の切り替え」: 題が目に見えない（C-14）
- 14章のスマホの幅「「回線」「シーン」「操作」」: 2つ目は「シーン一覧」（C-14）
- 14章「カメラごとに「URL を全部コピー」」: 今は「ユーザー名とパスワードを含む URL を全部コピー」（s2-33b の21 を受けた直しと見られる）
- 14章の配色: ダークの配信の札の値がない。今の `#FF4D5E` は白い字との比が 3.2（B-2）
- **spec の中の食い違い**: `language` は14章「日本語と英語」が「すぐ反映」、7章の表が「再起動して反映」。画面と本体は7章のとおり（言語の欄に「＊」）。

### ユーザーに決めてほしいこと
1. 「＊ 反映にはアプリの再起動が必要」を「＊ 保存すると本体が再起動します」に変えるか（C-5。裁定の文）
2. 「アプリ」と「本体」のどちらにそろえるか（C-6）
3. 「確認」と「確かめ」、「破棄」と「捨てる」のどちらにそろえるか（C-6）
4. 「切替」と「切り替え」のどちらにそろえるか（C-6）
5. 「住所」「ホスト」「アドレス」のそろえ方（C-6）
6. ボタンは動詞（「保存する」）か名詞（「保存」）か。確定のボタンを「反映する」から操作の名前に変えるか（C-4、C-7）
7. 設定の区分「送信」の名前を変えるか（中身は OBS の読み取り）（C-6）
8. カメラの追加は「追加」か、spec の「新しく作る」か（C-7）
9. 確かめを、ブラウザの窓から画面の窓にそろえるか（A-5。作業は中）
10. 状態の「配信を始める / 止める」を、状態で1つだけ出す形にするか（B-8）
11. カメラの「削除」を、カードを残して「消す予定」と出す形にするか（A-9）
12. 状態の「送り先」を字だけのリンクにするか（B-7）
13. ページごとの URL（`#cameras` など）を入れるか（A-13）
14. ライトの LOW の色 `#A86A00`（spec で認めた色）を、比 4.5 に届く濃さへ変えるか（B-2）
15. `language` の反映の仕方は、spec の14章と7章のどちらに合わせるか

### 写しの一覧
すべて `/tmp/claude-1000/-home-teru358-project-irl-live-keeper/9ba12fa0-d1e3-40a3-86be-c9d0f6c7da54/scratchpad/ui-audit/` の中。

- `01-login-pc-light.png`
- `02-login-wrong-password.png`
- `03-status-pc-light.png`
- `04-cameras-pc-light.png`
- `05-cameras-confirm-delete-pc-light.png`
- `06-phone-dialog-pc-light.png`
- `07-settings-obs-pc-light.png`
- `08-settings-admin-pc-light.png`
- `09-sources-list-pc-light.png`
- `10-source-editor-new-pc-light.png`
- `11-source-save-empty-name-error.png`
- `12-source-editor-open-pc-light.png`
- `13-source-editor-error-free-pc-light.png`
- `14-status-line-mobile-dark.png`
- `15-menu-sheet-mobile-dark.png`
- `16-cameras-mobile-dark.jpeg`
- `17-cameras-top-mobile-dark.png`
- `18-cameras-card-mobile-dark.png`
- `19-settings-top-mobile-dark.png`
- `20-settings-bottom-mobile-dark.png`
- `21-status-ops-mobile-dark.png`
- `22-status-scene-mobile-dark.png`
- `23-sources-list-mobile-dark.png`
- `24-source-editor-mobile-dark.png`
- `25-status-pc-dark.png`
- `26-status-en-pc-dark.png`
- `27-settings-en-mobile-light.png`
- `28-sources-en-mobile-light.png`

## ユーザーの決め（2026-11-28）

「ユーザーに決めてほしいこと」の15件への答え。番号はその一覧の番号。

- 1〜3、6〜9、11〜15: 指揮者のおすすめのとおり
  - 1: 「＊ 保存すると本体が再起動します」
  - 2: 「本体」
  - 3: 「確認」「捨てる」
  - 6: ボタンは動詞。確認の窓の確定は操作の名前（「保存する」「再起動する」「作り直す」）
  - 7: 設定の区分「送信」は「OBS の読み取り」
  - 8: カメラの追加は「追加」のまま、spec を直す
  - 9: 確認はブラウザの窓をやめ、画面の窓にそろえる
  - 11: カメラの削除はカードを残して「保存すると削除します」
  - 12: 状態の「送り先」は字だけのリンク
  - 13: ページごとの URL（`#cameras` など）
  - 14: ライトの LOW の色を比 4.5 に届く濃さへ
  - 15: `language` は7章（再起動して反映）にそろえ、14章を直す
- 4: **「切替」にそろえる**（おすすめの「切り替え」ではない）
- 5: **「アドレス」にそろえる**（おすすめの「住所」ではない）
- 10: 配信のボタン
  - 文言は OBS と同じ「配信開始」「配信終了」
  - ボタンは配信の状態で切り替わる1つ
  - 確認の窓は「配信を開始しますか？」「配信を終了しますか？」、ボタンは「はい」「いいえ」
  - 配信終了は赤。同じ種類の操作のボタンの色とそろえる
- ボタンの色の決まり（10 を受けた指揮者の案）

| 操作の種類 | 例 | 色 |
|---|---|---|
| 始める・決める | 配信開始、保存する、確認の窓の確定（保存・再起動・作り直し） | 青の塗り（`.btn.primary`） |
| 終わらせる・消す | 配信終了、カメラ・ソース・素材の削除、確認の窓の確定（終了・削除） | 赤（`.btn.stop`） |
| そのほか | 読み直す、コピー、いいえ・キャンセル | ふつう |

- 未決: 「はい」「いいえ」を配信の確認の窓だけにするか、ほかの確認の窓もそろえるか
