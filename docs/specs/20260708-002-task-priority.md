---
id: SPEC-002
title: Task に優先度(priority)を追加する
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-08
updated: 2026-07-09
issues: [ISSUE-008, ISSUE-010]
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

- [x] R1: Task 集約が priority(`low` | `medium` | `high`)を属性として保持する
- [x] R2: タスク作成時に priority を指定できる。未指定時の既定値は `medium` とする
- [x] R3: 既存タスクの priority を変更できる(振る舞いメソッド経由。集約の不変条件を壊さない)
- [x] R4: API のレスポンス DTO に priority を含める。app/web の DTO スキーマ(zod)と命名・enum 値が一致する
- [x] R5: 不正な priority 値(enum 外)は集約生成・更新時に弾く(ドメインエラー)

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
- **route / DTO**: 作成・更新リクエストと **すべての** Task レスポンス DTO(list / get / create / 状態遷移)に priority を追加。app/web の `features/tasks/api/schema.ts` の zod スキーマと命名を一致させる(**snake_case の `priority`**。既存 DTO の `created_at` 等の命名に合わせる)。enum 値は `low` / `medium` / `high`(web と一致)
- **既定値**: 未指定時は `medium`(確定)

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| priority をフロント独自 enrichment のまま維持 | 永続化されず複数クライアントで共有できない。実データで並べ替えが機能しない(現状の課題そのもの) |
| priority を数値(1..5)で保持 | 既存 web が low/medium/high の3値 enum を前提にしており、enum が意味的に明確。将来細分化が必要になったら別途検討 |

## 5. 実装計画

詳細計画は `docs/plans/SPEC-002-plan.md`(planner 作成済み)が正。概要:

- [x] T1: domain に Priority(enum + 検証)を追加し、Task に priority フィールドと変更メソッドを実装(impl-api)
- [x] T2: infra/memory の永続化に priority を反映(impl-api)
- [x] T3: route / DTO の入出力に priority を追加(`POST /tasks/{id}/priority` を含む)(impl-api)
- [x] T4: テスト追加(作成既定値・変更・不正値の異常系・境界)(tester)
- [ ] T5: app/web の DTO スキーマと命名/enum を突き合わせて整合(別作業 / SPEC-003 でフロント enrichment 撤去)

## 6. 経緯(時系列・追記のみ)

### 2026-07-08

- 初版作成。app/web のフロントエンドアーキテクチャ検討(Bun + Vite + React サンプル実装)の過程で、`features/tasks/domain` が `priority`(low/medium/high)を独自 enrichment として持つ一方、app/api の Task 集約には priority が存在しない乖離が判明。フロント側は暫定的に priority を残す判断をし、バックエンド契約として priority を持たせる本 Spec を起票した。status は draft(要件・既定値・DTO 命名の確定と承認を経てから着手)。
- ISSUE-008 を相互リンク(frontmatter `issues`)。ISSUE-008(app/api の Task 一覧ページネーション不在・web の単一取得契約欠如)は同じ Task 集約の web↔api DTO 契約整備・同じ web ファイル群(client / schema / mocks)で本 Spec と交差する。特に ISSUE-008 で確定する `GET /tasks/{id}` のレスポンス DTO は、本 Spec の priority 反映後は priority を含める必要があるため、着手時に DTO 契約を同期させる。
- ユーザー承認を得て status を `approved` に更新。SPEC-003(Go⇄TS 型共有基盤 / B2)の着手前ゲート判断で、本 Spec を **SPEC-003 の先行前提**として実装する順序が確定した(D1: priority を Go に追加してから OpenAPI を生成し、web が実契約で priority を利用する)。あわせて次を決定: **R2 の既定値 = `medium`**(web の CreateTaskForm 既定と一致)/ **R3(既存タスクの priority 変更)を今回スコープに含める**(専用の振る舞いメソッド + route を実装し、Task の priority 機能を API として完結させる。web は当面この変更 API を未使用)。DTO の命名/casing と priority 変更 route の形は Go 既存の規約・遷移エンドポイント様式に合わせる(planner が確定)。planner に実装計画(`docs/plans/SPEC-002-plan.md`)の作成を委譲する。

