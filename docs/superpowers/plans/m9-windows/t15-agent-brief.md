# T-15 の2日目の代理の確認（外を歩く。キャリア回線からの揺れ。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（以下「本体」）の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

1日目（置いたまま）は終わっていて、結果は同じフォルダの `t15-result.md` にある。**今日は2日目で、ユーザーが iPhone を持って外を歩く。** 先に `t15-result.md` を読み、1日目の数字（とくに「まとめ」「数字」「意見」）を頭に入れておく。

**前提・守ること・道具・片づけは、同じフォルダの `hide-agent-brief.md` のとおりにする。** 準備は、下の「準備」のとおりにする（`hide-agent-brief.md` の準備の 2〜7 を使い、スマホの送り方だけ違う）。

## 今日の形

- 持って歩くのは **iPhone（カメラ `sub`）だけ**。送り方は2つで、どちらも **キャリアのモバイル回線から Tailscale を通して** `sub` へ送る（Wi-Fi を切り、iPhone の Tailscale はつないだまま。URL のホストだけを Windows の Tailscale のアドレス `100.…` にする。`config.yaml` の `stream.publish_host` は変えない）
  - **前半（10分）: Larix Broadcaster の SRT**（URL の欄に `srt://100.…:8890`、streamid の欄に `publish:live/sub:<ユーザー>:<パスワード>`）
  - **後半（10分）: It's My Live の RTMP**（URL は `rtmp://100.…:1935/live/sub?user=…&pass=…`）
  - どちらも、1日目と同じ設定（送るビットレートは約 2,000 kbps）のままにしてもらう。違う設定にしたら、値を聞いて記録する
- **ユーザーは、外にいる間、このセッションに何も知らせられない。** 流れはユーザーが決めたとおり: 家の中で Larix の SRT を始める → 外に出て約10分歩く → 立ち止まって It's My Live の RTMP に替える → 約10分歩く → 帰ってきてから、このセッションに話しかける。**区切りの時刻は、あなたがログと CSV から自分で見つける**（下の「区切りの見つけ方」）。歩く間は、裏で動く記録（見張りと `walk-log.ps1`）に任せる
- 歩く前に、家の中で **Android（カメラ `main`、LAN）の IRL Pro を使って、新しい警告を確かめる**（下の W-0、約5分）
- **歩く間も、Android（`main`）は家の中から LAN で送り続ける**（IRL Pro、720p・3,000 kbps 固定）。シーンは **`T-A`（main と sub の2台）** にする。実際の使い方（1台は置いたまま、1台は持って歩く）に近い形で、次を確かめるため:
  - sub（`on_bad: hide`）が悪くなったとき、待機へ送らずに **sub だけを隠し**、良くなったら戻すこと。外の揺れで、隠す・戻すがばたつかないか
  - main を、比べるための基準にする。sub と同じ時刻に main も落ちていたら、キャリア回線ではなく PC や家の側の原因とわかる
  - Android は、充電につなぎ、画面が消えないようにしておいてもらう（main が止まると、T-A は待機へ送られ、sub の結果と混ざる。止まったら時刻を記録し、送り直してもらう）
- **本体は `irl-live-keeper.exe`（2026-11-23 00:06 の版、14,259,200 バイト）と `ctl.exe`（11,416,576 バイト）**。`watch-status.ps1` は 11-22 21:36 の版（`-Csv` がある）。準備 1 で、ユーザーに入れ替えたことを確かめてもらう
  - 1日目の版から増えたこと: OBS が映像を読み切れないとき（MediaMTX の `reader is too slow` が30秒で3回以上）、status の `warnings` に「<カメラの名前> の映像を OBS が読み切れていません（MediaMTX が映像を捨てています）。…」が1行出て、本体のログに INFO で `obs reader too slow camera=<id> count=<回数>` が出る。最後の `reader is too slow` から60秒たつと、警告が消え、`obs reader caught up camera=<id> count=0` が出る。**この警告は健康には入らない**（待機へ送らない・隠さない）
- MediaMTX の path は2段（`live/main`、`live/sub`）。1日目と同じ
- Studio Mode は使わない（外での配信では非推奨にした）。始める前に `.\ctl.exe studio status` が off であることを確かめる
- 記録のファイルは `t15-result.md`（末尾の「## 2日目（外を歩く）」の中身を、今日の記録で置き換える）
- 見張りのファイル:
  - W-0: `status-t15-slow.txt`（`-Csv` なし）
  - 歩く間: **前半と後半を通して1つ**。`status-t15-walk.txt` / `t15-walk.csv`。前半と後半は、本体のログと CSV の `protocol` の列（`srt` / `rtmp`）で分ける（下の「区切りの見つけ方」）（1日目は見張りを付け替えたときに約2秒のすきまができたため、今日は付け替えない）
  - OBS のイベント: `obs-t15-walk.txt`
  - 2分ごとのメディアソース・項目・メモリ: `inputs-t15-walk.txt`（`walk-log.ps1`）
