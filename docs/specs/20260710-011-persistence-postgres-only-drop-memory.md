---
id: SPEC-011
title: 永続化を Postgres 一本化し infra/memory を完全削除(テストも Postgres 化・DI 差し替え耐性の検証)
status: done
created: 2026-07-10
updated: 2026-07-11
issues: []
supersedes: null
---

# SPEC-011: 永続化を Postgres 一本化し infra/memory を完全削除(テストも Postgres 化・DI 差し替え耐性の検証)

## 1. ユーザー価値(なぜ作るか)

> **cc-orchestrator の開発者・保守者** が **単一の永続化経路(Postgres)だけを保守し、本番と同じ実 DB でテストできる** ようになり、**保守コストの削減と、本番挙動との乖離が減ることによる信頼性** を得る。

- **対象ユーザー**: 本リポジトリの開発者・保守者(および subagent パイプライン)
- **解決する課題**: 現状、各集約のリポジトリは `infra/memory` と `infra/postgres` の 2 実装を持ち、
  - 保守コストが二重化している(同一ポートを満たす 2 つの実装・2 系統のテスト経路)
  - 実行時に `APP_ENV=local/test` の in-memory フォールバックがあり、テストの多くが本番と異なる memory 実装に対して回っている(本番 Postgres との挙動乖離のリスク)
- **得られる価値**:
  - 永続化実装が 1 系統になり、削除により保守面が縮小する
  - テストが本番と同じ Postgres に対して回り、乖離由来の不具合が減る
  - 永続化層の dependency inversion が「store 差し替え(postgres→mysql 等)で service 層を壊さない」ことが検証・文書化され、将来の store 変更が安全になる
- **価値の検証方法**: 次をすべて満たしたら成功とみなす
  1. `infra/memory`(api・auth 双方)が repo から消え、api/auth が Postgres 経路のみで起動・テストされる
  2. 既存の外部契約(HTTP API / DTO / OpenAPI・ドメインポートのシグネチャ・DB スキーマ / マイグレーション / sqlc 生成結果)が不変のまま、既存の振る舞い契約テスト(`Run<集約>RepositoryContract`)が緑
  3. `service` / `route` / `domain` が store 固有型(`pgx` / `database/sql` / `pgconn` / `sqlcgen`)に非依存であることが確認され、mysql 差し替え時の変更面が `infra/postgres` 内に限定されることが文書化される

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 保守者として、リポジトリ層のバグを 1 実装だけ直せばよくしたい。なぜなら memory / postgres の二重実装は変更の取りこぼしと二度手間を生むから。
- テスト作成者として、テストを本番と同じ Postgres に対して書きたい。なぜなら memory と Postgres で挙動(UNIQUE 制約・トランザクション・原子性)が異なり、memory 緑でも本番で壊れうるから。
- 将来 store を差し替える開発者として、`infra/postgres` を新実装に置き換えるだけで済むことを保証したい。なぜなら service / domain まで改修が波及すると差し替えが現実的でなくなるから。

### 利用フロー

