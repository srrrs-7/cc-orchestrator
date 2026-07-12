---
id: RETRO-001
title: テスト戦略変更の Spec で、テストダブルを「DB 観測可能性」で先に分類しないと要件・例外条項が狭くなり手戻りする
status: open  # open | addressed | wontfix
severity: medium  # high(頻発・手戻り大 / タスクをブロック) | medium(回避したが非効率) | low(軽微)
source: admin
phase: spec
target: skills/spec
created: 2026-07-11
updated: 2026-07-11
synthesis: RETROSUM-001
tags: [spec-authoring, test-strategy, narrow-requirement, rework]
---

# RETRO-001: テスト戦略変更の Spec で、テストダブルを「DB 観測可能性」で先に分類しないと要件・例外条項が狭くなり手戻りする

## 1. 遭遇した課題(何が摩擦だったか)

> **skills/spec の起票プロセス**(および amend 前の SPEC-013 R2)の **「テストダブルを DB 観測可能性で分類する観点の欠落」** が原因で、**admin(Spec 起票)と planner** が **狭すぎる要件で計画を立て、レビュー段階での逸脱指摘 → Spec 改訂 → 計画差し戻しの手戻り**に至った。

- **具体的に何が起きたか**: SPEC-013(テスト実 DB 一本化)の起票時、admin が R2 で手書きダブル(`fakeRepository` / `stubListPageRepository` / `readerSpy` / `writerSpy` / `failingRepository` / `dbErrorRepository` / …)を一括で「置換対象」として列挙し、例外条項を「実 DB では現実的に誘発できない**障害系**」のみに狭く書いた。しかし `readerSpy`/`writerSpy` は service 層のポート振り分け(SPEC-010)や narrow-port の compile-time 型証明という「実 DB(`reader==writer` の単一プール)では**原理的に観測できない性質**」を検証しており、実リポジトリへの単純置換ではカバレッジが後退する(かつ planner 原案の spy は背後で in-memory fake を backing に温存しており、R2 の核心=DB 代替の排除にも反していた)。
- **どのアセットの問題か**: 欠落(skills/spec に、テスト戦略変更 Spec で「消す/残すダブル」を要件化する際の分類観点が無い)。

## 2. 影響(タスクにどう響いたか)

- **症状**: 手戻り(Spec 改訂 + planner 差し戻し)。狭い例外条項のまま計画が進み、レビュー段階でユーザーが「R2 が明示列挙した `readerSpy`/`writerSpy` を計画が『残す』のは Spec 逸脱では」と指摘して初めて是正された。
- **コスト**: 中。planner への裁定差し戻し 1 回(§4 判定表 + §6 の改訂)、Spec の R2 例外条項を 2 類型へ拡張 + 「in-memory fake backing 禁止」明文化の改訂、価値の検証方法の追記。実装フェーズ着手前に是正できたため実装のやり直しは無かったが、計画確定が 1 往復遅れた。

## 3. 改善提案(どう直すか)

- **skills/spec**: テンプレートまたは手順に、テスト戦略・テスト基盤を変更する Spec 向けのチェック項目を 1 つ追加する。「テストダブル(fake/stub/spy)を remove/replace の要件にする前に、各ダブルが**検証している性質が実 DB で観測可能か**で分類する。DB 非観測な seam(ポート振り分け・型証明・実装が到達し得ないエラー分岐)を検証するダブルは、最初から**例外カテゴリ**に置く(単純置換の対象にしない)」。
- **仮説**: あわせて `.claude/rules/testing.md` の「テストの実 DB 一本化(SPEC-013)」節にある「残してよいダブルの 3 類型」を、Spec 起票時のチェックリストとして相互参照できるようにすると、次回同種の Spec で分類漏れを防げる。適用可否は retro-synthesizer / admin の判断。

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: `docs/specs/20260711-013-unify-tests-real-db-test-databases.md` §6「2026-07-11(追記: R2 例外条項の拡張 — reader/writer spy の裁定)」に手戻りの経緯が記録されている。改訂前の R2 は例外を「障害系」限定で記述していた。planner の裁定回答(reader/writer spy が実 DB 非観測な service→ポート seam を検証する固有カバレッジである、という技術説明)が是正の根拠。
- **再現条件**: 既存のモック/ダブルを実基盤(実 DB 等)へ置換する種類のテスト戦略変更 Spec を起票し、ダブルの中に「実基盤では観測できない実装 seam を検証するもの」が混在しているとき。

## 5. 経緯(時系列・追記のみ)

### 2026-07-11

- 記録。SPEC-013(テスト実 DB 一本化)の planner 計画レビューの検収中に発見。ユーザー指摘 → admin 裁定(readerSpy/writerSpy を実 postgres Reader/Writer をラップする計数デコレータ化 + R2 例外条項を 2 類型へ拡張)で是正済みだが、Spec 起票時に分類観点があれば手戻りを避けられた摩擦として記録する。