- CSV の列は `time,id,state,kbps,protocol,rtt_ms,loss_rate,recv_mbps,scene,owner,stack`（`loss_rate` は %）

## 目的

外を歩いて回線が揺れたときに、次を確かめる。

- kbps の揺れ方。1日目の置いたままと比べて、どれだけ落ち込みが深く、長くなるか
- 揺れのせいで、まちがった切替（映像が乱れていないのに待機へ送る、乱れているのに送らない）が起きないか。正しい切替（送信が止まった、映像が止まった）が、どれだけ起きるか
- SRT の RTT とロス率が、kbps が落ちる前に上がるか（SRT の値を判断に使えるか）
- 新しい警告（OBS が読み切れていません）が出て、消えるか（W-0）

既定の閾値は、`fail_kbps` 800（これ未満が5秒続くと悪い）、`recover_kbps` 1500（これ以上が10秒続くと戻る）。

## 準備

1. ユーザーに確かめてもらう:
   - (a) **新しい `irl-live-keeper.exe`（00:06 の版）で本体を起動し直した**（`config.yaml` は1日目のまま）
   - (b) OBS は起動していて、配信していない。Studio Mode は off
   - (c) iPhone の Larix の SRT と、It's My Live の RTMP の送信先は、1日目の `100.…` の形のまま
   - (d) スマホは2台とも、まだ送っていない
2. `hide-agent-brief.md` の準備の 2〜6 をする（`ws-password.txt`、環境変数、`test-original.json` がないこと、試験だけの MediaMTX を起動して PID を控える、`scenes` の1行目が `test用`、`test setup`）
3. `watch-status.ps1 -Once` で、`warnings` が空であることを確かめる（「URL が古い形」が出ていたら、ユーザーにそのメディアソースの URL を直してもらう）。`.\ctl.exe studio status` が off
4. 本体の MediaMTX と OBS のメモリを控える: `Get-Process mediamtx, obs64, irl-live-keeper | Select-Object Id, ProcessName, WorkingSet64`（準備のあと、前半の終わり、後半の終わり）

## W-0 新しい警告（IRL Pro、Android、LAN、配信はまだ始めない）

1. 見張りを裏で始める: `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-t15-slow.txt -IntervalMs 1000`
2. `.\ctl.exe scene T-B`（`cam-main` だけのシーン。OBS の画面に main が映る）
3. ユーザーに、Android の IRL Pro を **1日目の最初の設定（1080p、ビットレート自動）** にして、`main` へ送ってもらう（URL の欄に `rtmp://<Windows の LAN のアドレス>:1935/live`、キーの欄に `main?user=…&pass=…`、認証の欄は空。1日目の T15-2c と同じ）
4. `main` が OK になってから、最大2分待つ。記録すること:
   - 本体のログの MediaMTX の `reader is too slow` の行の数と、最初の行の時刻
   - 本体のログの `obs reader too slow camera=main count=…` の時刻（最初の `reader is too slow` から何秒後か）
   - `watch-status.ps1 -Once` の `warnings` に、上の文が1行出たか（文をそのまま写す）。`main` の状態は OK のままか（健康に入っていないこと）
   - ユーザーに、OBS の画面で main の映像が乱れているか見てもらう
5. ユーザーに、IRL Pro を **720p・3,000 kbps 固定** に直して送り直してもらう（1日目の2回目と同じ。止めてから設定を変えてよい）
6. 最大2分待つ。記録すること:
   - 最後の `reader is too slow` の時刻と、`obs reader caught up camera=main count=0` の時刻（約60秒後のはず）
   - `warnings` から、その1行が消えたか
   - 1日目は、720p でも送り始めの1秒に `reader is too slow` が3行出た。今日も出たら、その3行で警告がもう一度出る（または続く）ことがある。そのときは、その最後の行から約60秒で消えれば正しい
7. **IRL Pro は 720p・3,000 kbps のまま送り続けてもらう**（歩く間の main）。W-0 の見張りを止める
   - 1080p で `reader is too slow` が出なかったら（PC の調子で出ないこともある）、そのことを記録して次へ進む。設定を変えて出そうとしなくてよい

## 歩く前の用意

