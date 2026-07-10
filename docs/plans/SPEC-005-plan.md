# SPEC-005 実装計画: app/api・app/auth の Postgres 永続化基盤(goose + sqlc)

- 起点 Spec: `docs/specs/20260709-005-postgres-persistence.md`(status: approved)
- 対象要件: R1〜R9(+ 非機能: std-lib 緩和 / サプライチェーン / セキュリティ / マイグレーション安全性 / 既存テスト維持)
- 関連ルール: `.claude/rules/db.md`(goose/sqlc 契約・seam・セキュリティ)、`.claude/rules/{api,auth,iac,testing,workflow,orchestration}.md`

この計画が方針・変更ファイル・手順・テスト戦略・リスクの正。Spec §5 のタスク(T1〜T8)を具体化する。

---

## 0. 確定した残ゲートの結論(要約)

Spec §5 で planner に委ねられた残ゲートの結論。詳細根拠は各節に展開する。

| ゲート | 結論 |
|---|---|
| 版固定 | pgx = `github.com/jackc/pgx/v5 v5.7.2`(唯一の新規 runtime require)。goose = `github.com/pressly/goose/v3/cmd/goose@v3.24.1`(`go run` CLI・require に載せない)。sqlc = `github.com/sqlc-dev/sqlc/cmd/sqlc@v1.28.0`(`go run` CLI)。**版はベースライン。impl-db が実装時に resolve 可否を確認し、resolve 不可なら最寄りの stable patch に調整して Makefile に確定記載する(Makefile が版の単一情報源)** |
| ディレクトリ/命名 | 各 Go スタック配下に `db/migrations/`(goose SQL・DDL のみ)・`db/queries/`(sqlc 入力)・`sqlc.yaml`。sqlc 生成コードは `infra/postgres/sqlcgen/`(package `sqlcgen`、コミット対象)。手書き実装は `infra/postgres/<集約>_repository.go` |
| スキーマ分離機構 | 同一 database 内の別スキーマ `api` / `auth`。**マイグレーション/クエリは全て非修飾(unqualified)**。実際のスキーマ配属は接続の `search_path=<schema>` で行う。**スキーマ(名前空間)の作成は goose の外(local=postgres init script / prod=iac のブートストラップ)**。goose のバージョン管理表 `goose_db_version` は各スタック自身のスキーマ内に置く(api/auth で衝突しない) |
| Make ターゲット名 | 各スタック Makefile: `make sqlc`(生成)/ `make migrate-up` / `make migrate-down` / `make migrate-status` / `make migrate-create name=<n>`。いずれも `make check` に含めない。`make test-integration`(build tag `integration`)を追加 |
| 切替の env / DSN / 本番必須強制 | 離散 env(`DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_USER`/`DB_PASSWORD`/`DB_SSLMODE`/`DB_SCHEMA`)からアプリ/goose が DSN を組み立てる。**選択規則: `DB_HOST` があれば Postgres → 無ければ `APP_ENV∈{local,test}` のときだけ memory、それ以外(既定=未設定/production)は起動失敗(fail-closed)**。パスワードは Secrets Manager の master secret JSON key から ECS `secrets` 経由で注入(平文なし) |
| seed 置き場所 | **client / user とも Postgres 経路の起動時 idempotent seed**(`ON CONFLICT DO NOTHING`)。マイグレーションは DDL のみ(データ・パスワードを含めない)。memory 経路の既存 seed はそのまま |
| authcode 単回使用/TTL | `authorization_codes` 表 + `expires_at timestamptz`。`Consume` = `DELETE ... WHERE code=$1 RETURNING expires_at`(行ロックで単一勝者、返った `expires_at` で `nil`/`ErrExpired` を分類、0 行=`ErrNotFound`)。`FindByCode` は `expires_at>now()` の有効行のみ返し、期限切れは lazy に DELETE。memory 実装と同値 |
| 統合テスト方式 | domain/service のユニットは fake 継続(DB 非依存)。`infra/postgres` は build tag `//go:build integration` で分離し、実 DB は CI の postgres service container。memory と postgres が同一の「振る舞い契約スイート」を共有 |
| iac 変更範囲 | plan まで。(a) スキーマ作成ブートストラップ、(b) auth への DB 接続 env 注入 + master secret の JSON key 注入(api も `DB_USER`/`DB_PASSWORD` 化)、(c) マイグレーション実行(推奨: init コンテナ `dependsOn: SUCCESS`、代替: 一回限り ECS タスク) |
| compose/Makefile 担当 | repo ルートの `compose.yml`(postgres サービス)/ ルート `Makefile`(`db-up` / `migrate`)は **impl-db** が担当(横断の永続化ツーリング。DSN/env 契約を DB seam と同じ手に集約)。prod 用マイグレーション image の `Dockerfile.migrate` も impl-db |

**tester の TDD 方針の推奨: 振る舞い契約スイート(memory/postgres 共通)は先行作成(TDD)。ただし `infra/postgres` の実 DB 統合テストは、スキーマ確定に依存するため impl-db のマイグレーション/生成が出た直後に一体で仕上げる「近接後追い」を推奨**(理由は §テスト戦略)。

---

## 1. 方針

### 1.1 採用アプローチ

DDD の依存性逆転を維持し、`infra/postgres` を `infra/memory` と同格の `Repository` 実装として追加する。契約の正は `domain/<集約>/repository.go` の interface(不変)。スキーマは goose(SQL)、クエリ→型安全アクセスは sqlc で単一ソースから生成し、生成物をコミットして CI で drift 検出する(SPEC-003 の思想を DB へ展開)。

```
domain/<集約>/repository.go        (Repository interface = ポート・契約の正 / 不変)
        ▲ 実装(依存性逆転)
infra/postgres/<集約>_repository.go ─uses─▶ infra/postgres/sqlcgen(sqlc 生成)─▶ database/sql + pgx/v5 stdlib
        ▲                                          ▲
db/queries/*.sql ──(make sqlc = go run sqlc)───────┘
db/migrations/*.sql ──(make migrate-up = go run goose)──▶ Postgres(schema=api / auth、search_path 分離)
                                                     ├─ local : compose postgres(schema は init script で作成)
                                                     └─ prod  : RDS(modules/db。schema は iac ブートストラップ)
cmd/*/main.go(persistence 配線ブロック=impl-db): DB_HOST の有無 + APP_ENV で memory/postgres を選択
CI(impl-ci): (1) sqlc drift(make sqlc → git diff --exit-code) (2) 統合テスト(postgres service + goose up→down→up + go test -tags=integration)
```

### 1.2 主要な設計判断と退けた代替案

