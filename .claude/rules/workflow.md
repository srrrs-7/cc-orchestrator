# 開発ワークフロー

作業は必ず Issue を単位として進める。orchestrator(メインの Claude)は各フェーズを対応する subagent に委譲する。

## パイプライン

```
docs/specs(仕様)
  → 1. issue-creator : Issue 起票(docs/issues)
  → 2. planner       : 実装計画の作成(docs/plans)
  → 3. tester        : 受け入れ条件からテストを先に作成(TDD。計画で後付けを指定した場合は 4 の後)
  → 4. impl-web / impl-api / impl-iac : 実装(scope が独立していれば並列可)
  → 5. tester        : テスト実行・不足テストの追加
  → 6. checker       : format / lint / type check
  → 7. review-security / review-performance / review-spec : レビュー(並列)
  → 8. 指摘対応       : Blocker / Major は impl agent に差し戻し、5→7 を再実行
```

- フェーズを飛ばさない。特に 6(checker)が通らない状態で 7(レビュー)に進まない
- レビュー agent はコードを変更しない。修正は必ず impl agent が行う

## Issue

- ファイル名: `docs/issues/ISSUE-NNN-<slug>.md`(NNN は 3 桁連番、slug は英語 kebab-case)
- status 遷移: `open → planned → in-progress → in-review → done`
- テンプレート:

```markdown
---
id: ISSUE-001
title: <日本語タイトル>
status: open
created: YYYY-MM-DD
spec: docs/specs/<file>.md
scope: [web, api, iac]   # 該当するものだけ
plan: null               # planner が docs/plans のパスを設定
---

## 概要

## 背景 / 目的

## 受け入れ条件
- [ ] <検証可能な形で書く>

## スコープ外

## 依存関係
- <先行して完了が必要な Issue があれば ISSUE-NNN を列挙>
```

## Plan

- ファイル名: `docs/plans/ISSUE-NNN-plan.md`(対象 Issue と同じ NNN)
- 必須セクション:
  - **方針**: 採用するアプローチと、退けた代替案の理由
  - **変更ファイル**: stack ごとの追加・変更ファイル一覧
  - **手順**: どの agent が何をどの順で行うか(並列可能な箇所を明示)
  - **テスト戦略**: 先行作成(TDD)か後付けか、何をどのレベルでテストするか
  - **リスク / 未確定事項**