1. `.\ctl.exe scene T-A`（main と sub の2台のシーン。sub が悪くなると sub だけを隠す。main も悪くなると待機へ送る）
2. 見張りを裏で始める（歩き終わるまで止めない）:
   - `powershell -ExecutionPolicy Bypass -File .\watch-status.ps1 -Log .\status-t15-walk.txt -Csv .\t15-walk.csv -IntervalMs 1000`
   - `.\ctl.exe watch 3600 .\obs-t15-walk.txt`
   - `powershell -ExecutionPolicy Bypass -File .\walk-log.ps1`（同じフォルダにある。45分の間、2分ごとに `ctl.exe inputs`、`ctl.exe items T-A`、メモリを `inputs-t15-walk.txt` に足す。人が見ていなくても、メディアソースが止まったか、sub が隠れていたかがあとでわかる。ファイルがなければ、ユーザーに Linux 側から写してもらう）
3. ユーザーに伝える:
   - **歩いている間は、スマホの画面を見たり操作したりしない。** 送り方を替えるときなど、操作は立ち止まって安全な所でする
   - 始める前に、iPhone の Wi-Fi を切り、Tailscale がつながっていることを確かめる
   - 外にいる間は、このセッションに知らせなくてよい。前半と後半の区切りは、こちらでログから見つける
   - 帰ってきたら、次の3つを、おおよそでよいので教えてほしい: (1) 家を出た時刻、(2) 映像が乱れそうな所（建物の陰、高架の下、駅や交差点の人の多い所、エレベーターなど）を通った時刻と場所、(3) スマホのアプリの画面で気づいたこと（止まった、エラーが出た、など）。覚えていなければ、なくてよい
   - 帰ってきたら、iPhone の送信を止める前に、このセッションに話しかける（止めたあとの DOWN を、歩いた結果と混ぜないため）
4. ユーザーに、家の中（Wi-Fi を切った状態）で Larix の SRT を送り始めてもらう。`status-t15-walk.txt` で `main` と `sub` がどちらも OK になるのを待つ（最大60秒）。`.\ctl.exe inputs` で2つのソースが `PLAYING` で cursor が進むこと（2回読んで比べる）。`.\ctl.exe items T-A` で `cam-main` と `cam-sub` が両方 `enabled=true`
5. `.\ctl.exe profile` が `IRLKeeper-Test` を出すことを確かめてから `.\ctl.exe stream start`。`streaming=True scene=T-A health=OK scene_cams=[main,sub] owner=none` になること
6. ユーザーに、外に出てよいことを伝える。そのあと、ユーザーが帰ってきて話しかけるまで待つ。歩く間に、自分から見張りのファイルを見にいく必要はない（記録は裏で残る）

## 区切りの見つけ方（帰ってきてから）

- **前半（SRT）**: 本体のログの `[SRT] [conn 100.…] is publishing to path 'live/sub'` から、`[SRT] [conn 100.…] closed` まで。CSV では、`id=sub` で `protocol=srt` の最初の行から最後の行まで
- **替える間**: SRT の `closed` から、`[RTMP] [conn 100.…] is publishing to path 'live/sub'` のあと sub が OK に戻るまで。ここで起きた DOWN と、sub を隠したことは、替えたため（正しい動き）と記録し、前半・後半の数字に入れない
- **後半（RTMP）**: `[RTMP] [conn 100.…] is publishing to path 'live/sub'` から、ユーザーが帰ってきて話しかけた時刻まで（`protocol=rtmp`）
- **途中の切れ**: 前半・後半の中で `closed` → `opened` がまた出たら（回線が切れて、アプリがつなぎ直した）、区切りではなく、その区間の中のできごととして数える。SRT の中で RTMP が出る前の切れか、替えたためかは、`closed` のあとに来たのが SRT か RTMP かで分ける
- **家の中の分**: 前半の初めには、外に出るまでの家の中（キャリア回線）の分が入る。ユーザーが家を出た時刻を教えてくれたら、その前を除いた数字も出す。教えてもらえなければ、前半の最初の2分を除いた数字も並べ、そのことを書く
- 区切りの時刻と、それを決めたログの行を、記録に書く

## 確かめること

