# 0c（シーンごとの逃がし先）Linux の OBS での実機の確かめ — 結果

日付: 2026-11-22。環境: M-13 の Linux 試験環境（OBS + Xvfb、MediaMTX、ffmpeg のカメラ3台、中継で帯域を絞る）。
カメラ main は standby、sub と hand は hide。`obs.scene_fallbacks: [{scene: S1, fallback: S2}]`（S2 は共通の待機 `Standby`）。
試験のシーン: `S1`（main_src + sub_src）、`S2`（sub_src）、`IMG`（色のソース）。既存の `B`（main_src）を L-9 で使った。
カメラを悪くするのは `cam_stop`（DOWN）。LOW（`rate`）は使っていない（reason は down のみ確かめた）。

## 結果: 13 項目すべて合格

| # | 結果 | 記録（ログの要点 / status） |
|---|---|---|
| L-1 | 合格 | `scene switch S1→S2 reason=down camera=main purpose=AutoFail`、owner auto、return S1、stack [S1]。S1 の sub_src は ON のまま |
| L-2 | 合格 | `S2→Standby reason=down camera=sub`、stack [S1, S2]。switch の約 0.04 秒後に `camera hidden camera=sub items=5`（待機に着いてから隠す）。S1/S2 の sub_src は OFF |
| L-3 | 合格 | `camera shown camera=sub` と同時刻に `Standby→S2 reason=ok purpose=AutoRecover`、stack [S1]。sub_src は ON |
| L-4 | 合格 | `S2→S1 reason=ok camera=main`、`standby changed owner=none`、stack [] |
| L-5 | 合格 | 待機 [S1, S2] から main と sub を同時に復旧 → `camera shown` の 1 秒後に `Standby→S1`（S2 を経由しない）、owner none、stack [] |
| L-6 | 合格 | S1 で同時に DOWN → `S1→Standby reason=down camera=main`、直後に `camera hidden camera=sub`。stack [S1, S2]（S2 には入らない） |
| L-7 | 合格 | 待機 [S1, S2]、sub は DOWN のまま main だけ復旧 → `Standby→S1`、stack []。`oc items S1` は main_src ON / sub_src OFF（sub は待機に着いたとき隠れていた） |
| L-8 | 合格 | S1 で sub だけ DOWN → シーンは変わらず `camera hidden camera=sub`、stack []。復旧で `camera shown` |
| L-9 | 合格 | S2・[S1] で `oc scene B` → まず `standby changed owner=none`（stack 空）、次に B の main が DOWN なので `B→Standby purpose=AutoFail`、return B、stack [B] |
| L-10 | 合格 | `standby/on` → `S2→Standby reason=command purpose=StandbyOn`、owner manual、return S1、stack []。`standby/off` → `Standby→S2 purpose=StandbyOffMonitored`、owner auto、stack [S1] |
| L-11 | 合格 | RemoveScene S1 → `standby changed owner=none`、stack []、S2 に残る。S1 を作り直すと warnings の行が消えた |
| L-12 | 合格 | `{S2: IMG}` を足して L-1 → L-2: `S2→IMG reason=down camera=sub`、stack [S1, S2]、sub は隠れた（待機ではなく IMG）。両方復旧で `IMG→S1` |
| L-13 | 合格 | S1 の行を `fallback: NOPE` にすると warnings に「scene_fallbacks の 1 行目: fallback「NOPE」が OBS にないため、共通の待機シーンへ逃がします」。main を DOWN にすると `S1→Standby`（共通の待機）、stack [S1] |

## 期待と違ったこと

なし。

## 気になった動き（不合格ではない）

- L-13 の「S1 の行を足す」は、S1 が既に別の行にあるので文字どおりだと同じ scene の重複になる。実際に重複させて起動すると、本体は起動せず `obs.scene_fallbacks[1].scene: 2 行目: "S1" は 1 行目と重複しています…` で終了した（検証どおり）。警告の確認は S1 の行の fallback を NOPE に書き換えて行った。
- L-11 の RemoveScene の直後に、ログに `scene changed by user from=S2 to=S2` が出た（OBS が現在のシーンの変更イベントを再送したためと思われる。状態は期待どおりで害はない）。
- L-4 の直後の status に一時的に `sub_src が映っていない可能性があります` の警告が出た（次の読みで消えた）。
- 本体の再起動直後は（カメラがまだ来ていないため）配信中なら監視対象シーンから待機や IMG へ落ちる。カメラが OK になって `oc scene S1` で戻した。
- 待機のとき main（standby 種別）は隠さず、sub（hide 種別）だけが隠れる（0b のとおり）。
- 試験の時間は DOWN の判定（down_sec 2 秒）と復旧（recover_sec 10 秒）で決まる。

## 後片付け

config.yaml を元に戻し、試験のシーン（S1、S2、IMG）を削除し、起動した本体・ffmpeg・中継・OBS・Xvfb(:77)・MediaMTX を PID で止めた。Xvfb :99 とほかのユーザーのプロセスには触れていない。
