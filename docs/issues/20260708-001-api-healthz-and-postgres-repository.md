---
id: ISSUE-001
title: app/api にヘルスチェック専用エンドポイントがなく PostgreSQL にも接続していない(SPEC-001 インフラ前提との乖離)
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-12
specs: [SPEC-001]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-001: app/api にヘルスチェック専用エンドポイントがなく PostgreSQL にも接続していない(SPEC-001 インフラ前提との乖離)

## 1. ユーザー価値への影響(なぜ対応するか)

> **SPEC-001 のインフラ(CloudFront → WAF → ALB → ECS → RDS PostgreSQL)上で app/api を実運用しようとする開発者** の **可用性監視の正確さとデータ永続性** が **app/api 側の実装がインフラ前提に追いついていないことで損なわれる**。

- **影響を受けるユーザー**: SPEC-001 の Terraform を実際に apply し、app/api を ECS 上で動かそうとするこのリポジトリの開発者
- **損なわれる価値**:
  - (a) ヘルスチェックの正確さ・軽量性: ALB Target Group のヘルスチェックに専用エンドポイントがなく `GET /tasks`(業務ロジック)を暫定使用するため、ヘルス監視が業務ロジックの成否に結合し、数秒間隔でリポジトリ全件取得(`FindAll`)が走る。
  - (b) データ永続性・水平スケール時の一貫性: app/api は in-memory リポジトリで RDS に接続しないため、ECS タスクの再起動・入れ替えでデータが失われ、`desired_count` が複数のときタスクごとに別々の in-memory ストアを持ち応答が一貫しない。インフラが用意する RDS PostgreSQL(コスト約 $12/月想定)が未使用のまま配線だけ残る。
- **影響範囲・頻度**: SPEC-001 のインフラを apply して app/api を実際に動かした場合に顕在化する。現状はインフラがまだ plan/実装段階のため潜在的(常時ではなく、実運用移行時に顕在化)。
- **回避策**:
  - (a) あり — ヘルスチェックは暫定で `GET /tasks`(200)を使用(SPEC-001 実装計画の方針で採用済み)。
  - (b) なし — データ永続化は in-memory のままでは代替不能。ただし SPEC-001 のスコープ(インフラのサンプル実装)では app/api の永続化は要求されておらず、現時点では許容された制約。

## 2. 現象(何が起きているか)

### 期待する動作

- ALB Target Group が、業務ロジックに依存しない軽量・認証不要のヘルスチェック専用エンドポイント(例: `GET /healthz`)で ECS タスクの生存を確認できる。
- app/api が、環境変数/シークレット経由で RDS PostgreSQL に接続し、データを永続化する(インフラがタスク定義に配線する DB 接続情報・Secrets Manager ARN を実際に利用する)。

### 実際の動作

- app/api のルートは `/tasks` 系のみで、ヘルスチェック専用エンドポイントが存在しない(`app/api/route/router.go:16-20`)。そのため SPEC-001 実装計画では ALB ヘルスチェックに `GET /tasks`(matcher=200)を暫定採用している(`docs/plans/SPEC-001-plan.md` 方針 5 / L29-32)。
- app/api は in-memory リポジトリ(`app/api/infra/memory/task_repository.go`)を配線しており(`app/api/cmd/api/main.go:39`)、RDS PostgreSQL には接続しない。インフラ側は将来のために DB 接続情報(Secrets Manager ARN / エンドポイント)をタスク定義に env / secret として配線する(`docs/plans/SPEC-001-plan.md` 方針 6 の注記 / L34-35, リスク欄 L156)。

### 再現手順

1. `app/api/route/router.go` を開き、登録ルートを確認する → `POST /tasks` / `GET /tasks` / `GET /tasks/{id}` / `POST /tasks/{id}/start` / `POST /tasks/{id}/complete` のみで `/healthz` 等のヘルスチェック用ルートがないことを確認する。
2. `app/api/cmd/api/main.go` の配線を確認する → 39 行目 `repo := memory.NewTaskRepository()` で in-memory リポジトリを使用し、DB(PostgreSQL)ドライバ・接続コードが存在しないことを確認する。
3. `app/api/infra/` 配下を確認する → `infra/memory` のみで `infra/postgres` 等の DB 実装が存在しないことを確認する。
4. `docs/plans/SPEC-001-plan.md` の方針 5・6 とリスク欄を確認する → ALB ヘルスチェックが `GET /tasks` 暫定であること、RDS は配線のみで app/api が接続しないこと、これらを別 Issue とする旨が記載されていることを確認する。

### 環境・条件

