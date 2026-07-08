---
id: SPEC-002
title: Task に優先度(priority)を追加する
status: draft  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-08
updated: 2026-07-08
issues: [ISSUE-008]
supersedes: null
---

# SPEC-002: Task に優先度(priority)を追加する

## 1. ユーザー価値(なぜ作るか)

> **タスク管理の利用者** が **タスクに優先度を設定・並べ替えできるようになり**、**重要なタスクから着手できる** 価値を得る。

- **対象ユーザー**: app/api / app/web のタスク管理機能の利用者
- **解決する課題**: 現状、app/api の Task 集約に優先度の概念が無い。app/web は暫定的にフロント独自の enrichment として `priority`(low/medium/high)を持ち `sortByPriority` を実装しているが、バックエンドが優先度を保持しないため、実データでは優先度が永続化されず並べ替えも機能しない
- **得られる価値**: 優先度がバックエンド契約として一貫し、複数クライアント(web / 将来の他クライアント)で同じ優先度が共有・永続化される
- **価値の検証方法**: Task を priority 付きで作成・更新でき、API レスポンスに priority が含まれ、app/web がフロント独自 enrichment を撤去して API の priority をそのまま利用できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 利用者として、タスク作成時に優先度を指定したい。なぜなら重要なタスクを区別したいから。
- 利用者として、既存タスクの優先度を後から変更したい。なぜなら状況で重要度が変わるから。
- 利用者として、優先度順にタスクを並べたい。なぜなら高優先度から着手したいから。

### 利用フロー

1. ユーザーがタスクを作成する際に priority(low / medium / high)を指定する(未指定時は既定値)
2. システムが priority を保持した Task を返す
3. ユーザーが既存タスクの priority を更新する
4. クライアントが priority で並べ替えて表示する

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: Task 集約が priority(`low` | `medium` | `high`)を属性として保持する
- [ ] R2: タスク作成時に priority を指定できる。未指定時の既定値を定める(候補: `medium`)
- [ ] R3: 既存タスクの priority を変更できる(振る舞いメソッド経由。集約の不変条件を壊さない)
- [ ] R4: API のレスポンス DTO に priority を含める。app/web の DTO スキーマ(zod)と命名・enum 値が一致する
- [ ] R5: 不正な priority 値(enum 外)は集約生成・更新時に弾く(ドメインエラー)

### 非機能要件

- 既存 Task の状態遷移(todo → doing → done)の振る舞いに影響を与えない(priority は状態遷移と直交)
- app/api の DDD レイヤ構成(domain / service / infra / route)と依存方向を維持する

### スコープ外(やらないこと)

- 優先度に基づく自動並べ替え・通知などの高度な機能(本 Spec は属性の保持と CRUD 反映まで)
- app/web 側の改修(本 Spec 完了後、別作業でフロントの独自 enrichment を API 由来へ置き換える)

## 4. 設計(どう実現するか)

### 方針

app/api の Task 集約に priority 値オブジェクト/enum を追加し、既存の非公開フィールド + 振る舞いメソッド方式を踏襲する。作成時に受け取り、専用メソッドで変更する。route / DTO 層で入出力に反映する。

### アーキテクチャ / データ / インターフェース

- **domain**: `Priority` 型(enum: low/medium/high)と検証を追加。`Task` に priority フィールドを持たせ、`ChangePriority` 等の振る舞いメソッドを追加。不正値は sentinel / カスタムエラー
- **infra/memory**: 永続化表現に priority を追加(既存 Repository interface を満たす形)
- **route / DTO**: 作成・更新リクエストと Task レスポンスに priority を追加。app/web の `features/tasks/api/schema.ts` の zod スキーマと命名(snake_case か camelCase か)・enum 値を一致させる
- **既定値**: 未指定時は `medium`(R2。要確定)

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| priority をフロント独自 enrichment のまま維持 | 永続化されず複数クライアントで共有できない。実データで並べ替えが機能しない(現状の課題そのもの) |
| priority を数値(1..5)で保持 | 既存 web が low/medium/high の3値 enum を前提にしており、enum が意味的に明確。将来細分化が必要になったら別途検討 |

## 5. 実装計画

詳細計画は着手時に planner が `docs/plans/SPEC-002-plan.md` に作成する。概要:

- [ ] T1: domain に Priority(enum + 検証)を追加し、Task に priority フィールドと変更メソッドを実装(impl-api)
- [ ] T2: infra/memory の永続化に priority を反映(impl-api)
- [ ] T3: route / DTO の入出力に priority を追加(impl-api)
- [ ] T4: テスト追加(作成既定値・変更・不正値の異常系・境界)(tester)
- [ ] T5: app/web の DTO スキーマと命名/enum を突き合わせて整合(別 Spec/作業でフロント enrichment 撤去)

## 6. 経緯(時系列・追記のみ)

### 2026-07-08

- 初版作成。app/web のフロントエンドアーキテクチャ検討(Bun + Vite + React サンプル実装)の過程で、`features/tasks/domain` が `priority`(low/medium/high)を独自 enrichment として持つ一方、app/api の Task 集約には priority が存在しない乖離が判明。フロント側は暫定的に priority を残す判断をし、バックエンド契約として priority を持たせる本 Spec を起票した。status は draft(要件・既定値・DTO 命名の確定と承認を経てから着手)。
- ISSUE-008 を相互リンク(frontmatter `issues`)。ISSUE-008(app/api の Task 一覧ページネーション不在・web の単一取得契約欠如)は同じ Task 集約の web↔api DTO 契約整備・同じ web ファイル群(client / schema / mocks)で本 Spec と交差する。特に ISSUE-008 で確定する `GET /tasks/{id}` のレスポンス DTO は、本 Spec の priority 反映後は priority を含める必要があるため、着手時に DTO 契約を同期させる。