| 判断 | 採用 | 退けた案と理由 |
|---|---|---|
| スキーマ分離の表現 | **非修飾 DDL/クエリ + 接続 `search_path`**、スキーマ作成は goose 外のブートストラップ | (a) 完全修飾(`api.tasks`)= sqlc の静的解析は安定するが DDL がスキーマ名に密結合し冗長。(b) goose 内で `CREATE SCHEMA` = goose のバージョン表がスキーマ存在前に必要になる鶏卵問題。非修飾+ブートストラップは sqlc が単一カタログで完結し、`search_path` が R3「search_path で分離」を字義どおり満たす |
| client の多値属性(redirect_uris / scopes / response_types / grant_types) | **`jsonb` 1 列ずつ**、repo が `encoding/json`(stdlib)で `[]string` と相互変換 | (a) `text[]` = `database/sql` + pgx stdlib の配列スキャンに癖があり sqlc 生成型が不安定。(b) 子テーブル正規化 = デモの単一クライアントに過剰。`jsonb`→`[]byte` は sqlc/database/sql で安定し stdlib のみで扱える |
| authcode の単回使用 | **`Consume` = DELETE ... RETURNING**(delete-based) | フラグ更新(`consumed=true`)は行が残り store が肥大、かつ memory 実装(delete-based)と振る舞いが乖離する。DELETE は行ロックで単一勝者を保証し memory と同値 |
| seed | **起動時 idempotent upsert(Postgres 経路)** | マイグレーション seed は静的 client には合うが、demo user のパスワード(現状は起動時ランダム生成・実質未使用)を committed SQL に置くと auth のセキュリティ規約(パスワードを SQL に埋め込まない)に触れる。起動時 seed は現行 `seed()` の意図に最も近く、fleet/再起動で冪等 |
| 本番の Postgres 必須強制 | **fail-closed**(既定で DB 未設定なら起動失敗、memory は `APP_ENV∈{local,test}` の明示 opt-in) | 「既定=memory、prod は APP_ENV=production を明示」は設定忘れで無言のデータ喪失(R6 が警戒する footgun)。fail-closed が安全側 |
| DSN の受け渡し | **離散 `DB_*` env からアプリ/goose が組み立て** | 単一 `DATABASE_URL` を iac が組み立てると、host/port は RDS リソース由来・password は Secrets 由来で組成が不格好。離散 env なら password だけ Secrets の JSON key から注入でき平文 URL を作らずに済む |
| prod マイグレーション実行 | **init コンテナ(`dependsOn: SUCCESS`)を推奨**、代替=一回限り ECS タスク | init コンテナはデプロイ毎に自動適用でき運用が単純。並行起動時の競合は goose の advisory lock / dev の `desired_count=1` で緩和。最終選択は impl-iac(plan) |
| ドライバ | `database/sql` + pgx/v5 stdlib | Spec 確定。pgxpool ネイティブは生成コードが pgx 直依存になり std-lib leaning に反する(将来最適化余地として保持) |

### 1.3 不変条件(絶対に崩さない前提)

- 新規 runtime require は **pgx のみ**。goose/sqlc は `go run <pkg>@<pinned>` の CLI で go.mod に載せない。生成コードは `database/sql`(標準)のみ依存
- `domain` / `service` / `route` は標準ライブラリ維持(緩和は `infra/postgres` に限定)
- SQL は sqlc のパラメータ化クエリのみ(文字列連結禁止)。接続情報は Secrets/env 注入で平文禁止
- `infra/memory` は削除しない。既存テスト(memory 経路)はグリーン維持
- `Repository` interface(ポート)は原則不変(§2.2 の 1 点を除く)
- `terraform apply` はしない(plan まで)

---

## 2. 変更ファイル(stack ごと)

### 2.1 app/api(impl-db 主担当)

| 種別 | パス | 内容 |
|---|---|---|
| 追加 | `app/api/db/migrations/000001_create_tasks.sql` | goose up/down。`tasks` 表(非修飾)。`id text PK` / `title text NOT NULL UNIQUE` / `status text NOT NULL CHECK(...)` / `priority text NOT NULL CHECK(...)` / `created_at`/`updated_at timestamptz NOT NULL` |
| 追加 | `app/api/db/queries/tasks.sql` | sqlc 入力。`-- name: UpsertTask :exec`(INSERT ... ON CONFLICT(id) DO UPDATE)/ `GetTaskByID :one` / `GetTaskByTitle :one` / `ListTasks :many` |
| 追加 | `app/api/sqlc.yaml` | version 2 / engine postgresql / schema=`db/migrations` / queries=`db/queries` / gen.go: package `sqlcgen`, out `infra/postgres/sqlcgen`, sql_package `database/sql`, emit_interface true |
| 追加 | `app/api/infra/postgres/sqlcgen/*.go` | sqlc 生成(コミット)。`db.go` / `models.go` / `tasks.sql.go` / `querier.go` |
| 追加 | `app/api/infra/postgres/task_repository.go` | `task.Repository` 実装。`sql.ErrNoRows`→`task.ErrNotFound` へマップ。`Reconstruct` でクローン境界を維持 |
| 追加 | `app/api/infra/postgres/db.go`(or `open.go`) | `*sql.DB` オープン(pgx stdlib 登録)・DSN 組み立て・ping。persistence 選択ヘルパ |
| 変更 | `app/api/cmd/api/main.go` | **persistence 配線ブロックのみ**(impl-db)。`DB_HOST`/`APP_ENV` で memory/postgres 選択、fail-closed、graceful shutdown で `db.Close()` |
| 変更 | `app/api/Makefile` | `sqlc` / `migrate-up` / `migrate-down` / `migrate-status` / `migrate-create` / `test-integration` を追加(版ピンを変数化) |
| 生成 | `app/api/go.mod` / `go.sum` | `require github.com/jackc/pgx/v5 v5.7.2`(+ 推移依存)。**pgx 以外の runtime require が増えないこと** |
| 追加(任意) | `app/api/Dockerfile.migrate` | goose バイナリ(build stage で `go build` した pinned goose)+ `db/migrations` を含む migrate image(prod 用)。iac が参照 |

### 2.2 app/auth(impl-db 主担当 + impl-auth のポート補助 1 点)

| 種別 | パス | 内容 |
|---|---|---|
| **変更(ポート補助)** | `app/auth/domain/authcode/code_challenge.go` | **impl-auth**: `CodeChallenge` に生の challenge 文字列を読む accessor(例 `Challenge() string`)を追加。現状 `Method()` のみで raw challenge を取り出せず、`infra/postgres` が PKCE challenge を永続化/`Reconstruct` できない。`CodeChallengeMethod.String()` は既存で流用可 |
| 追加 | `app/auth/db/migrations/000001_create_auth.sql` | `clients`(id PK / redirect_uris,allowed_scopes,response_types,grant_types を `jsonb NOT NULL`)/ `users`(id PK / username UNIQUE / password / profile_name / profile_email)/ `authorization_codes`(code PK / client_id / user_id / redirect_uri / scope / nonce nullable / challenge / challenge_method / expires_at timestamptz / consumed bool default false / created_at)。全て非修飾 |
| 追加 | `app/auth/db/queries/{clients,users,authcodes}.sql` | client: `GetClientByID :one` + `UpsertClient :exec`(seed 用)。user: `GetUserByID :one` / `GetUserByUsername :one` + `UpsertUser :exec`(seed 用)。authcode: `InsertAuthCode :exec` / `GetActiveAuthCode :one`(WHERE code=$1 AND consumed=false AND expires_at>now())/ `DeleteExpiredAuthCode :exec`(lazy 用)/ `ConsumeAuthCode :one`(DELETE ... RETURNING expires_at) |
| 追加 | `app/auth/sqlc.yaml` | api と同型(out `infra/postgres/sqlcgen`, package `sqlcgen`, sql_package `database/sql`) |
| 追加 | `app/auth/infra/postgres/sqlcgen/*.go` | sqlc 生成(コミット) |
| 追加 | `app/auth/infra/postgres/{client,user,authcode}_repository.go` | 各 `Repository` 実装。authcode は `Consume` の delete-based 単回使用・TTL・lazy eviction を実装。client は jsonb⇔`[]string` 変換 |
| 追加 | `app/auth/infra/postgres/db.go` + `seed.go` | `*sql.DB` オープン + persistence 選択ヘルパ + Postgres 経路の idempotent seed(demo client/user を `Upsert...` で投入) |
| 変更 | `app/auth/cmd/authz/main.go` | **persistence 配線ブロックのみ**(impl-db)。memory/postgres 選択、fail-closed、Postgres 時は Postgres seed を呼ぶ(memory 時は既存 `seed()` 維持)、shutdown で `db.Close()` |
| 変更 | `app/auth/Makefile` | api と同一ターゲット追加 |
| 生成 | `app/auth/go.mod` / `go.sum` | `require github.com/jackc/pgx/v5 v5.7.2`。**pgx 以外の runtime require が増えないこと** |
| 追加(任意) | `app/auth/Dockerfile.migrate` | api と同型の migrate image |

