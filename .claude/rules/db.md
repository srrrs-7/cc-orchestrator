---
paths:
  - "app/api/db/**"
  - "app/auth/db/**"
  - "app/api/infra/postgres/**"
  - "app/auth/infra/postgres/**"
  - "app/api/sqlc.yaml"
  - "app/auth/sqlc.yaml"
---

# DB / 永続化層 規約(goose + sqlc / Postgres)

app/api・app/auth の永続化を Postgres で行うための横断規約。担当 agent は impl-db。
DDD の依存性逆転を守り、永続化の詳細(SQL・ドライバ・生成コード)は各スタックの
`infra/postgres` に閉じ込める。`domain` はこの層に依存しない。

## ツール

- **マイグレーション: goose(pressly/goose)** — プレーン SQL の up/down。レビュー可能な差分として commit する
- **クエリ→型安全 Go 生成: sqlc** — `db/queries/**` の SQL から Go を生成する。OpenAPI 契約(SPEC-003)と同じ「単一ソースから生成」方針を DB クエリにも適用する
- ツールは Makefile 経由で `go run <pkg>@<version>` として実行し(`make openapi` の swag と同じパターン)、生成/マイグレーション用ツールを module の runtime 依存(go.mod)に載せない。**新規の runtime 依存は Postgres ドライバのみ**を想定する

## コマンド

実行は対象スタックのディレクトリ(`app/api` または `app/auth`)で行う。各ターゲットの実体はそのスタックの `Makefile` が単一の情報源(api / auth で同一の目的・コマンド名)。

| 目的 | コマンド |
|---|---|
| sqlc 生成(`db/queries` → `infra/postgres/sqlcgen`) | `make sqlc` |
| マイグレーション適用(最新まで) | `make migrate-up` |
| マイグレーションを 1 つ戻す | `make migrate-down` |
| マイグレーション適用状況の表示 | `make migrate-status` |
| マイグレーションファイルの新規作成 | `make migrate-create name=<slug>` |
| 実 DB 統合テスト | `make test-integration`(= `go test -tags=integration ./infra/postgres/...`。事前に接続先 Postgres へ `make migrate-up` 済みであること) |

上記はすべて **生成 / スキーマ操作 / 実 DB 依存であり検査ではない**ため、`make openapi` と同様に `make check` には含めない。
一方、sqlc 生成コード(`infra/postgres/sqlcgen`)は `make build` / `make vet` / `make test` の対象であり、スキーマとの drift は許容しない(CI: `.github/workflows/sqlc-drift.yml` が `make sqlc` を再実行して diff を検査する)。

**版(各スタックの `Makefile` が単一の情報源)**: goose `v3.24.1` / sqlc `v1.31.1`。いずれも `go run <pkg>@<version>` の CLI として実行し、module の go.mod には現れない(ツールバイナリを runtime 依存にしない)。新規 runtime 依存は Postgres ドライバ `github.com/jackc/pgx/v5 v5.7.2` のみ(`database/sql` の driver として blank-import。生成コード自体は `sqlc.yaml` の `sql_package: database/sql` により標準ライブラリのみで完結する)。

## レイアウト

各 Go スタック(`app/api` / `app/auth`)配下:

- `db/migrations/` — goose のマイグレーション SQL(up/down 対、非修飾 DDL。スキーマは `search_path` で選択されるため CREATE TABLE 等にスキーマ名を書かない)
- `db/queries/` — sqlc の入力クエリ SQL
- `sqlc.yaml` — `sql_package: database/sql` / `package: sqlcgen` / `out: infra/postgres/sqlcgen`
- `infra/postgres/sqlcgen/` — sqlc 生成コード(commit 対象。手で編集しない)
- `infra/postgres/<集約>_repository.go` — ドメインの `Repository` interface を sqlcgen 越しに満たす実装(例: `task_repository.go`、auth は `user_repository.go` / `client_repository.go` / `authcode_repository.go`)
- `infra/postgres/db.go` — `Config` / `ConfigFromEnv` / `DSN` / `SelectMode` / `Open`(接続プールの上限と ping タイムアウトを持つ)
- `infra/postgres/seed.go`(auth のみ) — 初期データ投入

