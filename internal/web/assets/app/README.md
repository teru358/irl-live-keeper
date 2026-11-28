# 管理ページ

状態は store.js の入れ物に集め、get / update(fn) / subscribe を通して扱う。
get は凍結した値を返す。購読通知と描画は queueMicrotask でまとめる。
画面は状態を受け取って lit-html の html を返す関数。root ごとの try/catch で描画の失敗を隔離する。
要素を ID で探して内容を書き換えない。root の取得、スクロール、フォーカスなどの DOM 操作だけを境界に置く。
notice / senderNotice は操作の知らせ、status は SSE の状況専用。SSE は知らせを変更しない。
events.js と api.js と store.js は DOM 非依存。

shell.js はログイン・初回パスワード・メニュー・テーマ・言語、settings.js は設定と再生成の確認窓、phone.js はスマホ用リンクを担当する。
入力値と未保存の基準、欄のエラー、操作の知らせは各 store に保持する。入力には live() を使う。
SSE は設定の状況と保存済み値の再取得だけを扱い、操作の知らせを変更しない。
Node テストは assets 外の jstest に置く。Go から Node を自動起動せず、開発時に明示実行する。
spec（`docs/superpowers/specs/2026-11-20-stage2-design.md` v0.35）の5章に従う。

## つなぎ直し

events.js の openStream は EventSource を包む。ブラウザは本体が止まっただけなら自分でつなぎ直す（readyState が 0）が、
本体のプロセスを起動し直すとセッションがなくなり、`/api/events` が 401 を返して EventSource は閉じる（readyState が 2）。
閉じたら、間をあけて `request('status')` を試す。401 なら request がログインへ移す。通ればつなぎ直す。それ以外の失敗は、また間をあけて試す。
状態のページ（connectEvents）とソースのページの SSE（sources.js）が同じ道を使う。OBS に入れるソースのページ（assets/source.js）は認証がなく、別。

## エディター

editor.js の mountEditor(container, {value, names, files, onFiles, label, t, onChange}) は Promise でエディターを返す。
ソース編集を開いたときに呼ぶ。この呼び出しまで CodeMirror は読み込まれない。
names は値の名前の文字列配列、files は素材の一覧を返す関数、onFiles は落とされたファイルを受ける関数、
label は翻訳済みの欄名、t は既存の UI 文の表を読む関数。
lit-html は空の安定した入れ物（同じテンプレート位置の div）だけを描き、その子を描かない。描画後に mountEditor を呼ぶ。
同じ要素では同じ部品を返す。画面を閉じるとき destroy()、作り直すときは再び mountEditor を呼ぶ。
入力途中の setValue は避ける（IME のイベントは CodeMirror 標準のまま）。

返り値: setValue(text)、insert(text)、insertAsset(name)、setErrors([{line, column, message}])、destroy()、view。
差し込みは、人がカーソルを置いたあとは選択範囲を置き換え、未選択ならカーソル位置に入れる。まだ置いていないときは末尾に入れる。
insertAsset の形は source-body.js の bodyAssetInsertion が決める。`<style>` の外は `<img src="/source/files/名前">`、
`<style>` の中の規則の中（declaration）と、コメント・文字列・@keyframes の直下は `url("…")` だけ、規則の外（rule）は `.image { background: url("…") no-repeat; }`。
その場所は cssContext が、カーソルの前の CSS から数える。
message は API が翻訳した文を渡す。line/column は1始まり、column は Unicode 文字単位。
エラーは現在の文書に対応するプレビュー応答だけを渡し、編集後には setErrors([]) で古い印を消す。
CodeMirror の言語は本文用の template-lexer.js だけで、CSS 用の @codemirror/lang-css は読み込まない（vendor には残してある）。

## ソース

ソースの編集は previewError / editorError / editorNotice を持ち、一覧の notice と分ける。
sources の SSE によるテスト解除の案内は testNotice に置き、操作の知らせは上書きしない。
回線の状況のテスト（sources-state.js の simulateOf）は、自由の JSON が読めないとき、文の表の文（src_custom_invalid）で例外を投げる。
プレビューは sandbox="allow-same-origin" の iframe に保持する。reconcile.js の updateRendered で文字列・DOMParser の木を再検査し、検査を通った本文を共通の reconcile で更新する。
HTML の規則（html_rules.json）は rules.js の loadRules が読む。失敗したら間をあけて取り直す（source.js と同じ）。
素材を上げるのも request を通す。request は FormData をそのまま送る（Content-Type はブラウザに任せる）ので、401 でログインへ移る。
URL のコピーは clipboard.js の copyValue（clipboard API、だめなら execCommand）で、カメラの送り先のコピーと同じ部品を使う。

## 設定