### 2.3 app/iac(impl-iac / plan まで)

| 種別 | パス | 内容 |
|---|---|---|
| 変更 | `app/iac/envs/dev/main.tf`(または `modules/db` / 新 `modules/migrate`) | (a) スキーマ `api`/`auth` の作成ブートストラップ、(b) `module.service_auth` に DB 接続 env(`DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_SSLMODE`/`DB_SCHEMA=auth`)を注入、api は `DB_SCHEMA=api` を追加し既存 `DB_CREDENTIALS` を `DB_USER`/`DB_PASSWORD`(master secret の JSON key `:username::`/`:password::`)へ置換、(c) マイグレーション実行(init コンテナ or 一回限りタスク)+ migrate image 用 ECR/参照 |
| 変更(必要時) | `app/iac/modules/service/*` | init コンテナ(`dependsOn`)対応の container_definitions 拡張、または一回限りタスク定義の受け口 |
| 変更(必要時) | `app/iac/modules/network/*` | RDS SG が ECS からの 5432 を既に許可済みか確認(既存 `db_port` 変数あり)。不足なら追加 |

### 2.4 CI / repo ルート(impl-ci / impl-db)

| 種別 | パス | 担当 | 内容 |
|---|---|---|---|
| 追加 | `.github/workflows/sqlc-drift.yml` | impl-ci | Go setup → api/auth で `make sqlc` → `git add -N` + `git diff --exit-code`(contract-drift.yml と同型) |
| 変更 or 追加 | `.github/workflows/cicd.yml`(api/auth job)または新 job | impl-ci | postgres service container で `make migrate-up` → goose up→down→up の健全性 → `make test-integration`。既存 `make check`(memory 経路)は据え置き |
| 変更 | `.github/workflows/cicd.yml` env コメント / `cache` | impl-ci | 「stdlib-only, no go.sum」前提が pgx 追加で崩れる箇所のコメント修正・Go module cache 有効化 |
| 変更 | `compose.yml` | **impl-db** | `postgres` サービス(`127.0.0.1` バインド・init script マウント・volume)追加。api/auth に `DB_*` env と `depends_on: postgres(healthy)` を付与 |
| 追加 | `docker/postgres/initdb/00-schemas.sql`(名称は impl-db 裁量) | **impl-db** | `CREATE SCHEMA IF NOT EXISTS api; CREATE SCHEMA IF NOT EXISTS auth;`(compose 初回 init) |
| 変更 | ルート `Makefile` | **impl-db** | `db-up`(postgres のみ起動)/ `migrate`(api+auth の `migrate-up` を compose DSN で実行)追加 |
| 変更 | `.claude/rules/db.md` | **impl-db** | 「コマンド」表・レイアウト表を確定版で追記(§手順 T8) |
| 変更 | `CLAUDE.md` | admin | コマンド早見表に DB 系ターゲットを反映(admin のメタ整備範囲) |

---

## 3. 手順(agent 別・順序と並列可否)

> 並列可能箇所を明示。**api の task と auth の 3 集約は scope 独立で並列可**(別 module・別ディレクトリ)。

### フェーズ A: 準備・ポート補助(直列)
- **A1 (impl-auth)**: `CodeChallenge` に raw challenge accessor を追加(§2.2 の 1 点)。他のポート(interface / `Reconstruct`)は不変。ここだけ impl-auth が先行し、以降 impl-db が利用する。
  - 他の value object の string accessor は確認済みで既存(`ClientID`/`RedirectURI`/`Username`/`Nonce`/`Code`/`Scope`/`Profile`)。**impl-db は実装着手時に再確認し、不足があれば impl-auth / impl-api に最小追加を依頼**(必要時のみのポート変更)。

### フェーズ B: テスト先行設計(A と並列開始可)
- **B1 (tester)**: memory/postgres 共通の「振る舞い契約スイート」を設計。`Repository` を引数に取り、正常/異常/境界を回す table-driven(§テスト戦略の観点表)。memory は無条件で通す。postgres 版は `//go:build integration` で分離し、実 DB 接続と migrate 前提を記述。
  - authcode の単回使用(2 回 Consume→1 回 nil / 1 回 ErrNotFound)・TTL(期限切れは FindByCode/Consume で不可視・lazy 削除)・並行 Consume(単一勝者)を明記。
  - task の UNIQUE(title) は memory と postgres で振る舞いが異なる(memory は Save で dup title を黙認)ため、**共通スイートは両者共有の不変(id upsert・FindByID/Title の ErrNotFound・FindAll)に限定し、UNIQUE 制約違反は postgres 専用テストで検証**(§リスク R-a)。

### フェーズ C: 実装(api と auth を並列)
- **C1 (impl-db / app/api)**: `db/migrations` → `sqlc.yaml` → `make sqlc` 生成 → `infra/postgres/task_repository.go` + DSN/選択ヘルパ → `cmd/api/main.go` の persistence 配線 → `Makefile` ターゲット。go.mod に pgx 追加。
- **C2 (impl-db / app/auth)**: `db/migrations` → `db/queries` → `make sqlc` 生成 → `infra/postgres/{client,user,authcode}_repository.go` + seed + DSN/選択ヘルパ → `cmd/authz/main.go` の persistence 配線(Postgres 時 seed)→ `Makefile`。go.mod に pgx 追加。
  - **C1 と C2 は並列可**(A1 完了後)。
- **C3 (impl-db / 横断)**: `compose.yml`(postgres + init script + volume + DB_* env)、`docker/postgres/initdb/`、ルート `Makefile`(`db-up`/`migrate`)、任意の `Dockerfile.migrate`。C1/C2 の DB_*/DSN 契約が固まり次第。

### フェーズ D: インフラ・CI(C の命名確定に一部依存)
- **D1 (impl-iac / plan まで)**: スキーマ作成ブートストラップ・auth への DB env 注入・api の Secrets JSON key 化・マイグレーション実行(init コンテナ推奨)。`make plan`(dev)で差分確認、破壊的変更(replace)を報告。**apply しない**。C1/C2 の env 名(`DB_*`)・migrate image 契約に依存。
- **D2 (impl-ci)**: `sqlc-drift.yml`(Go のみ)追加、統合テスト job(postgres service + goose up→down→up + `make test-integration`)追加、既存 CI コメント/cache 修正。C1/C2 の `make sqlc` / `make migrate-up` / `make test-integration` 命名に依存。
  - **D1 と D2 は互いに独立で並列可**。

### フェーズ E: 検証・レビュー(直列ゲート)
- **E1 (tester)**: `make test`(memory 経路・既存グリーン維持)+ `make test-integration`(ローカル compose postgres or CI)実行、不足テスト補完。
- **E2 (checker)**: 各スタック `make check`(fmt-check + lint + vet + build + test)、iac `make check ENV=dev`、web は無関係。**生成コード(`sqlcgen`)が build/vet/test を通ること**を確認。E1 通過後に実施。
- **E3 (review-security / review-performance / review-spec、並列)**: E2 通過後。
  - security: 平文接続情報の不在(env/Secrets のみ)、SQL のパラメータ化(連結禁止)、authcode/パスワードの機微データ扱い(§リスク R-b の plaintext password を明示評価)、search_path のみの分離が権限境界でない点。
  - performance: コネクション寿命/プール設定、`FindAll` の全件・authcode の lazy eviction のクエリ効率、init コンテナ毎回 migrate のコスト。
  - spec: R1〜R9 の充足(§4 の対応表)。