| # | 手順 | 記録すること |
|---|---|---|
| W-1 | 前半（Larix の SRT、外を歩いて約10分） | 本体のログの `[SRT] [conn 100.…] opened` / `closed`、`camera changed`、`scene switch`、`camera hidden` / `camera shown`、`media source refreshed`。数字は下の「集計」 |
| W-2 | 後半（It's My Live の RTMP、外を歩いて約10分）: ユーザーが外で立ち止まって、Larix を止め、It's My Live で送り直す（知らせは来ない。上の「区切りの見つけ方」で見つける）。**見張りは付け替えない** | W-1 と同じ（RTT とロス率は出ない） |
| W-3 | まちがった切替の判断 | sub を隠したとき（`camera hidden camera=sub`）と待機への切替（`scene switch`）のたびに、その直前の kbps と、ユーザーに聞いたその時刻の場所（道の様子）を並べる。ユーザーは歩いている間 OBS の画面を見られないので、映像が実際に乱れていたかは、(1) MediaMTX の `closed` や `decode error` の有無、(2) OBS のメディアソースが止まったか（`inputs-t15-walk.txt` の2分ごとの `ctl.exe inputs`。`PLAYING` で cursor が進んでいるか）、(3) kbps の落ち込みの深さと長さ、から判断し、どれで判断したかを書く。「送信が止まった・ほぼ0」は正しい切替。待機への切替が起きたら、main も悪かったか（main の kbps）を必ず書く（sub だけ悪いのに待機へ送ったなら、まちがい）。「800 を少し下回っただけで、5秒の後すぐ戻った」なら、まちがった切替の疑いとして書く |
| W-4 | 長い時間の様子 | 本体のログの MediaMTX の WARN の数（`reader is too slow`、`decode error` など、種類ごと）、`media source refreshed` の数、`obs reader too slow` が出たか、メモリの変化 |

ユーザーが帰ってきて話しかけたら、まず上の3つ（家を出た時刻、道の様子、アプリで気づいたこと）を聞き、それから片づけをして、記録を作る。

## 集計（前半と後半、それぞれ）

CSV の集計は PowerShell で行ってよい。区間は、「区切りの見つけ方」で決めた前半と後半（替える間を除く）。集計に使ったコマンドも記録に書く。

1. 1日目の「数字」の表と同じ列: 行数、kbps の最小 / 平均 / 最大、5%点、800 未満の行、1500 未満の行、LOW と DOWN の回数と長さ、sub を隠した回数と長さ（`camera hidden` から `camera shown` まで。1日目の「待機への切替」にあたる）、待機への切替の回数
   - main も、同じ区間の kbps の最小 / 平均 / 最大、LOW と DOWN の回数を出す（基準。sub と同じ時刻に落ちた所があれば、その時刻を書く）
2. 1日目の「集計に使ったコマンド」と同じ式で: 標準偏差、変動係数、前の秒より40%超落ちた回数、5秒の移動平均の最小
3. 800 未満が続いた区間の長さの一覧（2秒以上のもの）。**3〜4秒で戻った区間**（あと少しで LOW になった所）と、**5秒以上続いた区間**（LOW になった所）を分けて数える
4. 前半（SRT）だけ:
   - `rtt_ms` と `loss_rate` の最小 / 平均 / 最大（1日目の判定区間と並べる）
   - **先に上がるか**: 800 未満が2秒以上続いた区間それぞれについて、その始まりの前の10秒の `rtt_ms` と `loss_rate` の平均と最大を出し、前半全体の平均と比べる。kbps が落ちる何秒前から上がっていたか（上がっていなかったか）を書く
5. 1日目の数字と並べた表（置いたままの SRT・It's My Live と、歩いた SRT・It's My Live の4行）

## 片づけ

`hide-agent-brief.md` の片づけのとおり（`stream stop`、3秒待って `test restore`、試験の MediaMTX を PID で止める、見張りを止める）。ユーザーに、iPhone と Android の送信を止めてよいこと、iPhone の Wi-Fi を戻してよいことを伝える。

## 記録

`t15-result.md` の末尾の「## 2日目（外を歩く）」の中身を、次の形で置き換える（1日目の部分は変えない。パスワードを書かない）。

- まとめの表（W-0〜W-4 の結果を1行ずつ）
- 準備（時刻、PID、メモリの表。歩く間のメモリは `inputs-t15-walk.txt` から）
- 区切りの時刻と、それを決めたログの行。ユーザーから聞いた家を出た時刻と道の様子
- W-0 の流れ（ログの行と時刻、`warnings` の文）
- W-1、W-2 の流れ（`hide-agent-brief.md` の「記録」と同じ形。ユーザーに聞いた道の様子の時刻も入れる）と、上の「集計」の表
- W-3 の判断（切替ごとに1行）
- W-4
- 意見:
  - (a) 既定の閾値（800 / 1500、5秒 / 10秒）は、歩いたときのキャリア回線に合っているか。下げる・上げるなら、どの値にして、どの区間の切替が変わるか（CSV で試算する）
  - (b) SRT と It's My Live の RTMP で、揺れ方や切替の起き方に差があったか
  - (c) SRT の RTT とロス率を、悪くなったことの判断に使えそうか（集計の 4 の結果から）
  - (d) 新しい警告（W-0）の出方と消え方に、困る所があったか（出るのが遅い、消えるのが遅い、文がわかりにくい、など）
- 気になった動き

ユーザーに `t15-result.md` ができたことを伝える。