## 接続 env 契約

`infra/postgres/db.go` の `ConfigFromEnv` が読む discrete な環境変数: `DB_HOST` / `DB_PORT`(既定 `5432`)/ `DB_NAME` / `DB_USER` / `DB_PASSWORD` / `DB_SSLMODE`(既定 `disable`)/ `DB_SCHEMA`(既定は api=`api`、auth=`auth`)。単一の DSN/URL ではなく discrete 値にしているのは、パスワードを Secrets Manager 注入の環境変数のまま扱い、iac 側で URL を組み立てずに済ませるため。

`SelectMode` による永続化実装の選択規則(fail-closed):

- `DB_HOST` が設定されている → Postgres(`APP_ENV` の値に関わらず)
- `DB_HOST` が未設定かつ `APP_ENV ∈ {local, test}` → in-memory(`infra/memory`)
- `DB_HOST` が未設定かつ上記以外(`APP_ENV=production`・未設定・未知の値を含む) → エラー(memory へのフォールバックなし。本番相当では Postgres 接続を必須とする)

## スキーマ分離

api と auth は同一の Postgres database を共有し、`search_path`(`DB_SCHEMA`)でスキーマを分離する(api=`api` スキーマ、auth=`auth` スキーマ)。マイグレーションの DDL は非修飾のままこの `search_path` に依存する。

スキーマそのものの作成は goose の管轄外:

- ローカル: `docker/postgres/initdb/00-schemas.sql`(compose postgres の初期化スクリプト)
- 本番: マイグレーション用イメージのエントリポイントが goose 実行前に `CREATE SCHEMA IF NOT EXISTS` を実行する

## マイグレーションの実行

- **ローカル**: 各スタックの `make migrate-up`(compose の postgres が対象)。リポジトリルートの `make migrate`(`db-up` を前提ターゲットとし、api → auth の順に `migrate-up` を呼ぶ)。ルートの `make up` / `make up-d` は `migrate` を前提ターゲットに持つため、compose 起動時に自動適用される
- **本番**: ECS の init コンテナとして `app/{api,auth}/Dockerfile.migrate` + `db/migrate-entrypoint.sh` を実行する。接続は libpq 標準の `PG*` 環境変数で行う

## CI

- `.github/workflows/sqlc-drift.yml` — `db/queries` / `db/migrations` / `sqlc.yaml` の変更を検知し、`make sqlc` を再実行して `infra/postgres/sqlcgen` に diff がないか検査する(api / auth 独立ジョブ)
- `.github/workflows/cicd.yml` の `api-integration` / `auth-integration` ジョブ — postgres service container を起動し、goose で up → down → up を通したうえで `make test-integration` を実行する

## 契約(seam)

- 実装対象はドメインが宣言する `domain/<aggregate>/repository.go` の `Repository` interface。**ポート(interface)側は impl-api / auth、実装(`infra/postgres`)側は impl-db** が持つ
- `FindByX` が該当なしのとき、ドメインの `ErrNotFound` 等の sentinel error を返す(`sql.ErrNoRows` を握りつぶさない)。振る舞いは既存の `infra/memory` 実装と一致させる
- クエリ / スキーマを変えたら sqlc を再生成して commit する。Go と生成物を別々に更新しない(drift 検査は impl-ci が CI に用意する)

## セキュリティ

- 接続情報(ホスト・ユーザー・パスワード)をコード・tfvars に平文で書かない。RDS のマスター資格情報は Secrets Manager 管理(`app/iac/modules/db`)で、アプリには環境変数 / Secrets 経由で注入する
- SQL は sqlc のパラメータ化クエリを用い、文字列連結でクエリを組み立てない(SQL インジェクション防止)
