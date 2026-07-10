---
paths:
  - "app/api/db/**"
  - "app/auth/db/**"
  - "app/api/infra/postgres/**"
  - "app/auth/infra/postgres/**"
  - "app/api/sqlc.yaml"
  - "app/auth/sqlc.yaml"
  - "app/migrator/**"
---

# DB / 永続化層 規約(goose + sqlc / Postgres)

app/api・app/auth の永続化を Postgres で行うための横断規約。担当 agent は impl-db。
DDD の依存性逆転を守り、永続化の詳細(SQL・ドライバ・生成コード)は各スタックの
`infra/postgres` に閉じ込める。`domain` はこの層に依存しない。

マイグレーションの**適用**(up/down/status の実行)は `app/api` / `app/auth` 本体では
なく、共有ツール `app/migrator`(独立 go.mod の Go module。詳細は後述)が行う
(2026-07-09 リファクタ、SPEC-005 plan §RF.1)。`app/migrator/**` も impl-db の担当。

## ツール

- **マイグレーション: goose(pressly/goose)** — プレーン SQL の up/down。レビュー可能な差分として commit する。実行(適用)は `app/migrator` が **library** として使う(後述)。`app/api`・`app/auth` の Makefile は `migrate-create`(新規マイグレーションファイルの scaffold。DB 接続なし)にのみ goose の `go run` CLI を使う
- **クエリ→型安全 Go 生成: sqlc** — `db/queries/**` の SQL から Go を生成する。OpenAPI 契約(SPEC-003)と同じ「単一ソースから生成」方針を DB クエリにも適用する。`app/api`・`app/auth` それぞれの Makefile が `go run <pkg>@<version>` の CLI として実行する
- **goose の閉じ込め**: goose を library として require するのは `app/migrator/go.mod` だけ。`app/api`・`app/auth` の go.mod は goose を require しない(migrate-create は `go run pkg@version` の CLI 実行のため go.mod に現れない)。両スタックの新規 runtime 依存は Postgres ドライバ(pgx)のみを保つ

## コマンド

実行場所はターゲットにより異なる。

| 目的 | 実行場所 | コマンド |
|---|---|---|
| sqlc 生成(`db/queries` → `infra/postgres/sqlcgen`) | `app/api` または `app/auth` | `make sqlc` |
| マイグレーションファイルの新規作成 | `app/api` または `app/auth` | `make migrate-create name=<slug>`(DB 接続なし。ファイル生成のみ) |
| 実 DB 統合テスト | `app/api` または `app/auth` | `make test-integration`(= `go test -tags=integration ./infra/postgres/...`。事前に接続先データベースへマイグレーション適用済みであること) |
| マイグレーション適用(api・auth 両方、ローカル) | リポジトリルート | `make migrate`(`db-up` を前提ターゲットとし、`app/migrator` を `-target api` / `-target auth` で 2 回実行する) |
| マイグレーション適用(任意の target・command を直接指定) | `app/migrator` | `go run ./cmd/migrator -target api\|auth [-command up\|down\|status] [-migrations-dir <path>]`(または `make run ARGS="..."`) |

**per-stack の `migrate-up` / `migrate-down` / `migrate-status` ターゲットは存在しない**(この 2026-07-09 リファクタで `app/migrator` に一本化して移管済み)。マイグレーションの「適用」に関する操作はすべて `app/migrator` 経由で行う。

上記はすべて **生成 / スキーマ操作 / 実 DB 依存であり検査ではない**ため、`make openapi` と同様に `make check` には含めない(`app/migrator/Makefile` の `check` も同様に `fmt-check` + `lint` + `vet` + `build` + `test` のみ)。
一方、sqlc 生成コード(`infra/postgres/sqlcgen`)は `make build` / `make vet` / `make test` の対象であり、スキーマとの drift は許容しない(CI: `.github/workflows/sqlc-drift.yml` が `make sqlc` を再実行して diff を検査する)。

