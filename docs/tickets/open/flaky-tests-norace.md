---
id: flaky-tests-norace
title: 揺れる既存のテスト3つ（obs の Studio Mode の境界、core の保存）
status: 実装待ち
priority: 中
kind: 保守
opened: 2026-11-22
closed: null
related: []
---
# [flaky-tests-norace] -race なしで落ちる既存のテスト2つ（obs の Studio Mode の境界、core の保存の再試行）

**状態**: 実装待ち / **優先**: 中 / **種別**: 保守

## 現象

go test を -race なしで走らせると、internal/obs の TestStudioModeChangedBeforeAndAfterSnapshot（順序: [connected studio=true changed=true changed=false]。10回中7〜10回）と internal/core の TestAsyncPersistFailureRetries（「書けない警告」にならない）が落ちる。-race を付けると通る。fa709ab（F-9 の前）でも同じ。本体の不具合ではなく、テストの同期の問題の見立て（sol）: obs は偽の OBS の hook が、Studio Mode の読み取りの境界 mark() の前に false のイベントを送れる。core は「最初の書き込みだけ成功」の注入が、非同期の保存と隠す前の同期の保存の順に依存する

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-22: 起票。
- 2026-11-22: 状態: 実装待ち — 直し方の案: obs は境界が確定してから false を送る同期点を足す（待ちを伸ばしても直らない）。core は保存の内容か明示した同期点で「隠す前の保存は成功、隠したあとの保存から失敗」を指定し、再試行は呼び出しの数と成功を明示的に待つ
- 2026-11-22: 題名: 揺れる既存のテスト3つ（obs の Studio Mode の境界、core の保存） — 2026-11-22 追加: internal/core の TestRunReturnsWhenAsyncWriteIsStuck も、-race 付きのパッケージ一括の実行で TempDir の RemoveAll（directory not empty）で落ちることがある（main でも。単独では通る）。止まった非同期の書き込みが、テストの後片づけのあとにファイルを書いている見立て