- 対象: `app/api`(Go)。関連: SPEC-001 のインフラ(`app/iac`、`docs/specs/20260708-001-aws-ecs-api-infra.md`)。
- 発見文脈: SPEC-001 の実装計画(`docs/plans/SPEC-001-plan.md`)策定中に、インフラ側の前提と app/api 実装の乖離として判明。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/api/route/router.go:16-20` に登録されているのは `/tasks` 系ルートのみで、ヘルスチェック専用エンドポイントは未定義。
- 事実: `app/api/cmd/api/main.go:39` は `memory.NewTaskRepository()` を配線しており、DB 接続コードは存在しない。同ファイル L23 で `defaultPort = "8080"`(コンテナポート)を定義。
- 事実: `app/api/infra/` 配下は `memory` パッケージ(`infra/memory/task_repository.go`)のみで、PostgreSQL 実装(`infra/postgres` 等)は存在しない。
- 事実: `app/api/domain/task/repository.go` にリポジトリ interface が定義されており(`Save` / `FindByID` / `FindByTitle` / `FindAll`)、in-memory 実装が `var _ task.Repository = (*TaskRepository)(nil)` で契約を満たしている。→ 同 interface を満たす PostgreSQL 実装を追加すれば差し替え可能な設計。
- 事実: SPEC-001 実装計画(`docs/plans/SPEC-001-plan.md`)の方針 5・6 およびリスク欄(L156)で、ヘルスチェックの `GET /tasks` 暫定使用と RDS 未接続が既知の制約として記録され、app/api 側の別 Issue 化が推奨されている。
- 仮説: これらは不具合(退行)ではなく、app/api がインフラ(SPEC-001)より先に in-memory サンプルとして実装され、インフラ前提(専用ヘルスチェック・RDS 永続化)に app/api 側が未対応であることによる仕様上の乖離。

### 根本原因

app/api がサンプル(in-memory・業務ルートのみ)として先行実装され、SPEC-001 が前提とする「軽量なヘルスチェック専用エンドポイント」と「RDS PostgreSQL への接続・永続化」に app/api 側がまだ対応していない(実装の欠落)。設計上はリポジトリ interface が抽象化されているため、差し替え・追加で解消可能。

## 4. 対応(どう解決するか)

### 対応方針

本 Issue はインフラ(SPEC-001)側では対応せず、app/api 側の改善として扱う(SPEC-001 の実装計画では app/api を変更しない方針)。以下 2 点を app/api の変更として計画・実装する(planner による計画化を推奨)。優先度は (1) > (2)。(2) はデータモデルの永続化設計を伴うため、独立した Spec 化も検討する。

1. **ヘルスチェック専用エンドポイントの追加**: app/api に `GET /healthz`(認証不要・軽量・業務ロジック非依存で 200 を返す)を追加し、SPEC-001 のインフラ側 ALB Target Group のヘルスチェックパスを `/tasks` から `/healthz` に切り替える(app/api と app/iac の双方の変更を伴うため、対応時に SPEC-001 側の該当設定更新も連動させる)。
2. **PostgreSQL リポジトリ実装の追加**: `task.Repository` interface を満たす PostgreSQL 実装(`app/api/infra/postgres` 等)を追加し、環境変数/シークレット(DB エンドポイント・Secrets Manager 由来の認証情報)経由で接続する。in-memory 実装は開発/テスト用に残し、配線を環境で切り替えられるようにする。

### 実施内容

- [x] app/api に `GET /healthz` を追加(業務ロジック非依存・認証不要・軽量 200)
- [x] `/healthz` のテストを追加(標準 `go test`、正常系:200 応答)
- [ ] SPEC-001 のインフラ側 ALB ヘルスチェックパスを `/healthz` に切り替え(対応時に別途調整・別作業として連動)
- [x] `task.Repository` を満たす PostgreSQL 実装(`infra/postgres` 等)を追加
- [x] DB 接続情報を環境変数/シークレット経由で受け取る配線を `cmd/api/main.go` に追加(in-memory と切り替え可能に)
- [x] PostgreSQL 実装のテストを追加(interface 越しの fake 差し替え方針は `.claude/rules/testing.md` の api 方針に従う)

### 再発防止

- SPEC(インフラ)とアプリの前提の乖離は、Spec 起票・計画時にトレーサビリティで突き合わせ、乖離を Issue として明示する運用を継続する(本 Issue はその運用で SPEC-001 計画中に検出された)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。SPEC-001(`docs/specs/20260708-001-aws-ecs-api-infra.md`)の実装計画(`docs/plans/SPEC-001-plan.md`)策定中に、インフラ前提と app/api 実装の乖離として判明したものを起票。
- 確認した事実: app/api にヘルスチェック専用エンドポイントがなく(`app/api/route/router.go:16-20`)、ALB ヘルスチェックは `GET /tasks` を暫定使用(計画方針 5 / L29-32)。app/api は in-memory リポジトリで RDS に接続しない(`app/api/cmd/api/main.go:39`、`app/api/infra/memory/task_repository.go`)一方、インフラは DB 接続情報をタスク定義に配線する(計画方針 6 注記 / L34-35、リスク欄 L156)。
- severity は medium と判定。判定根拠: (a) ヘルスチェックは `GET /tasks` で暫定代替でき回避策あり、(b) RDS 未接続(in-memory)は SPEC-001 のスコープ(インフラのサンプル)では要求外で現時点は許容された制約。ただし実運用移行時にはデータ喪失・レプリカ間不整合を招くため low ではなく medium とした(critical/high ではないのはインフラサンプルの目標達成は本 Issue で阻害されないため)。
- 次にやること: planner による app/api 側の対応計画化。まず `GET /healthz` 追加(app/api + app/iac 連動)を優先し、PostgreSQL リポジトリ実装は独立作業(必要なら Spec 化)として扱う。SPEC-001 側 frontmatter の `issues` への相互リンク追記は admin が実施予定。

### 2026-07-12

- 解消確認(app/api 側)。SPEC-011 により Postgres 永続化は完了(`app/api/infra/postgres`、`cmd/api/main.go` が fail-closed で Postgres 接続、in-memory フォールバック廃止)。ヘルスチェック専用エンドポイントは `GET /health`(`app/api/route/health_handler.go`、認証 middleware 外、`{"status":"ok"}`)として実装済み(`/healthz` ではなく `/health` だが目的は同等)。テストは `health_handler_test.go` に存在。
- 残件: SPEC-001 インフラ側 ALB ヘルスチェックパスを `/health` に切り替える作業は app/iac 側の別タスク(本 Issue の app/api スコープは解消)。