**版**: goose `v3.24.1`(`app/migrator/go.mod` の `github.com/pressly/goose/v3` require が単一の情報源。`app/api`・`app/auth` の Makefile の `GOOSE_VERSION`(migrate-create の CLI 版)もこれと同じ値に保つ)/ sqlc `v1.31.1`(各スタックの Makefile が単一の情報源)。sqlc は常に `go run <pkg>@<version>` の CLI として実行し、module の go.mod には現れない。新規 runtime 依存は Postgres ドライバ `github.com/jackc/pgx/v5 v5.7.2` のみ(`app/api`・`app/auth`・`app/migrator` の go.mod いずれも)。`database/sql` の driver として blank-import し、sqlc 生成コード自体は `sqlc.yaml` の `sql_package: database/sql` により標準ライブラリのみで完結する。

## レイアウト

各 Go スタック(`app/api` / `app/auth`)配下:

- `db/migrations/` — goose のマイグレーション SQL(up/down 対、非修飾 DDL。各スタックは自分専用の database に接続するため、スキーマ名・database 名を DDL に書かない。デフォルトの `public` スキーマに作成される)
- `db/queries/` — sqlc の入力クエリ SQL
- `sqlc.yaml` — `sql_package: database/sql` / `package: sqlcgen` / `out: infra/postgres/sqlcgen`
- `infra/postgres/sqlcgen/` — sqlc 生成コード(commit 対象。手で編集しない)
- `infra/postgres/<集約>_repository.go` — ドメインの `Repository` interface を sqlcgen 越しに満たす実装(例: `task_repository.go`、auth は `user_repository.go` / `client_repository.go` / `authcode_repository.go`)
- `infra/postgres/db.go` — `Config` / `SelectMode` / `Open`(接続プールの上限と ping タイムアウトを持つ)。**環境変数を直接読まない**(下記「接続 env 契約」参照)
- `infra/postgres/seed.go`(auth のみ) — 初期データ投入

リポジトリ直下:

- `app/migrator/` — api/auth 共有のマイグレーション実行ツール(独立 go.mod)。`app/api` / `app/auth` と同じ DDD レイヤ構成(2026-07-10 リファクタ):
  - `cmd/migrator/main.go` — コンポジションルート(CLI フラグ parse・`NewEnv()`+`validate`・infra を service に配線・実行・エラー→exit code。ロジックを持たない)
  - `cmd/migrator/env.go` — この module 自身の環境変数を読む唯一の場所(`os.Getenv` は本ファイルのみ。`DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`/`DB_SSLMODE`/`DB_NAME`/`DB_MAINTENANCE_NAME` + `MIGRATOR_TIMEOUT` を読み、`Env` + `NewEnv` + `validate` で `infra/postgres.Config` / `domain/migration.DatabaseName` に射影する)
  - `domain/migration/` — ドメイン層(pgx/goose/os を import しない)。`Target`(`api`/`auth` の VO・既定 migrations dir を導出)/ `Command`(`up`/`down`/`status` の VO)/ `DatabaseName`(identifier 検証 `^[a-z_][a-z0-9_]*$` + 63byte・`Quoted()` によるクォート。injection 防御の純ロジック)/ `port.go`(`Database{ EnsureExists(ctx, DatabaseName) error }` / `Runner{ Run(ctx, Command, dir string) error }` の 2 ポート)
  - `service/migrate.go` — `Database` の `EnsureExists` → `Runner` の `Run` を協調させる薄い application 層。domain のみに依存
  - `infra/postgres/` — `Database` ポートの実装(`EnsureExister`。`pg_database` 存在確認 + `CREATE DATABASE` 冪等・`42P04`/`23505` 分類・再確認)+ 接続 `Open`/`Config.DSN`(pgx stdlib)
  - `infra/goose/` — `Runner` ポートの実装。goose library(`SetDialect` + `RunContext`)。実行タイムアウト(`MIGRATOR_TIMEOUT`)の適用
  - `Dockerfile`(ビルドコンテキストはリポジトリルート。`app/{api,auth}/db/migrations` を COPY するため。ビルド対象は `./cmd/migrator`)

## 接続 env 契約

