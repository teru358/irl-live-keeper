---
id: fb-integ-status-mismatch
title: 0c の結合のテストが一括の実行でまれに status の食い違いで落ちる
status: 設計待ち
priority: 中
kind: 保守
opened: 2026-11-23
closed: null
related: []
---
# [fb-integ-status-mismatch] 0c の結合のテストが一括の実行でまれに status の食い違いで落ちる

**状態**: 設計待ち / **優先**: 中 / **種別**: 保守

## 現象

2026-11-23: go test -race ./... の一括の実行で1回だけ、TestIntegrationFallback の seed 20001 が「1秒の終わり: status が偽の OBS と違う: 世界の program=Dual … status: scene=Standby stack=[Dual]」で落ちた。main でも、32 並列などでも再現しない。T5 で直した静かな時点の判定の隙間（fbSync、inflight、reqSeq、sendMu）の残りか、本体が今のシーンを取り違える道かは未確定

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-23: 起票。
- 2026-11-23: 状態: 実装待ち — 案: HB-INV-03 の直し（7adaf3f。中核ループの observe で、処理した時点を記録する）と同じ考えで、fbCheck の比べる時点を中核ループが SceneChanged を処理した後にそろえる。直す前に、seed 20001 を負荷のもとで数千回回して再現を取る
- 2026-11-23: 状態: 設計待ち — 負荷のもとで再現した。core のテストのバイナリ（-race）を16本並べて TestIntegration を回すと、16本中1本で normal/seed20009 が同じ形（1秒の終わり: status が偽の OBS と違う。世界の program=Grp、status scene=Standby stack=[Grp]、inflight=0）で落ちた。32本×2回（HiddenInvariants と FallbackInvariants）では32本中16〜18本で落ち、status の食い違いのほか、INV-03、INV-04（回復から30秒たっても hidden.json が空でない）、入場の逆順も出た（nproc 32 に対して強い過負荷）。結合のテストの静かな時点の判定に実時間の待ち（1ms の sleep の繰り返し、StopTimeout 300ms など）が残っていて、過負荷で崩れる見立て。本体の不具合かの切り分けは、16本の負荷で seed20009 を回し、静かな時点の判定を実時間に頼らない形を設計してからにする