- **E4 (指摘対応)**: Blocker/Major は impl-db(実装)/ impl-auth(ポート)/ impl-iac / impl-ci へ差し戻し、E1→E3 を再実行。今回対応しない指摘は issue-creator が Issue 起票。

### フェーズ F: 記録・規約反映
- **F1 (impl-db)**: `.claude/rules/db.md` の「コマンド」表・レイアウト表を確定版で追記(空欄だった箇所)。
- **F2 (admin)**: `CLAUDE.md` コマンド早見表に DB 系ターゲットを反映。
- **F3 (admin + spec skill)**: SPEC-005 の status を `in-progress`→(完了時)`done`、§6 経緯に結果追記、frontmatter `updated` 更新。

---

## 4. 要件 → 手順・テストの対応

| 要件 | 手順 | テスト戦略での担保 |
|---|---|---|
| R1 task の postgres 実装(memory と同値) | C1 | B1 共通スイート(memory/postgres 両方)+ postgres 専用 UNIQUE テスト。E1 統合テスト |
| R2 auth 3 集約 + authcode 単回使用/TTL | A1, C2 | B1 共通スイート + authcode の単回使用/TTL/並行 Consume 専用テスト。E1 統合テスト |
| R3 goose スキーマ + search_path 分離 | C1/C2 migrations, C3/D1 スキーマ作成 | E1 で goose up→down→up、統合テストが `search_path=api`/`auth` で疎通 |
| R4 sqlc 生成コードをコミット・build/vet/test 通過 | C1/C2 `make sqlc` | E2 checker が `make check`(build/vet/test に sqlcgen 含む) |
| R5 マイグレーション実行手段(local make / prod 一回限り) | C1/C2 Makefile, C3 ルート Makefile, D1 iac | E1(local)/ D2 CI で goose up。D1 plan で prod 手段 |
| R6 環境切替 + 本番 Postgres 必須 + 平文禁止 | C1/C2 選択ヘルパ(fail-closed), D1 Secrets 注入 | postgres 選択ロジックのユニット(DSN 組み立て/選択分岐)+ E3 security |
| R7 compose + ルート Makefile に postgres | C3 | `make db-up`/`make migrate` の疎通(E1 手動/ローカル) |
| R8 CI sqlc drift(+ migration 健全性) | D2 | `sqlc-drift.yml` の fail 検証、goose up→down→up job |
| R9 db.md コマンド表・CLAUDE.md 早見表 | F1, F2 | レビュー(review-spec)で反映確認 |

---

## 5. テスト戦略

### 5.1 レベルと配置

- **domain / service(ユニット・DB 非依存)**: 既存の fake/memory を継続使用。**変更しない・グリーン維持**(非機能「既存テスト維持」)。A1 で追加する `CodeChallenge.Challenge()` にはユニットテストを 1 本追加(impl-auth)。
- **`infra/memory`(既存)**: 既存テストをそのまま維持。共通スイート導入時も memory 側は無条件実行に組み込む。
- **`infra/postgres`(統合)**: `//go:build integration` で分離。`make test`(既定)は tag 無しでビルド対象外 → DB 無しでもグリーン。`make test-integration`(`go test -tags=integration ./infra/postgres/...`)で実 DB に対して実行。CI は postgres service container を立て、`make migrate-up` 後に実行。
- **選択ロジック/DSN 組み立て(ユニット・DB 非依存)**: `DB_HOST`/`APP_ENV` の分岐(postgres / memory / fail-closed)と DSN 文字列組成は tag 無しユニットで検証(実 DB 不要)。

### 5.2 TDD か後付けか(推奨)

- **共通振る舞い契約スイート = 先行作成(TDD)**。要件(memory と同値・単回使用・TTL)を先にテストとして固定し、impl-db 実装のゴールを明確化する。memory 側は既存実装で即グリーン、postgres 側は実装が揃うまで赤(build tag で通常 CI はブロックしない)。
- **`infra/postgres` の実 DB 統合テスト = 近接後追い**を推奨。理由: テーブル定義・sqlc 生成型・接続手順(migrate 前提)が impl-db の C1/C2 で初めて確定するため、スイートの骨子(観点・期待エラー)は B1 で先に書き、実 DB を叩く配線(接続/truncate/migrate 呼び出し)は C1/C2 直後に一体で仕上げる。純 TDD で全 DB 配線を先出しすると、確定前のスキーマに対する空振りメンテが増える。
- 観点は各リポジトリで 正常/異常/境界 を最低限カバー(testing.md)。

### 5.3 観点(抜粋)

| 集約 | 正常 | 異常 | 境界 |
|---|---|---|---|
| task | Upsert→FindByID/Title 一致、FindAll 件数、id 再 Save で更新 | FindByID/Title 該当なし→`ErrNotFound`、別 id で同 title Save→UNIQUE 違反(postgres 専用) | 空 FindAll、title 100 rune |
| client | Seed→FindByID 一致(jsonb⇔[]string 往復)| FindByID 該当なし→`ErrNotFound` | 空 scope/redirect の配列 |
| user | Seed→FindByID/FindByUsername 一致 | 該当なし→`ErrNotFound` | 大文字小文字・trim 済み username |
| authcode | Save→FindByCode(有効)一致、Consume→nil | 期限切れ→FindByCode `ErrNotFound`(+lazy 削除)、Consume 期限切れ→`ErrExpired`、二重 Consume→`ErrNotFound`、並行 Consume→単一 nil | TTL 境界(now 直前/直後) |

### 5.4 テスト分離・冪等性

- テスト間は truncate(または一意スキーマ/トランザクション rollback)でクリーン化。実時間 sleep 依存を避け、TTL 検証は `expires_at` を過去/未来に直接投入して評価(sleep しない)。
- CI 統合 job は毎回まっさらな service container + `make migrate-up`。

---

## 6. リスク / 未確定事項

### 6.1 planner が判断し確定した項目(実装で覆さない前提。ただしレビューで再評価対象)

- **R-a UNIQUE(title) と memory の非対称**: Spec §4 は「Title の一意制約を DB 制約に反映」を求めるが、memory は Save で dup title を黙認する。postgres は UNIQUE 違反でエラーになる。→ 共通スイートは両者共有の不変に限定し、UNIQUE は postgres 専用で検証(§5.3)。service 層の `DuplicateChecker` が通常経路で先に弾くため実害は限定的。**レビューで「制約を入れる/入れない」を最終確認**。
- **R-b demo user パスワードの plaintext at rest**: 既存設計(`user.password` は平文・`VerifyPassword` は現行フローで未使用)を Postgres に持ち込むと平文パスワードが DB に残る。本 Spec のスコープはストアの導入で、資格情報モデルの変更(ハッシュ化)は含まない。→ **現状踏襲**とし、ハッシュ化は将来 Issue。review-security の明示評価対象。
- **R-c search_path はアプリが master 資格情報で接続する限り「権限境界」ではない**: api/auth が同一 master ユーザで接続するため、search_path 分離は名前空間分離であって権限分離ではない(両スキーマにアクセス可能)。Spec のスコープ(名前空間分離)は満たすが、スキーマ毎の専用 DB ロールによる真の権限分離は **将来 Issue** として明記。
- **R-d 版ピンの実在性**: pgx `v5.7.2` / goose `v3.24.1` / sqlc `v1.28.0` はベースライン。オフラインで resolve 検証ができていない。→ **impl-db が実装時に `go run`/`go get` で resolve を確認し、不可なら最寄り stable に調整して Makefile(単一情報源)に確定記載**。E2 checker が build 通過で担保。

### 6.2 実装中に確認/決定が必要(impl 各 agent が閉じる)