**実行時本体(`cmd/api` / `cmd/authz`)**: 各スタックの `cmd/<bin>/env.go`(`package main`)が discrete な `DB_*` 環境変数を読み、`infra/postgres.Config`(`Host` / `Port` / `Name` / `User` / `Password` / `SSLMode`)を組み立てる。`infra/postgres` パッケージ自身は環境変数を読まない(`ConfigFromEnv` のような関数は無い。テスト容易性のため env 読み取りは `cmd/<bin>` 層に一本化されている)。読む変数は `DB_HOST` / `DB_PORT`(既定 `5432`)/ `DB_NAME` / `DB_USER` / `DB_PASSWORD` / `DB_SSLMODE`(既定 `require`。fail-closed 既定 = 未設定時は平文接続へ後退せず暗号化を既定とする。ローカルで非 TLS の Postgres(compose の `postgres` サービス等)へ接続する場合は `DB_SSLMODE=disable` を明示する。ISSUE-016 m-2)。**`DB_SCHEMA` は廃止**(旧: 単一 database + `search_path` によるスキーマ分離。2026-07-09 リファクタで別データベース分離に置き換え済み)。`DB_NAME` に組み込みの既定値は無く(Postgres モード時は `Config.Validate` が必須として検証する)、実際の値(api=`api`、auth=`auth`)は compose(ローカル)/ iac(本番)側で明示的に注入する運用とする。

**マイグレーション実行(`app/migrator`)**: 上記とは別に自分自身の `configFromEnv`(`app/migrator/config.go`)を持ち、同じ `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_SSLMODE` に加え、`DB_NAME`(**既定は `-target` の値**: `api` / `auth`。未設定でも動く)と、`CREATE DATABASE` を発行する接続先を指定する `DB_MAINTENANCE_NAME`(既定 `postgres`)を読む。実行時本体の `Config` には `MaintenanceName` に相当するフィールドは無い(api/auth 自身は自分のデータベースを作成する責務を持たず、既存のものへ接続するだけのため)。

いずれも discrete な値(単一の DSN/URL ではなく)にしているのは、パスワードを Secrets Manager 注入の環境変数のまま扱い、iac 側で URL を組み立てずに済ませるため。

`SelectMode` による永続化実装の選択規則(fail-closed。実行時本体のみに適用され、`app/migrator` には存在しない):

- `DB_HOST` が設定されている → Postgres(`APP_ENV` の値に関わらず)
- `DB_HOST` が未設定かつ `APP_ENV ∈ {local, test}` → in-memory(`infra/memory`)
- `DB_HOST` が未設定かつ上記以外(`APP_ENV=production`・未設定・未知の値を含む) → エラー(memory へのフォールバックなし。本番相当では Postgres 接続を必須とする)

## 別データベース分離

api と auth は同一の Postgres インスタンス上で、**それぞれ専用のデータベース**(api=`api` データベース、auth=`auth` データベース)に分離される。**`search_path` は使わない**(旧設計: 単一 database を `search_path`/`DB_SCHEMA` で api スキーマ・auth スキーマに分離していたが、2026-07-09 リファクタでこの別データベース分離に置き換えた)。マイグレーションの DDL は非修飾のまま各データベースのデフォルト `public` スキーマに適用される。

データベースそのものの作成(旧設計での「スキーマの作成」に相当)は goose の管轄外で、`app/migrator` の `ensureDatabase`(`app/migrator/database.go`)が担う:

- 対象データベースの `CREATE DATABASE` を、`DB_MAINTENANCE_NAME`(既定 `postgres`)への接続から発行する。既に存在する場合は何もしない(冪等)。concurrent な複数実行(同時に 2 つの init コンテナが起動する等)がどちらも成功で終わるよう、`duplicate_database` / 該当する `unique_violation` を成功として扱う
- ローカル・CI・本番のすべてでこの一本化された経路を使う(旧: ローカルは compose の `docker-entrypoint-initdb.d` スクリプト、本番は `Dockerfile.migrate` のエントリポイントが個別に `CREATE SCHEMA IF NOT EXISTS` していた構成は廃止済み)

