---
id: integ-tempdir-leak
title: 結合のテストの一時フォルダが /tmp に残ることがある
status: 実装待ち
priority: 低
kind: 保守
opened: 2026-11-23
closed: null
related: []
---
# [integ-tempdir-leak] 結合のテストの一時フォルダが /tmp に残ることがある

**状態**: 実装待ち / **優先**: 低 / **種別**: 保守

## 現象

2026-11-22: go test -race ./... のあと、/tmp/TestIntegrationHiddenInvariantsnormalseed… が3個残った。internal/core の結合のテスト（integration_test.go の 623・737 行、integration_fb_test.go の 924 行など）が t.TempDir を使う所で、hidden.json の非同期の書き込みが後片づけと重なった見立て（hideHarness と同じ型。hideHarness は 9940c29 で、書き込みの途中の数を待つ形に直した）

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-23: 起票。
- 2026-11-23: 状態: 実装待ち — 直し方の案: hideHarness と同じく、結合のテストの世界でも hidden.json の書き込みの関数を包み、後片づけの前に書き込みの途中の数が 0 になるのを待つ