- **R-e sqlc の jsonb / timestamptz マッピング**: `sql_package: database/sql` での `jsonb`→`[]byte`、`timestamptz`→`time.Time`、nullable 列→`sql.NullString`/ポインタの生成型を impl-db が確認。nullable(nonce)は `sql.NullString` 想定。ズレたら `overrides` を `sqlc.yaml` に追加。
- **R-f goose のバージョン表配置**: 各スタックのスキーマ内に `goose_db_version` を置く(api/auth 衝突回避)。goose 実行時に `search_path=<schema>` を確実に効かせる(DSN パラメータ)。スキーマが事前作成されていること(init script / iac)が前提 — この順序を C3/D1 で担保。
- **R-g prod マイグレーション実行方式**: init コンテナ(`dependsOn: SUCCESS`)推奨だが、並行タスク起動時の goose 競合(advisory lock)と `desired_count>1` の扱いを impl-iac が plan で明記。代替=一回限り ECS タスク(CI/CD or 手動トリガ)。
- **R-h migrate image の配布**: `Dockerfile.migrate` の image をどの ECR に push しどう参照するか。apply しないため plan 段階では task 定義に image URI 参照を置くのみ。実運用の push 経路(ルート Makefile 拡張)は本 Spec では plan 記述に留め、実配線は後続に委ねる(過剰スコープ回避)。impl-iac が plan コメントで明記。
- **R-i Dockerfile / CI の「stdlib-only」前提の陳腐化**: pgx 追加で go.sum が生成される。api/auth の `Dockerfile`(`COPY go.mod go.sum*` glob は対応済み)と `cicd.yml`/`contract-drift.yml` の `cache: false`・コメントを impl-ci が更新(module cache 有効化を推奨)。

### 6.3 ユーザー判断が必要(現時点で planner からの推奨あり)

- **なし(ブロッカー無し)**。上記 R-a / R-b / R-c は planner 推奨(現状踏襲 + 将来 Issue)で進められる。レビュー(E3)で覆る場合のみユーザーに再確認する。

---

## リファクタリング(2026-07-09): 別データベース + app/migrator

> このセクションは初回実装(commit `af2e2b2`)の**上への差分**である。§0〜§6 の初回計画は履歴として残し(過去エントリは編集しない)、以下でユーザー指示による 2 点の構成変更を計画する。SPEC-005 §6 経緯の末尾「設計変更(リファクタリング)」エントリと R3 / R5 / §4 の更新に対応する。

### RF.0 変更の要点(初回実装との差分)

| 観点 | 初回実装(af2e2b2) | 本リファクタリング後 |
|---|---|---|
| api ⇔ auth の分離 | 同一 database `app` 内の別スキーマ `api` / `auth`(接続 `search_path`) | **同一 RDS インスタンス上の別データベース `api` / `auth`**。各サービスは自 DB のみに接続。スキーマ分離は廃止 |
| 分離のための env | `DB_SCHEMA`(api=`api` / auth=`auth`)+ DSN の `search_path` | `DB_SCHEMA` **廃止**。`DB_NAME` を per-service 化(api=`api` / auth=`auth`)。DSN から `search_path` 除去 |
| マイグレーション実行体 | スタックごとの `Dockerfile.migrate` + `db/migrate-entrypoint.sh`(psql で `CREATE SCHEMA` → goose up)/ ローカルは各スタック `make migrate-up` | **新規 `app/migrator`(単一 Go バイナリ + 単一イメージ)**。`-target api\|auth` で(a)対象 DB を未存在なら作成(b)当該スタックの `db/migrations` を goose で適用 |
| DB / スキーマの作成主体 | local=compose init script(`docker/postgres/initdb`)/ prod=migrate イメージの entrypoint(`CREATE SCHEMA`) | **local / CI / prod とも `app/migrator` が `CREATE DATABASE`**(一貫性重視)。compose の init script は廃止 |
| goose の所在 | 各スタック `Makefile` の `go run goose@ver`(CLI・go.mod 非依存) | **`app/migrator/go.mod` の library require**(実行)+ 各スタック `Makefile` の `migrate-create` のみ `go run goose@ver`(ファイル雛形生成・DB 非依存)。api/auth の runtime require は pgx のみを維持(不変) |
| migration SQL / sqlc | 各スタック `db/migrations` / `db/queries` / `sqlc.yaml` / `infra/postgres/sqlcgen` | **無変更**(SQL は各スタックに残す。DDL は非修飾のまま。sqlc 生成物・`sqlc-drift.yml` も変わらない) |

### RF.1 方針

#### RF.1.1 別データベース分離(旧: 別スキーマ)

- **各サービスが自分専用の database に接続する。** api は DB `api`、auth は DB `auth`。両 DB は同一 RDS インスタンス(既存 `modules/db` の 1 台)上に置く。DDL は初回実装のまま**非修飾**(スキーマ名も database 名も書かない)で、接続先 database の `public` スキーマに素直に適用される。`search_path` 操作は一切不要になる。
- goose のバージョン表 `goose_db_version` は各 database の `public` に 1 つずつ置かれ、DB が分かれているので初回実装で気にしていた「同一 DB 内で api/auth の版表が衝突する」問題(§6.2 R-f)は**構造的に消える**。
- **退けた代替案**:

| 判断 | 採用 | 退けた案と理由 |
|---|---|---|
| 分離の粒度 | 別 **database**(同一インスタンス) | (a) 別スキーマ(初回実装)= master ユーザ共有では権限境界にならず(§6.2 R-c / ISSUE-016)、`search_path` の取り回し(DSN・goose・test・migrate)が全経路に染み出す。(b) 別 RDS インスタンス = コスト増でユーザー指示(「同一インスタンス上の別データベース」)に反する。別 database はバウンデッドコンテキストを DB 単位で分け、接続情報だけで境界が閉じる |
| DB 名 | `api` / `auth`(リテラル) | 旧スキーマ名を踏襲し混乱を避ける。将来 `var.*_db_name` 化する余地は残す(現状は不要な一般化を避けリテラル) |

#### RF.1.2 マイグレーション集約(app/migrator)

- **`app/migrator` を新規 Go モジュール(独立 `go.mod`)として追加**し、prod の `Dockerfile.migrate` + `db/migrate-entrypoint.sh`(api/auth 各 1)とローカルの各スタック `make migrate-up` を置換する。migrator は次を行う:
  1. **対象 DB の作成(未存在時)**: `CREATE DATABASE` は**トランザクション不可**なので、まずメンテナンス database(既定 `postgres`。RDS / compose / postgres image のいずれにも常在)へ接続し、`SELECT 1 FROM pg_database WHERE datname = $1` で存在確認 → 無ければ `CREATE DATABASE "<name>"` を autocommit で実行する。並行実行の競合は `SQLSTATE 42P04`(duplicate_database)を成功扱いにして冪等化する。identifier はパラメータ化できないため、DB 名を `^[a-z_][a-z0-9_]*$` で検証+クォートし injection を防ぐ。
  2. **当該スタックの `db/migrations` を適用**: 対象 database へ接続し直し、goose を**ライブラリとして**呼ぶ(`goose.SetDialect("postgres")` → `goose.RunContext(ctx, command, db, dir)`)。`search_path` は使わない。