## マイグレーションの実行

`app/migrator`(独立 go.mod。api/auth の go.mod に goose を持ち込まないための隔離)が唯一の実行経路。CLI 契約:

```
migrator -target api|auth [-command up|down|status] [-migrations-dir <path>]
```

- `-target`(必須): `api` か `auth` のいずれか。既定の `DB_NAME`(未設定時)と既定の `-migrations-dir`(`/migrations/<target>`。コンテナイメージの COPY レイアウトに対応)を決める
- `-command`(既定 `up`): goose に渡すコマンド(`up` / `down` / `status`)
- 実行フロー: (1) `DB_MAINTENANCE_NAME` への接続で対象データベースを `ensureDatabase` により作成(未存在時のみ)、(2) 対象データベースへ接続し直し、`goose.RunContext` を `-command` で実行。goose 実行本体には接続確認(`pingTimeout`。5 秒)とは別に `defaultMigrationTimeout`(既定 5 分。`MIGRATOR_TIMEOUT` 環境変数で上書き可)の deadline を設けており、ハング(ロック待ち等)時に無期限待機せず fail-fast する

- **ローカル**: リポジトリルートの `make migrate`(`db-up` を前提ターゲットとし、`app/migrator` を `-target api` → `-target auth` の順に実行する)。ルートの `make up` / `make up-d` は `migrate` を前提ターゲットに持つため、compose 起動時に自動適用される
- **本番**: ECS の init コンテナとして `app/migrator` イメージ(`app/migrator/Dockerfile`。ビルドコンテキストはリポジトリルート)を `-target api` または `-target auth` で実行する。両スタックの `db/migrations` を 1 つの共有イメージにバンドルし、`-target` で選択する(RF-b)。`dependsOn: SUCCESS` でアプリ本体コンテナの起動をゲートする

## CI

- `.github/workflows/sqlc-drift.yml` — `db/queries` / `db/migrations` / `sqlc.yaml` の変更を検知し、`make sqlc` を再実行して `infra/postgres/sqlcgen` に diff がないか検査する(api / auth 独立ジョブ)
- `.github/workflows/cicd.yml` の `migrator` ジョブ — `app/migrator` 自身の `make check`(独立 go.mod のため専用レーン)
- `.github/workflows/cicd.yml` の `api-integration` / `auth-integration` ジョブ — pinned な postgres service container を起動し、`app/migrator` の `-target` / `-command` 経由でデータベース作成 + up → down → up の健全性確認を行った上で、対象スタックの `make test-integration` を実行する

## 契約(seam)

- 実装対象はドメインが宣言する `domain/<aggregate>/repository.go` の `Repository` interface。**ポート(interface)側は impl-api / auth、実装(`infra/postgres`)側は impl-db** が持つ
- `FindByX` が該当なしのとき、ドメインの `ErrNotFound` 等の sentinel error を返す(`sql.ErrNoRows` を握りつぶさない)。振る舞いは既存の `infra/memory` 実装と一致させる
- クエリ / スキーマを変えたら sqlc を再生成して commit する。Go と生成物を別々に更新しない(drift 検査は impl-ci が CI に用意する)
- 実行時本体(`cmd/<bin>/env.go`)が読む DB_\* 環境変数の一覧・既定値を変える場合、それは impl-api / impl-auth の変更範囲(`cmd/*/env.go`)。impl-db は `infra/postgres.Config` の形(フィールド)とその消費側(`db.go`)のみを担当し、env 読み取りロジック自体には触れない

## セキュリティ

- 接続情報(ホスト・ユーザー・パスワード)をコード・tfvars に平文で書かない。RDS のマスター資格情報は Secrets Manager 管理(`app/iac/modules/db`)で、アプリには環境変数 / Secrets 経由で注入する
- SQL は sqlc のパラメータ化クエリを用い、文字列連結でクエリを組み立てない(SQL インジェクション防止)。`CREATE DATABASE` のようにパラメータ化できない識別子位置(`app/migrator/database.go`)は allowlist 検証(`validateIdentifier`)+ quoting の二重の防御で組み立てる