1. 開発者がローカルで `make up`(先行して `make migrate`)を実行すると、api/auth は compose の Postgres に接続して起動する(memory フォールバックは存在しない)
2. テスト実行時はテスト用 Postgres コンテナが立ち上がり、リポジトリ / route / service のテストがその実 DB に接続して回る
3. CI は変更 stack を検出し、DB 依存テストを Postgres 付きのジョブで実行する
4. store を差し替えたい開発者は `infra/postgres` を新実装で置換し、同じ `Run<集約>RepositoryContract` を回して振る舞い等価を確認する(service / domain は無改修)

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: `infra/memory` パッケージ(app/api・app/auth)を**完全削除**する。実行時合成(`cmd/*/main.go`)・`SelectMode` の memory 分岐・memory を参照する全テストから除去する
- [x] R2: 永続化選択を Postgres 一本化する。`SelectMode` / `Mode` / `ModeMemory` を整理し、`DB_HOST` 未設定 + `APP_ENV=local/test` の in-memory フォールバックを廃止して **Postgres 必須(fail-closed)** にする。ローカルは compose の Postgres 経路を前提とする
- [x] R3: テストを Postgres に一本化する。**テスト用 Postgres コンテナを立て、リポジトリ / route / service のテストの向き先をその実 DB に切り替える**。従来 `infra/memory` をテストダブルにしていた route テストは実 DB 経路へ移す。振る舞い契約テスト(`Run<集約>RepositoryContract`)は Postgres 実装のみで回す
- [x] R4: 永続化層の DI が store 差し替え(postgres→mysql 等)に対して `service` 層を壊さないことを検証する。store 固有の seam(`sql.ErrNoRows` 翻訳・unique 違反判定・DSN スキーム・`*sql.Tx`)が `infra/postgres` 内に閉じていることを確認し、漏れがあれば閉じ直す。`service` / `route` / `domain` が domain ポート / domain エラーのみに依存する状態を保証・文書化する
- [x] R5: 削除可能なコードを削除する。memory の実装・テスト・compile assertion(`var _ Port = ...`)・doc コメント参照、および不要になった `Mode` / `ModeMemory` 等の分岐を除去する

### 非機能要件

- **挙動不変(絶対条件)**: 外部から観測できる HTTP API / DTO / OpenAPI 契約・ドメインポートのシグネチャ・env 契約(`DB_*` / `DB_READER_*` / `ISSUER`)・DB スキーマ / マイグレーション / sqlc 生成結果を変えない。既存テストが安全網
- **SPEC-009 の思想を尊重**: オフライン `make check`(`--network none`)を壊さない。DB を要するテストはネットワーク有効な integration フェーズ(テスト用 Postgres コンテナが立つジョブ)に集約する
- **SPEC-010 の維持**: CQRS reader/writer 分離(`OpenPair` / Reader・Writer・Repository ポート / プールルーティング)の設計を維持する
- テスト実行時間の増加は許容範囲に収める(ローカル / CI とも実用的な時間で完了する)

### スコープ外(やらないこと)

- 実際の mysql 実装の追加(今回は「差し替え可能であることの検証 + seam の閉じ直し」まで。mysql 実装本体は別 Spec)
- Go 1.26 bump(ISSUE-027 で別途対応)
- `.env` → compose 移行(調査の結果、app 設定は既に compose.yml に inline 済み・`.env` も既に git-ignore 済みのため対応不要)
- ドメインロジック・HTTP 契約・API の振る舞いの変更(本 Spec はリファクタリング=挙動不変)

## 4. 設計(どう実現するか)

### 方針

`infra/memory` が担う 2 つの役割(①実行時フォールバック / ②テストダブル)を両方 Postgres に寄せて廃止する。

**① 実行時(Postgres 必須・fail-closed)**
- `infra/postgres/db.go` の `SelectMode` を「DB 接続必須・未設定はエラー」に簡素化し、`Mode` / `ModeMemory` を削除(または Postgres 単一に集約)する
- `cmd/api/main.go` / `cmd/authz/main.go` の memory 分岐・`seedMemory` を削除する。合成は常に `OpenPair`(SPEC-010)経由の Postgres 配線に一本化する
- ローカルは既に compose が Postgres 経路(`make up` は先行して `make migrate`)なので、実行フローは実質不変

**② テスト(Postgres 一本化)**
- テスト用 Postgres コンテナを立て、`DB_*` をテスト用 DB に向ける。既存の integration-tagged テスト(contract / pool / selection)を土台に、従来 untagged で memory-backed だった unit テスト(route ハンドラ等)を実 DB 経路へ移す
- 振る舞い契約テスト `Run<集約>RepositoryContract` は Postgres 実装のみで回す(memory 側バインディングを削除)
- build tag 方針・CI ジョブ再編(`cicd.yml` の api/auth unit job と api-integration/auth-integration job の関係)・テスト用 Postgres コンテナの配線は **planner が詳細化**する
- **error injection の扱い(planner が決定する設計上の要点)**: 「リポジトリが DB エラーを返す→ハンドラが 500」等、実 DB では再現しにくいエラーパスのテストは、domain ポートを満たす最小のテスト専用スタブ(テスト直近に置く軽量 fake。`infra/memory` パッケージの復活ではない)で対応する余地を planner が判断する。ただし削除した `infra/memory` を実質的に再導入しないこと