- **goose の閉じ込め**: goose は `app/migrator/go.mod` の require に載る(library 実行)。**api/auth の `go.mod` は変更しない**ため、両モジュールの runtime require は `github.com/jackc/pgx/v5` のみ(価値検証 #4)を維持する。go workspace は無い(確認済み)ので、独立モジュールの追加は api/auth のビルド・依存に影響しない。ファイル雛形生成の `migrate-create` だけは各スタック `Makefile` に `go run goose@<ver>` CLI として残す(DB 非依存・go.mod 非依存)。**版の同期**: `app/migrator/go.mod` の goose と各スタック `Makefile` の `GOOSE_VERSION` は同じ版に揃える(コメントで相互参照)。
- **`-target` と `-command`**:
  - `-target api|auth`(必須): 適用する migrations dir(既定 `/migrations/<target>`、イメージ内)と既定の対象 DB 名(api→`api` / auth→`auth`)を選ぶ。
  - `-command up|down|status`(既定 `up`): goose のサブコマンド。`down`/`status` でも「DB 未存在なら作成」の冪等ステップは先に走る(既存なら no-op)ので、CI の up→down→up ヘルスチェックがそのまま通る。
  - `-migrations-dir <path>`: イメージ内は既定 `/migrations/<target>`。ローカル / CI は sibling を明示指定(例 `app/api/db/migrations`)。
- **接続 env(migrator)**: `DB_HOST` / `DB_PORT`(5432)/ `DB_USER` / `DB_PASSWORD` / `DB_SSLMODE`(disable)は app と共有。`DB_NAME`(既定=target 名)= 作成+適用する対象 DB。`DB_MAINTENANCE_NAME`(既定 `postgres`)= `CREATE DATABASE` 用の接続先。**`DB_SCHEMA` は持たない。** 失敗時は非 0 exit(ECS init コンテナの `SUCCESS` ゲートが機能する)。DSN / password をログに出さない。
- **イメージ**: multi-stage Go build →（psql 不要になったので）distroless / scratch 相当の最小 runtime。両スタックの migrations を `COPY app/api/db/migrations /migrations/api` / `COPY app/auth/db/migrations /migrations/auth` で焼き込む。**ビルドコンテキストはリポジトリルート**(`docker build -f app/migrator/Dockerfile .`)—migrator ソースと両スタックの migrations を同一コンテキストから COPY するため。ENTRYPOINT=`["/migrator"]`、`command`(ECS)で `-target` を渡す。
- **DB 作成主体の一元化**: local / CI / prod とも DB 作成は migrator に寄せる。compose の `docker/postgres/initdb/00-schemas.sql` は**廃止**。
  - **退けた代替案**: 「ローカルだけ compose init script で 2 DB を作る」= prod は init script を使えず DB 作成ロジックが 2 箇所に分裂する(migrator を作った意義が薄れる)。「iac(postgresql provider)で DB 作成」= private-subnet の RDS へ Terraform から SQL 実行できない(踏み台が要る)ため初回実装と同様に不採用。メンテナンス DB `postgres` が常在するので migrator 単独でブートストラップできる。

#### RF.1.3 不変条件(このリファクタで崩さない)

- api/auth の `go.mod` の runtime require は **pgx のみ**(goose は `app/migrator` に閉じる)。
- migration SQL(`db/migrations`)・`db/queries`・`sqlc.yaml`・`infra/postgres/sqlcgen` は**変更しない**(DDL は非修飾のまま)。`sqlc-drift.yml` も変わらない。
- `domain` / `service` / `route` は不変。`Repository` interface(ポート)も不変。`infra/memory` も不変(選択ロジックの env 契約のうち `DB_HOST`/`APP_ENV` の fail-closed は不変で、消えるのは `DB_SCHEMA` のみ)。
- `terraform apply` はしない(plan まで)。

### RF.2 変更・追加・削除ファイル一覧(stack ごと)

#### RF.2.1 app/migrator(新規・impl-db)

| 種別 | パス | 内容 |
|---|---|---|
| 追加 | `app/migrator/go.mod` / `go.sum` | module `github.com/srrrs-7/cc-orchestrator/app/migrator`。require = `github.com/pressly/goose/v3`(library)+ `github.com/jackc/pgx/v5`(driver)。**独立モジュール**(api/auth の go.mod に影響しない) |
| 追加 | `app/migrator/main.go`(+ 必要なら `run.go` / `database.go`) | CLI(`-target` / `-command` / `-migrations-dir`)+ env 読み取り + `ensureDatabase`(検証・クォート・42P04 冪等)+ goose library 実行。ロジックは薄く、配線は main に閉じる |
| 追加 | `app/migrator/Dockerfile` | multi-stage build。両スタック migrations を `/migrations/{api,auth}` に COPY。**ビルドコンテキスト=リポジトリルート**。最小 runtime(psql 不要) |
| 追加 | `app/migrator/Makefile` | `fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `check`(api/auth と同型)。migrator 単体の CI レーン用 |
| 追加 | `app/migrator/*_test.go` | tester: `-target`→dir/DB 名マッピング、identifier 検証/クォート、command パース、(任意)実 DB での CREATE DATABASE 冪等 + up/down/up |

#### RF.2.2 app/api(impl-db、テストは tester)

| 種別 | パス | 内容 |
|---|---|---|
| 変更 | `app/api/infra/postgres/db.go` | `Config.Schema` フィールド・`ConfigFromEnv` の `DB_SCHEMA` 読み取り・`DSN()` の `search_path` を**削除**。`DB_NAME` 既定は持たない(必須)。`SelectMode` / fail-closed は不変 |
| 変更 | `app/api/cmd/api/main.go` | persistence 配線の `slog.Info(..., "schema", cfg.Schema)` から `schema` を除去(impl-db。配線ブロックのみ) |
| 変更 | `app/api/Makefile` | `DB_SCHEMA` / `GOOSE_DBSTRING` の `search_path` を削除。`DB_NAME` 既定を `api` に。**`migrate-up` / `migrate-down` / `migrate-status` を削除**(実行は migrator へ)。`migrate-create`(`go run goose create`・DB 非依存)と `sqlc` / `test-integration` は残す |
| 変更 | `app/api/db/migrations/000001_create_tasks.sql` | **DDL は不変**。ヘッダコメントを「スキーマ(search_path)」→「専用 database」に更新 |
| 削除 | `app/api/Dockerfile.migrate` | migrator に統合 |
| 削除 | `app/api/db/migrate-entrypoint.sh` | migrator に統合(CREATE SCHEMA/psql 経路ごと廃止) |
| 変更(tester) | `app/api/infra/postgres/persistence_selection_test.go` | `DB_SCHEMA` / `search_path` を検証する箇所を削除・更新(DSN に search_path が無いことを確認) |
| 変更(tester) | `app/api/infra/postgres/task_repository_integration_test.go` | `testDSN()` から `DB_SCHEMA` / `search_path` 削除、`DB_NAME` 既定を `api` に。契約検証本体(repotest 呼び出し)は不変 |

#### RF.2.3 app/auth(impl-db、テストは tester)— api と対称

| 種別 | パス | 内容 |
|---|---|---|
| 変更 | `app/auth/infra/postgres/db.go` | `Schema` / `defaultSchema` / `DB_SCHEMA` / `search_path` を削除。`ConfigFromEnv` は `DB_SCHEMA` を読まない。`SelectMode`(引数版)は不変 |
| 変更 | `app/auth/Makefile` | api と同様(`DB_SCHEMA`・`search_path`・`migrate-up/down/status` 削除、`DB_NAME` 既定 `auth`、`migrate-create`/`sqlc`/`test-integration` 残置) |
| 変更 | `app/auth/db/migrations/000001_create_auth.sql` | DDL 不変。ヘッダコメントを database 分離に更新 |
| 削除 | `app/auth/Dockerfile.migrate` / `app/auth/db/migrate-entrypoint.sh` | migrator に統合 |
| 変更(tester) | `app/auth/infra/postgres/persistence_selection_test.go` | api と同様 |
| 変更(tester) | `app/auth/infra/postgres/testdb_integration_test.go` | `testDSN()` から `DB_SCHEMA`/`search_path` 削除、`DB_NAME` 既定 `auth`。`truncateTable` 等の本体は不変 |
| 不変 | `app/auth/infra/postgres/seed.go` ほか | seed / repository 実装は database 切替の影響を受けない(同じ DDL・同じクエリ) |

#### RF.2.4 リポジトリルート / compose(impl-db)

| 種別 | パス | 内容 |
|---|---|---|
| 変更 | `compose.yml` | postgres の initdb マウント(`./docker/postgres/initdb:...`)を削除。api env: `DB_SCHEMA` 削除・`DB_NAME=api`。auth env: `DB_SCHEMA` 削除・`DB_NAME=auth`。ヘッダコメントを「別スキーマ」→「別 database(migrator が作成)」に更新。`POSTGRES_DB` は maintenance 用に残置可(migrator は既定 `postgres` に接続) |
| 削除 | `docker/postgres/initdb/00-schemas.sql`(ディレクトリごと) | DB 作成を migrator に一元化 |
| 変更 | ルート `Makefile` | `migrate` を **`go run ./app/migrator -target api -migrations-dir app/api/db/migrations` + 同 auth** に置換(各スタックの `migrate-up` 呼び出しを廃止)。`db-up` / `up` / `up-d` の前提関係は不変。コメントを 2 DB + migrator に更新 |

#### RF.2.5 app/iac(impl-iac / plan まで)

| 種別 | パス | 内容 |
|---|---|---|
| 変更 | `app/iac/envs/dev/main.tf` | api service `environment`: `DB_SCHEMA=api` を削除し `DB_NAME="api"`(旧 `module.db.db_name`)に。auth service: `DB_SCHEMA=auth` 削除・`DB_NAME="auth"`。両 `migration_environment`: `DB_SCHEMA` 削除・`DB_NAME` を対象 DB 名に・`DB_MAINTENANCE_NAME`(`postgres` or `module.db.db_name`)追加。`migration_image` を**共有 migrator リポジトリ URL**に、`migration_command=["-target","api"|"auth"]` を渡す。コメントを database 分離に更新 |
| 追加 | migrator 用 ECR リポジトリ(`envs/dev` 直下 or 小 `modules/migrator`) | `app/migrator` イメージ 1 本を置く共有 ECR。api/auth 両 service が同一イメージを `-target` 違いで参照する。実装形は impl-iac 裁量 |
| 変更 | `app/iac/modules/service/variables.tf` | `migration_command`(list(string)、container `command`)変数を追加。`migration_environment`/`migration_secrets`/`migration_image` の description から schema 前提を除去。`migration_image` の「自リポジトリ `:migrate` 既定」の扱いを見直し(共有 migrator を caller が明示) |
| 変更 | `app/iac/modules/service/ecs.tf` | migration コンテナ定義に `command = var.migration_command`(非空時)を追加。他は不変 |
| 変更 | README(`modules/service` / `modules/db` / `envs/dev`) | 「スキーマブートストラップ」→「database ブートストラップ(migrator が `CREATE DATABASE`)」、単一 migrator イメージ + `-target`、`DB_NAME` per-service、`search_path` 記述の除去 |

#### RF.2.6 CI / ドキュメント

| 種別 | パス | 担当 | 内容 |
|---|---|---|---|
| 変更 | `.github/workflows/cicd.yml` | impl-ci | `changes` に `migrator`(`app/migrator/**` + workflow)追加。**新 `migrator` job**(setup-go + golangci-lint + `make check`、cache `app/migrator/go.sum`)。`api-integration`/`auth-integration`: 「Create schema」psql ステップを **migrator 実行**(`go run ./app/migrator -target <s> -migrations-dir app/<s>/db/migrations`= DB 作成+適用)に置換、`DB_SCHEMA` env を `DB_NAME`(api/auth)に、up→down→up を migrator `-command down`/`up` に。`app/migrator/**` 変更で両 integration も再実行するようフィルタ追加 |
| 不変 | `.github/workflows/sqlc-drift.yml` | impl-ci | migrations / sqlc 入力は不変のため**変更不要**(確認のみ) |
| 変更 | `.claude/rules/db.md` | impl-db | frontmatter `paths` に `app/migrator/**` 追加。「スキーマ分離」節→「database 分離」(DB_SCHEMA/search_path 廃止、DB_NAME per-service)。「マイグレーションの実行」節を `app/migrator`(`-target`/`-command`/`CREATE DATABASE`)へ書き換え。コマンド表から per-stack `migrate-up/down/status` を除去、migrator と root `make migrate` を記載。版行(goose は app/migrator に所在)更新 |
| 変更 | `CLAUDE.md` | admin | リポジトリ概要に `app/migrator` を追記、`make migrate` が migrator 経由・goose は app/migrator に閉じる旨を早見表/概要に反映(admin のメタ整備範囲) |
| 更新 | `docs/issues/20260709-017-*.md` | issue-creator | 経緯に「migrate イメージが per-stack `:migrate` 2 本 → 共有 `app/migrator` 単一イメージに変わった」を追記(push 経路の未配線は引き続き open。対応内容の参照を単一イメージ前提へ更新) |

### RF.3 手順(agent 別・順序と並列可否)

> 依存の核は **impl-db が確定する「env 契約(`DB_SCHEMA` 廃止・`DB_NAME` per-service)」と「migrator の CLI / イメージ契約(`-target`/`-command`/イメージ名)」**。iac・ci・tester はこれに従属する。

#### フェーズ RA: 核となる env / migrator 契約(impl-db、内部は一部並列)
- **RA1 (impl-db)**: `app/migrator` を新規実装(go.mod/go.sum・main.go・Dockerfile・Makefile)。`ensureDatabase` + goose library 実行。**この段で `-target`/`-command`/env/イメージ名の契約を確定**(iac・ci が待つ)。
- **RA2 (impl-db)**: api / auth の `db.go` から `DB_SCHEMA`/`search_path`/`Schema` を除去、`DB_NAME` per-service 化、api `main.go` の slog 修正。**api と auth は独立で並列可**。
- **RA3 (impl-db)**: 各スタック `Makefile`(migrate-up/down/status 削除・DB_NAME 既定・migrate-create 残置)、`compose.yml`、ルート `Makefile`(migrate→migrator)、`docker/postgres/initdb` 削除、`Dockerfile.migrate` / `migrate-entrypoint.sh` 削除、migrations のヘッダコメント。RA1/RA2 の命名確定後。

#### フェーズ RB: テスト更新(tester、RA の契約確定後に着手)
- **RB1 (tester)**: `persistence_selection_test.go`(api/auth)から `DB_SCHEMA`/`search_path` 検証を除去し、search_path 不在を確認。`testDSN()`(api/auth integration)を `DB_NAME` per-service へ更新。**契約スイート(repotest)は不変**。
- **RB2 (tester)**: `app/migrator` の単体テスト(target マッピング・identifier 検証・command パース)を追加。実 DB 冪等/ up→down→up は CI の postgres で担保(任意でローカル integration)。
  - RB1 は RA2、RB2 は RA1 に従属。RB1 / RB2 は互いに独立で並列可。

#### フェーズ RC: インフラ・CI(RA 契約に従属、互いに並列)
- **RC1 (impl-iac / plan まで)**: envs/dev(DB_NAME per-service・migration_environment の schema 除去 + DB_MAINTENANCE_NAME・migration_image を共有 migrator に・migration_command)、modules/service(`migration_command` 変数 + ecs.tf)、migrator ECR、README 群。`make plan`(dev)で差分確認、**replace 等の破壊的変更を報告**、**apply しない**。
- **RC2 (impl-ci)**: cicd.yml(migrator job + changes filter + integration job の migrator 化 + DB_NAME 化)。sqlc-drift は不変確認のみ。
  - **RC1 と RC2 は並列可**(RA1 のイメージ名・`-target`/DB_NAME 契約に共通従属)。

#### フェーズ RD: 検証・レビュー(直列ゲート)
- **RD1 (tester)**: 各スタック `make test`(memory 経路・不変)+ api/auth `make test-integration`(migrator で DB 作成+適用後)+ `app/migrator` `make test`。
- **RD2 (checker)**: `app/api` / `app/auth` / `app/migrator` の `make check`、iac `make check ENV=dev`。**api/auth の go.mod に pgx 以外の runtime require が増えていないこと**を明示確認(価値検証 #4 の維持)。
- **RD3 (review-security / performance / spec、並列、RD2 通過後)**:
  - security: 平文接続情報の不在、`CREATE DATABASE` の identifier injection 防止(検証+クォート)、別 DB でも master ユーザ共有のため権限境界ではない点(ISSUE-016 の再確認)。
  - performance: migrator の 2 接続(maintenance→target)コスト、init コンテナ毎回 migrate。
  - spec: R3(別 database)/ R5(app/migrator)/ 価値検証 #1〜#4 の充足、`DB_SCHEMA` 残存の不在。
- **RD4 (指摘対応)**: Blocker/Major は impl-db / impl-iac / impl-ci に差し戻し、RD1→RD3 を再実行。今回対応しない指摘は issue-creator が起票。

#### フェーズ RE: 記録
- **RE1 (impl-db)**: `.claude/rules/db.md` を database 分離 + migrator 版に更新(frontmatter `paths` 追加含む)。
- **RE2 (issue-creator)**: ISSUE-017 の経緯に単一 migrator イメージ化を追記。
- **RE3 (admin)**: `CLAUDE.md` 反映。SPEC-005 §5(refactor タスク要約)と §6 経緯を spec skill 手順で更新(status は `in-progress` 継続、価値検証 #1〜#4 を実 DB で再確認できたら `done`)。

### RF.4 要件 → 手順・テストの対応(refactor 差分)

| 要件 / 検証 | 手順 | テストでの担保 |
|---|---|---|
| R3(api/auth = 別 database、`search_path` 廃止) | RA2, RA3, RC1 | RB1(DSN に search_path 無し)、RD1 の統合テスト(各 DB へ疎通)、RD3-spec |
| R5(`app/migrator` が `-target` で DB 作成 + migrations 適用、goose は migrator に閉じる) | RA1, RA3, RC1, RC2 | RB2(migrator 単体)、CI の migrator 実行 + up→down→up、RD2 の go.mod pgx-only 確認 |
| 価値検証 #1(再起動後もデータ保持) | 変わらず(別 DB でも永続化不変) | RD1 統合テスト + 手動確認(compose) |
| 価値検証 #2(memory と同値の契約テスト green) | repotest 不変 | RB は契約スイートを変更しない。RD1 で green 維持 |
| 価値検証 #3(sqlc drift が fail) | 変わらず | sqlc-drift 不変(RC2 は確認のみ) |
| 価値検証 #4(runtime require は pgx のみ) | RA1(goose を app/migrator に隔離) | RD2 で api/auth go.mod を明示確認 |

### RF.5 テスト戦略

- **既存の振る舞い契約スイート(`infra/repotest` + memory/postgres 統合)は DB 非依存の「振る舞い」検証であり流用する。** リポジトリの振る舞い(同値・単回使用・TTL)は database 分離で一切変わらないため、契約スイート本体は無改修。
- **変わるのは 3 点のみ**: (1) DSN 組み立て(search_path 除去)= `persistence_selection_test.go`、(2) 統合テストの接続セットアップ(`testDSN` の `DB_NAME` per-service・schema 除去)、(3) 統合セットアップ(schema 作成 → migrator の DB 作成 + 適用)。いずれも tester(RB1)/ impl-ci(RC2)が担う。
- **`app/migrator` は新規テスト対象**(RB2): DB 非依存の純ロジック(`-target`→dir/DB 名、identifier 検証/クォート、`-command` パース、42P04 冪等の分岐)を table-driven ユニットで、実 DB 依存(実際の `CREATE DATABASE` 冪等・goose up/down/up)は CI の postgres service で担保。実時間 sleep に依存しない。
- **TDD か後付けか**: env 契約の除去(search_path)は挙動を「減らす」変更なので、**テストを先に赤化**(search_path を期待する assertion を落とす)してから db.go を直す TDD が素直。migrator は新規実装のため、CLI/identifier 契約のユニットを先に書く(TDD)。実 DB 統合は初回計画と同じく「近接後追い」。
- **観点**: migrator の identifier 検証は 正常(`api`/`auth`)/ 異常(空・記号・SQL メタ文字)/ 境界(先頭数字・最大長)を最低限カバー。

### RF.6 リスク / 未確定事項

#### RF.6.1 planner が確定した項目(実装で覆さない前提・レビューで再評価可)
- **RF-a メンテナンス DB の実在**: migrator の `CREATE DATABASE` 接続先は既定 `postgres`。compose(postgres image)・CI(postgres service)・RDS はいずれも `postgres` database を持ち、接続ユーザ(compose 超ユーザ / RDS master)は CREATEDB 権限を持つ、という前提。**RDS で `postgres` が使えない場合は `DB_MAINTENANCE_NAME=<bootstrap db_name>`(既定 `app`)に上書き**できるよう env 化しておく(impl-iac が plan で選択)。
- **RF-b 単一 migrator イメージ + `-target`**: prod は per-stack `:migrate` 2 本ではなく共有 `app/migrator` 1 本を api/auth の migration コンテナが `-target` 違いで参照する。ECR は 1 リポジトリで足りる。**ISSUE-017 はこの単一イメージ前提へ更新**(push 経路の未配線は依然 open)。
- **RF-c 別 database でも権限境界ではない**: api/auth が同一 RDS master ユーザで接続する限り、別 database でも相互アクセスは技術的に可能(ISSUE-016 の m/R-c と同じ)。database 分離は名前空間 + 独立スキーマ進化の分離であって権限分離ではない。DB ごとの専用ロール発行は将来 Issue(ISSUE-016 追跡)。
- **RF-d データ移行は不要**: 初回実装は実 RDS へ apply 済みでなく(prod は ISSUE-017 でブロック)、永続データが無いため schema→database の移行対象は無い。既存 running への影響も無い。

#### RF.6.2 実装中に確認/決定(impl 各 agent が閉じる)
- **RF-e goose library API の版**: `goose.RunContext` / `goose.SetDialect` のシグネチャは pressly/goose v3 の版に依存。`app/migrator/go.mod` の goose 版を確定し、スタック `Makefile` の `GOOSE_VERSION`(migrate-create 用 CLI)と揃える(impl-db)。
- **RF-f migration コンテナの `command` 注入**: `modules/service` に `migration_command` を足して ECS container `command` に流す。`terraform plan` でタスク定義の差分(コンテナ定義変更 = 新リビジョン)と新 ECR の追加を確認・報告(impl-iac、apply しない)。
- **RF-g CI の作業ディレクトリ**: `api-integration` は `working-directory: app/api` だが migrator 実行はリポジトリルート基点(`go run ./app/migrator ...`)。ステップ単位で working-directory を上書きするか `-migrations-dir` を相対解決するかは impl-ci が決める。
- **RF-h `app/migrator` の rule / agent 所有**: 新 agent は作らず **impl-db が所有**(Go の永続化/マイグレーションツールで db.md の担務に収まる)。db.md の frontmatter `paths` に `app/migrator/**` を追加してルールを適用させる(RE1)。

#### RF.6.3 ユーザー判断が必要
- **なし(ブロッカー無し)**。DB 分離方式(別 database)・migrator 集約はユーザー確定済み。RF-a の maintenance DB 名と RF-b の ECR 構成は planner 推奨で進められる(レビューで覆る場合のみ再確認)。
