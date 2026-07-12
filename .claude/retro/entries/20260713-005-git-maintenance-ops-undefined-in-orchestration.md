---
id: RETRO-005
title: git 維持作業(main 取り込みマージ・競合解消)の役割とフローが orchestration に未定義
status: addressed
severity: medium
source: admin
phase: orchestration
target: rules/orchestration.md  # 併記: rules/workflow.md(維持作業の軽量フロー欠落)
created: 2026-07-13
updated: 2026-07-13
synthesis: RETROSUM-001
tags: [missing-role, whitelist-gap, git-ops, ambiguous-rule]
---

# RETRO-005: git 維持作業(main 取り込みマージ・競合解消)の役割とフローが orchestration に未定義

## 1. 遭遇した課題(何が摩擦だったか)

> **rules/orchestration.md(ホワイトリスト / 割り振り表)と rules/workflow.md(パイプライン)** に **git 維持作業(ブランチへの main 取り込み・競合解消)の役割定義とフローが欠落**しており、**admin** が **複数の判断を即興で解釈しながら進めることになった**。

- **具体的に何が起きたか**: ユーザーの `git pull origin main`(feature ブランチへの origin/main 取り込み)が競合で失敗し、その完了を支援するタスクで以下の判断がすべてルールから読み取れず、admin がその場で解釈した:
  1. **`git merge` / `git config --unset` の実行可否**: whitelist は「参照系コマンド」と「commit / push(ユーザーが指示したときのみ)」のみ列挙。merge は状態変更系だが未列挙で、「列挙されていない作業はすべて委譲対象」に従うと委譲すべきだが、**git 操作を担当する agent が割り振り表に存在しない**
  2. **競合解消の委譲単位**: `app/` 配下の編集(go.mod 3 ファイル)は禁止事項により admin 不可。ファイル所有で切って impl-api / impl-auth / impl-db に 3 並列委譲したが、この切り方(stack 所有 / 単一 agent 一括 / そもそも委譲対象か)はルールに定義がない
  3. **パイプラインの適用範囲**: workflow.md のパイプラインは Spec / Issue 起点の機能開発・不具合対応前提。マージ取り込みという維持作業に planner / tester / review-* をどこまで適用すべきか不明。今回は「impl(競合解消)→ checker → commit」の軽量フローを即興で構成した
  4. **merge commit の授権解釈**: 「commit はユーザーが指示したときのみ」に対し、「ユーザー自身が実行した `git pull` の完了 = merge commit の指示とみなす」という解釈を admin が独自に行った
- **どのアセットの問題か**: 欠落(維持系 git 作業という反復タスク類型に対する役割・フローの定義がない)

## 2. 影響(タスクにどう響いたか)

- **症状**: 非効率(判断ポイントごとの即興解釈)+ 将来の一貫性リスク(session ごとに扱いがぶれ得る)。今回はブロック・手戻りなし
- **コスト**: 今回は解釈コストのみ。ただし dependabot が稼働しており(今回の競合自体が dependabot bump 由来)、**main → feature の取り込みマージと go.mod 競合は今後も反復することがほぼ確実**で、都度同じ即興判断が必要になる

## 3. 改善提案(どう直すか)

1. **rules/orchestration.md のホワイトリストに追記**: 「ユーザーが明示的に指示・実行した git 操作(pull / merge / branch 操作等)の完了と、それに付随する git 設定の修正。ただし競合解消が `app/` 配下の編集に及ぶ場合、解消の編集はファイル所有 stack の impl agent に委譲する(例: `app/api/go.mod` → impl-api、`app/migrator/**` → impl-db)」
2. **rules/workflow.md に維持作業の軽量フローを追記**: 「依存 bump / main 取り込みマージ等の維持作業: admin が merge を実行 → 競合解消を stack 所有 impl に委譲(解消方針は admin が決めて指示)→ checker(対象 stack の `make check`)→ merge commit。ドメイン知識を要する実質的な競合(コードロジックの衝突)のみ planner / review-* を挟む」。tester を省略できる条件(`make check` が test を含む Go スタック等)も明記する
3. 仮説: 上記 2 件で足りない場合、「git-ops」を担う agent の新設も選択肢だが、頻度に対して過剰の可能性が高い。まず whitelist / workflow の追記で様子を見るのが妥当

## 4. 根拠 / 再現(なぜそう言えるか)

- **根拠**: 本セッションの実行ログ。`git merge origin/main` → `app/{api,auth,migrator}/go.mod` の 3 競合 → impl-api / impl-auth / impl-db へ 3 並列委譲 → checker(3 スタック `make check` 全緑)→ merge commit `85d6e7e`。whitelist(rules/orchestration.md「admin が直接行ってよいこと」)に merge の記載なし、割り振り表に git 操作の行なし
- **再現条件**: feature ブランチ運用中に origin/main が進み(dependabot merge 等)、取り込みマージで `app/` 配下のファイルが競合したとき

## 5. 経緯(時系列・追記のみ)

### 2026-07-13

- 記録。`git pull origin main` 失敗の支援タスク(2026-07-12〜13)完了後の振り返りで、実行中に即興解釈した判断ポイント 4 件を摩擦として吸い上げた

### 2026-07-13(addressed へ遷移)

- RETROSUM-001 提案 2 として統括・適用(コミット `9a9de7e`): `rules/orchestration.md` のホワイトリストに「ユーザーが明示的に指示・実行した git 操作の完了 + 付随する git 設定の修正。`app/` 配下の競合解消はファイル所有 stack の impl に委譲」を追記、`rules/workflow.md` に「維持作業(依存 bump / main 取り込みマージ)」の軽量フロー(admin merge → impl 委譲 → checker → merge commit。Go スタックは tester 省略可)を新設。改善提案 3 の「git-ops agent 新設」仮説は頻度に対し過剰として不採用。