**DI 差し替え耐性の検証と seam の閉じ直し(調査で確認済みの現状)**
- `service` / `route` / `domain` は `pgx` / `database/sql` / `pgconn` / `sqlcgen` を一切 import していない(現状で service 層は差し替え安全)
- store 固有の差し替え面は次に限定され、いずれも `infra/postgres` 内に封じ込め済み。本 Spec ではこれを再確認し、漏れがあれば閉じ直す:
  - `sql.ErrNoRows` → domain sentinel への翻訳(api: task_reader.go / auth: client・user・authcode・refreshtoken の各 repository)
  - api の unique 違反判定(`pgconn.PgError` / SQLSTATE `23505` / 制約名 `tasks_title_key`。`task_repository.go` / `task_writer.go`)
  - 両 `db.go` の `pgx/v5/stdlib` blank import と DSN スキーム `postgres://`
  - `refreshtoken_repository.go` の `Rotate` 用 `*sql.Tx`
- `domain/task` の `DBError.Unwrap()` が driver error を露出しうる点は、route が cause を検査しない(category 型で分岐)ため差し替え安全。現状維持とし本 Spec に記録する
- 「mysql に差し替えるなら `infra/postgres` を新実装で置換し同じ contract を回す」旨を rules / README に一文で追記できる状態にする(実装はしない)

### アーキテクチャ / データ / インターフェース

- ドメインポート(`domain/<集約>/repository.go` の Reader / Writer / Repository)・HTTP 契約・OpenAPI・DB スキーマ・sqlc 生成物は**不変**
- 変更は「実装(`infra/memory` の削除)」「合成(`main.go` / `SelectMode`)」「テスト配線(向き先を Postgres へ)」「CI ジョブ構成」「rules / README の記述」に限定される
- 影響ファイル(stack 別・確定は planner が精緻化):
  - app/api: `infra/memory/`(削除)、`infra/postgres/db.go`(`SelectMode` / `Mode` 整理)、`cmd/api/main.go`・`cmd/api/env.go`、`route/task_handler_test.go`(memory→実 DB)、`service/*_test.go`(必要に応じ)
  - app/auth: `infra/memory/`(削除)、`infra/postgres/db.go`、`cmd/authz/main.go`、`route/token_user_not_found_test.go`・`route/helpers_test.go`(memory→実 DB)
  - 横断: `.github/workflows/cicd.yml`(unit / integration job 再編)、テスト用 Postgres コンテナの compose 配線、`.claude/rules/{db,testing}.md`、`app/api/README.md`・`app/auth/README.md`

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| `infra/memory` をテスト専用パッケージとして温存(実行時のみ Postgres 一本化) | ユーザー方針(memory 完全削除・テストも Postgres 一本化)に反する。同一ポートを満たす 2 実装の保守負債が残る |
| CI の unit job も Postgres 化して全テストを実 DB で回す | SPEC-009 のオフライン `make check`(`--network none`)不変条件を崩す。DB 依存テストは integration フェーズに集約する構成を採用 |
| memory を残し実行時フォールバックのみ維持 | 「Postgres 一本化」「fail-closed の一貫性」を満たさない |
| 今回 mysql 実装まで追加して差し替えを実証 | スコープ肥大。まず「差し替え可能であることの検証 + seam の閉じ直し」に絞り、mysql 実装は別 Spec とする |

## 5. 実装計画

