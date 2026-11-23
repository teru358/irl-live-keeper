---
id: integ-tempdir-leak
title: 結合のテストの一時フォルダが /tmp に残ることがある
status: 是正済
priority: 低
kind: 保守
opened: 2026-11-23
closed: 2026-11-23
related: []
---
# [integ-tempdir-leak] 結合のテストの一時フォルダが /tmp に残ることがある

**状態**: 是正済 / **優先**: 低 / **種別**: 保守

## 現象

2026-11-22: go test -race ./... のあと、/tmp/TestIntegrationHiddenInvariantsnormalseed… が3個残った。internal/core の結合のテスト（integration_test.go の 623・737 行、integration_fb_test.go の 924 行など）が t.TempDir を使う所で、hidden.json の非同期の書き込みが後片づけと重なった見立て（hideHarness と同じ型。hideHarness は 9940c29 で、書き込みの途中の数を待つ形に直した）

## 原因

## 処置案・裁定

## 修正内容

- 2026-11-23: hideHarness の待ち（書き込みの途中の数が 0 で20ms 新しい書き込みがない）を trackedHiddenPath（helpers_test.go）にまとめ、0b・0c の結合のテストの4か所（runSeed、TestIntegrationWaitingHiddenFixedSequence、runSeedFB、newFBFixed）で Deps.HiddenWrite を包んで使う。確かめた結果: 32本×2回の -race の負荷で、直す前は一時フォルダが5個残り、直したあとは0個。go test -race ./... は通る

## 経緯

- 2026-11-23: 起票。
- 2026-11-23: 状態: 実装待ち — 直し方の案: hideHarness と同じく、結合のテストの世界でも hidden.json の書き込みの関数を包み、後片づけの前に書き込みの途中の数が 0 になるのを待つ
- 2026-11-23: 是正内容 — hideHarness の待ち（書き込みの途中の数が 0 で20ms 新しい書き込みがない）を trackedHiddenPath（helpers_test.go）にまとめ、0b・0c の結合のテストの4か所（runSeed、TestIntegrationWaitingHiddenFixedSequence、runSeedFB、newFBFixed）で Deps.HiddenWrite を包んで使う。確かめた結果: 32本×2回の -race の負荷で、直す前は一時フォルダが5個残り、直したあとは0個。go test -race ./... は通る
