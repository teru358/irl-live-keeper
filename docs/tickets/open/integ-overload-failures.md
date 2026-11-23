---
id: integ-overload-failures
title: 結合のテストが強い過負荷（32本×2回）で INV-03・INV-04・入場の逆順で落ちる
status: 設計待ち
priority: 低
kind: 保守
opened: 2026-11-23
closed: null
related: []
---
# [integ-overload-failures] 結合のテストが強い過負荷（32本×2回）で INV-03・INV-04・入場の逆順で落ちる

**状態**: 設計待ち / **優先**: 低 / **種別**: 保守

## 現象

2026-11-23: core のテストのバイナリ（-race）を32本×-test.count=2 で並べ、TestIntegrationHiddenInvariants と FallbackInvariants を回すと（nproc 32）、status の食い違いを直したあとも、INV-03（seed 1132: 人の true の印のあとの次の false の要求）、INV-04（seed 1047: 回復のあとも hidden.json に未決着の記録が残る）、入場の逆順（seed 25002: Nest の false が mode=3 で未適用のまま Nest へ切り替えた）が出る。16本×2回では0回。実時間の待ち（StopTimeout 300ms、HiddenRetry 5ms、settle の静かさ）が過負荷で崩れる見立てで、本体の不具合かは未確定（Codex の記録の読み: scratchpad の out/fbmis.md）

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-23: 起票。
