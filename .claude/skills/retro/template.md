---
id: RETRO-NNN
title: <摩擦の一行要約>
status: open  # open | addressed | wontfix
severity: medium  # high(頻発・手戻り大 / タスクをブロック) | medium(回避したが非効率) | low(軽微)
source: <摩擦を surface した主体。agent名(例 impl-api) | skill名(例 spec) | admin>
phase: orchestration  # spec | plan | test | impl | check | review | orchestration | other
target: <改善対象の .claude アセット。例 rules/db.md | agents/impl-api.md | skills/spec | CLAUDE.md | orchestration>
created: YYYY-MM-DD
updated: YYYY-MM-DD
synthesis:   # 統括レポートで扱われたら RETROSUM-NNN を記入(未統括なら空)
tags: []  # 自由タグ(例: [ambiguous-rule, missing-command, wrong-model])
---

# RETRO-NNN: <摩擦の一行要約>

## 1. 遭遇した課題(何が摩擦だったか)

> **<どのアセット>** の **<どの記述 / 欠落>** が原因で、**<誰(agent/skill)>** が **<どう詰まったか>**。

- **具体的に何が起きたか**:
- **どのアセットの問題か**: (曖昧 / 欠落 / 誤り / 矛盾 のどれか)

## 2. 影響(タスクにどう響いたか)

- **症状**: (手戻り / 誤った前提 / ブロック / 非効率 / 再実行)
- **コスト**: (可能なら定量。例: checker を 3 回リトライ、誤配置で 1 フェーズ戻り)

## 3. 改善提案(どう直すか)

対象アセットへの**具体的で実行可能な**提案を書く。「rules/db.md にXを追記」「impl-api の報告形式に Y を追加」のように、どのファイルに何を変えるかまで踏み込む。断定できない場合は「仮説:」を付ける。

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: (ファイル:行 / subagent 報告の抜粋 / 実行ログ)
- **再現条件**: (どのタスク・どの stack・どの入力で起きたか)

## 5. 経緯(時系列・追記のみ)

### YYYY-MM-DD

- 記録。<どのタスクの検収中に、どこで摩擦を見つけたか>