settings.js は cameras / settings のページごとに独立した store と確認窓を持つ。
GET の field_locations が項目の所属を決め、settings-pages.js の projectPage で送る値を絞る。
settings-pages.js の rebaseDraft は新しい保存済み値へ差分を当て直し、カメラは id で照合する。裏の読み直し（load(true)）は、取得の間に入力があっても、
その下書きを新しい値へ当て直す（settings-flow.js の applyLoaded）。
keeper-settings-refresh で、保存したページでないほうのページがハッシュを更新する（すぐ反映のときだけ。保存したページは自分で読む）。
再起動して反映のときは読みに行かず、status の apply が applied か reverted になったとき（connection.js の reloadAfterApply）に、両ページの設定と送り先を読み直す。
status の OBS の接続（store の obs）は、設定のシーンの選ぶ欄に要る。OBS がつながったら、ページに設定があればシーンを読み直す（applyStatus、sceneReadNeeded）。
keeper-draft はメニューの印だけに使う。「直前の保存を取り消す」はない（窓の「元に戻して再起動」と --cli restore-backup だけ）。
「本体を再起動する」は、そのページに下書きがあれば断り、知らせを出す（restartBlocked。パスワードの作り直しと同じ形）。再起動だけの確かめの窓は「設定は変わりません」と出し、保存済みの基準は変えない。
別のページの項目の誤りが返ったときは、その誤りを keeper-field-errors でそのページの store へ渡してから、そのページへ移る（settings-flow.js の receiveErrors）。
設定の読み込みの失敗は「保存していません。」を付けない。保存と確かめの失敗は付ける。
connection.js は接続・知らせ・読み直しの DOM を使わない部品（isOnline、setNotice、createSenderLoader など）、
settings-flow.js は設定ページの失敗・読み込み・状態・保存の終わり・変更なしの判定（Node で偽の部品を渡して試せる）。
api.js の request は、応答のない失敗（fetch の TypeError）を文の表の「本体につながっていません」の文にして network を立てる。
network の知らせは、つながり直したとき（keeper-reconnected）に消す。知らせは、別のページへ移ったとき、同じ欄の次の操作を始めたとき、
元の状態が終わったとき（例: OBS のテストの解除の知らせは配信が止まったとき）に消す。
カメラの送り先は main.js が状態を持ち、settings.js の空の安定した入れ物を root として描く。
パスワードの目のボタンは eye.js の passwordEye が1つだけ持つ（shell.js、settings.js、main.js が使う）。

## 確認の窓と配信のボタン

確認の窓は confirm-dialog.js が決める（DOM を持たない）。confirmSpec が題（「〜しますか？」）、題の下の行、「はい」の色（終わらせる・消すは赤の `.btn.stop`、ほかは青の `.btn.primary`）、開いたときのフォーカス（赤は「いいえ」）を返し、confirmContent が中身、showConfirm が showModal・最初のフォーカス・閉じたあとの元のボタンへの戻しを担当する。
Escape は「いいえ」と同じ。ブラウザの `confirm()` は使わない。1つの質問だけの窓は confirm.js の askConfirm(spec, opener)（true なら「はい」）。
設定とカメラの窓（`#settings-confirm`、`#cameras-confirm`）は要素を残し、同じ中身と同じ決め方（settings-state.js の confirmationSpec）を使う。
元のボタンは忙しい間 disabled になるので、押した時点で控え、使えるようになってから戻す。
ソースのページを離れるときの確認は答えが遅れて来るので、keeper-before-page を先に止め、「はい」のあとで keeper-navigate をもう一度出す。
配信のボタンは stream-control.js（状態の画面の「配信開始」「配信終了」の1つ。OBS 未接続は押せず理由を出す。配信開始の窓に on_stream_start のシーンを出すため、押したときに settings を読む）。
カメラの「削除」はカードを残す。削除する下書きから外し（dirty、保存はそのまま動く）、保存済みのカメラは settings-state.js の removeCamera / undoCameraRemoval / cameraCards で「保存すると削除します」と「取り消す」のカードにする。

## 文言（S2-92）

文の表（`internal/i18n/locales`）の言葉は、本体・確認・捨てる・切替・アドレス・OBS の読み取りにそろえる。ボタンは動詞（保存する・削除する・編集する・コピーする）。追加と、確認の窓の「はい」「いいえ」は変えない。
`confirmationDanger`（settings-state.js）は、カメラを消す保存と、配信中に本体を再起動する操作（再起動、再起動して反映の保存、再起動を伴う作り直し）を赤にし、開いたときのフォーカスを「いいえ」に置く。配信していなければ青。警告の行（`settings_stream_warning`）と同じ条件。
項目の名前と説明は `settingText(t, path, key, suffix)`。ふつうは `setting_<最後の名前>` だが、同じ名前で意味が違う `specificSettings`（obs.port、admin.port、obs.password、admin.password、on_stream_start.enabled）だけは `setting_<パス>` を使う。
「＊ 保存すると本体が再起動します」は、1ページに1回だけ、項目の並びの下に出す（印は `settings_restart_mark`）。カメラの保存で config.yaml のコメントが消えることは、保存の確認の窓の行で言う。
ソースの保存の誤りは、理由ごとの鍵（`sources.name_empty`・`name_chars`・`name_long`・`title_long`・`size_range`・`reserved_name`・`duplicate`）で返る。「URL 用の名前」の欄の下に、使える文字と「作ったあとは変えられません」を出す。
OBS につながっていないとき、状態の画面のシーンの欄は「OBS の設定を開く」を出し、`keeper-settings-group`（設定の区分）と `keeper-navigate` で設定の「OBS」へ移る。