### 2026-07-08(承認)

- status を draft → approved に更新し、未確定だった項目を確定した: (1) タスク作成時に priority 未指定の場合の既定値は `medium`、(2) API の DTO はすべて snake_case の `priority` フィールドで統一(既存 `created_at` 等の命名規約に合わせる)、(3) priority の enum 値は `low` / `medium` / `high`(app/web の zod スキーマと一致)。
- 上記確定を前提に、planner が `docs/plans/SPEC-002-plan.md` を作成し、tester(TDD)→ impl-api → checker → review の pipeline で実装に着手する。priority はすべての Task レスポンス DTO(list / get / create / 状態遷移)に含める(ISSUE-008 の単一取得 DTO とも同期)。app/web 側の enrichment 撤去は本 Spec 完了後の別作業(スコープ外のまま)。

### 2026-07-09(レビュー派生の課題起票)

- 本 Spec のセキュリティレビューで検出した app/api の横断的堅牢化不足を **ISSUE-010** として起票し、frontmatter `issues` に相互リンクした。内容: 全 HTTP ハンドラでリクエストボディサイズ上限(`http.MaxBytesReader`)が無く、`http.Server` に `ReadTimeout` / `WriteTimeout` 等の防御設定も無い(緩やかな DoS への defense-in-depth 不足)。**これは SPEC-002 の差分起因ではなく既存の全ハンドラに以前から存在する課題**で、SPEC-002 で新設した `changePriority` も同じパターンを踏襲しているため影響面の一つに含まれる。SPEC-002 のスコープ外として切り出し、対応可否・タイミングは ISSUE-010 側で追跡する(本 Spec の完了条件には含めない)。severity は low(サンプル・認証なしで実害限定、退行ではない)。

### 2026-07-09(実装完了・レビュー通過 → done)

- pipeline を完走: planner(契約確定)→ tester(TDD。先行テストで RED を確認)→ impl-api(緑化)→ checker(`make check` = fmt-check + lint + vet + build + test すべて緑、golangci-lint 0 issues)→ review×3(security / performance / spec conformance)。
- 実装(`domain/task/priority.go`・`task.go`・`errors.go`、`service/{dto.go,task_service.go}`、`infra/memory/task_repository.go`、`route/{task_handler.go,router.go,response.go}`。priority 変更は `POST /tasks/{id}/priority`)は R1〜R5 と非機能(priority と状態遷移の直交)をすべて満たし、テストで検証済み。std-lib のみ・外部依存追加なし。
- レビュー結果: **Blocker 0 / Major 0**。Minor 2 件を処理 —(a)仕様準拠レビューが挙げた `priority: null` 明示送信の境界テストを tester が追加(create → medium / change → 400)、(b)セキュリティレビューが挙げた HTTP ボディ上限・Server タイムアウト不足は SPEC-002 起因でない既存横断課題として **ISSUE-010** に切り出し(本 Spec の完了条件外)。パフォーマンスレビューは指摘 0(既存パターン踏襲、劣化なし)。
- 「価値の検証方法」のうち **app/api 側の契約提供(priority 付き作成・変更、全レスポンス DTO への snake_case `priority` 反映、web の zod との命名・enum 一致)を検証済み**。残る「app/web が独自 enrichment を撤去して API priority を消費」(T5)は §スコープ外で **SPEC-003(型共有基盤 / T4)に委譲**。SPEC-003 の着手解除条件(SPEC-002 完了 = checker 緑 + レビュー通過)を満たしたため、本 Spec の app/api スコープを完了として **status を done** に更新する(web 追従は SPEC-003 で継続。T5 は未チェックのまま SPEC-003 で消化)。