詳細は `docs/plans/SPEC-011-plan.md`(planner が作成)に置く。概略の作業順:

- [x] T1: planner が本 Spec から実装計画を作成(テスト用 Postgres コンテナの立て方・build tag / CI ジョブ再編・削除対象と error injection の扱いを確定) — `docs/plans/SPEC-011-plan.md`
- [x] T2: tester がベースライン(特性化)確認 — 既存 contract / integration テストが現状緑であることを確認
- [x] T3: impl-db が `infra/postgres` の seam 確認・整理、テスト用 DB 接続基盤の整備、memory 削除に伴う Postgres 側テスト受け皿の整備
- [x] T4: impl-api / impl-auth が `cmd/*/main.go` の memory 分岐削除・`SelectMode` 簡素化・route テストの Postgres 切替、domain ポートが store 非依存であることの保証
- [x] T5: impl-ci が CI(`cicd.yml`)の unit / integration ジョブ再編とテスト用 Postgres コンテナの配線、SPEC-009 オフライン方針との整合
- [x] T6: tester がテスト実行(実 DB)、checker が fmt / lint / type check、review-security / review-performance / review-spec がレビュー(特に review-spec で挙動・契約不変を確認)
- [x] T7: `.claude/rules/{db,testing}.md` と `app/api/README.md`・`app/auth/README.md` の memory 記述を現実に合わせて更新
- [x] T8: 価値の検証方法の 3 条件を確認し、本 Spec を done 化

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成。プロジェクト全体リファクタリングの一環として、ユーザーから「auth/api の infra/memory は不要・Postgres 一本化」「永続化層は store 差し替え(postgres→mysql 等)で service 層を壊さない DI に」「現在の動作を保証した上で削除できるコードは削除」の方針を受領。
- 事前調査で以下を確認: (1) service / route / domain は既に store 固有型に非依存で、DI は現状ほぼ達成済み(差し替え面は `infra/postgres` 内の狭い seam に限定)。(2) `infra/memory` は実行時フォールバックとテストダブルの 2 役を担い、削除の主たる波及はテスト(route テストのダブル・contract の memory バインディング・CI の memory-backed unit job)。
- テスト方針をユーザーと確定: **完全削除の上でテストも Postgres 一本化。テスト用 Postgres コンテナを立て、テスト時は向き先をそれに切り替える**。CI の unit job を丸ごと Postgres 化して SPEC-009 のオフライン `make check` を崩す案は不採用とし、DB 依存テストは integration フェーズに集約する。
- スコープ確定: 本 Spec は「memory 削除 + Postgres 一本化 + テストの Postgres 化 + DI 差し替え耐性の検証 / seam 閉じ直し」。Go 1.26 bump は ISSUE-027 で別管理。`.env` → compose 移行は実質完了済み(app 設定は既に compose.yml に inline、`.env` は既に git-ignore)のため対応不要と判断しスコープ外とした。
- status を approved として planner フェーズに進める(ユーザーが方針の主要分岐を承認済み)。

### 2026-07-11

- 実装完了。`infra/memory`(api 3 ファイル / auth 12 ファイル)を完全削除し、`cmd/*/main.go` を Postgres 一本化(`APP_ENV` / `SelectMode` / `ModeMemory` 除去)。route テストを untagged(エラー注入)と `//go:build integration`(実 DB)の 2 層に再編。共有 test-DB ヘルパ `infra/postgres/testsupport` を api/auth に整備。
- 検証: api/auth 双方で `make check`(オフライン)・`make test-integration`(実 DB)・checker 全 stack 緑。review-spec(R1–R5 満たす) / review-security(fail-closed 維持、Blocker 0) / review-performance(SPEC-010 維持)完了。
- 価値の検証方法 3 条件を確認: (1) `infra/memory` 消滅・Postgres 経路のみ、(2) 外部契約不変・contract テスト緑、(3) service/route/domain が driver 非依存・seam は `infra/postgres` 内に文書化。
- status を `done` に更新。
