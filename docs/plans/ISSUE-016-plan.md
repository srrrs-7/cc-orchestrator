# ISSUE-016 実装計画 — 最小権限 DB ユーザー(R-c)

- 起点: `docs/issues/20260709-016-auth-db-connection-least-privilege-and-tls-hardening.md`
- 対象スコープ: 本 Issue の残タスク **(R-c) 最小権限 DB ユーザー**のみ。もう一方の (m-2) `DB_SSLMODE` fail-open は 2026-07-10 の修正ラウンドで解消済み(既定を `"require"` に変更、経緯セクション参照)。本計画では扱わない。
- 関連: SPEC-005(app/api・app/auth の Postgres 永続化)、SPEC-005 plan §6.2 R-c / RF.6.1 RF-c(「別 database でも master 共有では権限境界ではない」と明記され将来 Issue に送られた項目)。
- 作成日: 2026-07-10

---

## 0. 現状(調査結果)

- api・auth・migrator の 3 経路がいずれも **RDS マスターユーザ**(`module.db.master_user_secret_arn` の同一シークレット)で接続する。分離は `DB_NAME=api`/`auth`(別データベース)のみで、これは名前空間分離であって権限境界ではない(`app/iac/envs/dev/main.tf` の `module.service_api`:151-152 / `module.service_auth`:249-250、`app/iac/modules/db/README.md:56-63`)。api の接続情報が漏れれば auth データベースも読み書きできる(逆も同様)。
- RDS は `publicly_accessible = false` で private subnet 内(`app/iac/modules/db/main.tf:58`)。**Terraform を実行する場所(CI ランナー / 開発者端末)から RDS に直接ネットワーク到達できない**(VPN / 踏み台なし)。この制約は既に `app/iac/modules/service/README.md:149-168` に記録済みで、データベース作成を Terraform ではなく VPC 内で動く `app/migrator`(ECS init コンテナ)に委ねた理由そのもの。**この制約が本計画のプロビジョニング方式選定を決定づける。**
- `app/migrator` は既に VPC 内でマスター資格情報を用い、冪等な `CREATE DATABASE`(存在確認 `pg_database` → `CREATE` → 競合 SQLSTATE 分類、`app/migrator/infra/postgres/database.go`)を行っている。ロール作成 / GRANT を冪等に足す自然な受け皿。
- migrator の runtime 依存は `pgx` のみ(`.claude/rules/db.md`「新規 runtime 依存は pgx のみ」)。ドメイン層 `domain/migration` に 2 ポート `Database{EnsureExists}` / `Runner{Run}`、application 層 `service.Migrate` が両者を協調(`app/migrator/service/migrate.go`)。
- app 側(`app/api`・`app/auth`)は `DB_USER`/`DB_PASSWORD` を env(ECS `secrets` 経由)から読むだけで、**どのロールで接続するかにコードは依存しない**。ランタイムロールの差し替えは原理上 iac(task definition の `valueFrom`)のみで完結し、api/auth のアプリコード変更は不要。

---

## 1. 方針

### 1.1 ロール分離の設計(3 ロール)

RDS 上に以下を作る。命名は暫定で、impl-db / impl-iac が確定する。

| ロール | 権限 | 接続先 | 用途 |
|---|---|---|---|
| `api_app` | `LOGIN`、`CONNECT` on database `api` のみ、`USAGE` on schema `public`、対象テーブルへ `SELECT/INSERT/UPDATE/DELETE`、SERIAL 用 sequence へ `USAGE,SELECT`。**`CREATEDB`/`CREATEROLE`/`SUPERUSER` なし、DDL(`CREATE`)権限なし** | database `api` | api ランタイム |
| `auth_app` | 同上を database `auth` に対して | database `auth` | auth ランタイム |
| migrator | `CREATE DATABASE`(bootstrap)+ 対象 DB への DDL(goose) | maintenance + 各 DB | マイグレーション実行(init コンテナ) |

真の権限境界にするための REVOKE/GRANT の要点(Postgres 16 = RDS engine 16.4 前提):

- **クロスデータベース遮断**: 既定では全ロールが `PUBLIC` 経由で任意データベースに `CONNECT` できる。`REVOKE CONNECT ON DATABASE api FROM PUBLIC` / 同 `auth`、そのうえで `GRANT CONNECT ON DATABASE api TO api_app`(auth も同様)。これで `api_app` は `auth` データベースへ接続できない(名前空間分離が権限境界に昇格する核心)。
- **DDL 遮断**: Postgres 15+ は `public` スキーマの `CREATE` を既定で `PUBLIC` に付与しない(16.4 は該当)。加えて `api_app` に `CREATE ON SCHEMA public` を与えないことで DDL 不可を担保。
- **既存 + 将来オブジェクトへの DML 付与**: goose 適用でテーブルが作られる。`GRANT ... ON ALL TABLES IN SCHEMA public TO api_app` で既存分、`ALTER DEFAULT PRIVILEGES ... GRANT ... TO api_app` で以後 migrator が作る分を自動付与(sequence も同様)。→ **GRANT は goose up の後に流す**(順序が重要、§3 の実行フロー参照)。

**migrator ロールの絞り込み(「マスターより絞る余地」への回答)**:

- 第 1 段(本計画の推奨・必須): **migrator は当面マスター資格情報を継続使用**する。理由 — (a) `CREATE ROLE`/`GRANT`/`REVOKE`(=他ロールの権限を左右する操作)には `CREATEROLE` 相当が要り、そもそもロール群を最初に作れるのはマスターだけ(ブートストラップの鶏卵)。(b) migrator は `dependsOn: SUCCESS` でゲートされる短命 init コンテナで、常時稼働の攻撃面ではない。今回の実害(常時稼働の api/auth 両アプリがマスター共有 = 片方漏洩で全 DB 侵害)を最優先で閉じる。
- 第 2 段(将来・任意、退けず記録): 専用 `migrator` ロール(`CREATEDB` + api/auth 両 DB の owner、非 superuser)を導入し init コンテナをそれで動かす。ただし (i) その `migrator` ロール自体の作成は結局マスターが要る、(ii) IAM レベルでマスター secret を読めるのは migrate 実行主体のみに限定したい → **より強い境界は「マイグレーションを init コンテナから独立した一回限り ECS タスク(RunTask)に分離し、その task execution role だけがマスター secret を読める」構成**(`modules/service/README.md:190-194` に既出の代替案)。本 Issue のスコープ外とし、§6 リスクに残差として明記。

### 1.2 プロビジョニング方式(重要な設計判断)

| 候補 | pros | cons | 判定 |
|---|---|---|---|
| **A. Terraform `cyrilgdn/postgresql` provider で ROLE/GRANT を宣言的管理** | 権限をコードで宣言・drift 検知可能。terraform plan に権限差分が出る | **RDS が private・到達不能**(§0)。plan/apply する CI・端末から接続できず、VPN/踏み台の新設が必要で運用コスト大。パスワードも provider に渡す=state 露出。データベース作成を migrator に委ねた既存判断と矛盾 | **退ける**(到達性が致命的) |
| **B. `app/migrator` を拡張し ROLE/GRANT を冪等適用**(推奨) | 既に VPC 内でマスター接続 + 冪等 DDL(`ensureDatabase`)を実施済みで受け皿が自然。新 provider・VPN 不要。`dependsOn: SUCCESS` の fail-closed ゲートに乗る。CREATE DATABASE と同じ設計パターン | migrator にロジック追加。パスワードの供給元を別途用意する必要(§1.3) | **採用** |
| C. 手動 runbook(psql で CREATE ROLE/GRANT) | 実装ゼロ | 非再現・drift の温床・秘密の手作業扱い。CI で検証不能 | **退ける**(初回の緊急手段としてのみ §6 に併記) |

**採用 = B**。migrator に「対象データベースの `public` スキーマに対し、対象ロールを冪等に用意し最小権限を付与する」責務を追加する。`ensureDatabase`(存在確認→作成→競合分類)と同じ冪等パターンで、`CREATE ROLE` に `IF NOT EXISTS` が無い点は `pg_roles` の事前 SELECT + `ALTER ROLE ... PASSWORD`(パスワード同期)で吸収する。

### 1.3 secret 管理(資格情報の分離・注入)

パスワードは「ロールを作る側(migrator)」と「そのロールで接続する側(api/auth アプリ)」の両方が知る必要があり、単一ソースから両者へ配る:

- **採用**: Terraform で `random_password` を api/auth 各ロール分生成 → 専用 `aws_secretsmanager_secret`(+ `secret_version`)に **master secret と同じ JSON 形状 `{"username","password"}`** で格納(既存の `:username::`/`:password::` valueFrom 構文をそのまま流用できる)。
  - api アプリコンテナ: `DB_USER`/`DB_PASSWORD` を **api 専用 secret** から注入(master secret 参照を撤去)。auth も同様に auth 専用 secret。
  - migrate コンテナ: `DB_USER`/`DB_PASSWORD` は **master secret**(CREATE DATABASE/ROLE/GRANT に必要)を継続。加えて「作るべきロールの資格情報」を別 env(例 `APP_DB_USER`/`APP_DB_PASSWORD`)として **api/auth 専用 secret** から注入し、migrator が `ALTER ROLE <app> PASSWORD ...` で同期する。
  - `secret_read_arns`: api サービス = `[master_secret, api_app_secret]`、auth = `[master_secret, auth_app_secret]`。
- **トレードオフ(明記)**: `random_password` はパスワードが Terraform **state に載る**(現行 backend は S3 + `encrypt=true`、`app/iac/envs/dev/versions.tf`)。master は `manage_master_user_password` で state 非搭載だったので、この点は後退。
- **退けた代替**: migrator 自身がパスワードを生成し Secrets Manager に `PutSecretValue` で書く案 → state 露出は避けられるが、**migrator に AWS SDK 依存**が入り「runtime 依存は pgx のみ」原則(`.claude/rules/db.md`)を破る + IAM 書き込み権限が要る。**退ける**。
- **残差(IAM 層)**: api サービスの task execution role は(migrate コンテナのために)master secret を読めてしまう。app コンテナ自身は master 資格情報を env として受け取らないが、IAM 上は読める。真の IAM 分離は §1.1 第 2 段の「マイグレーション独立タスク」構成が必要。§6 に残す。

### 1.4 apply 順序(依存)

1. `module.db`(RDS)+ 新 secret 群を作成。
2. `module.service_*`(task definition が新 secret を参照)を作成 / 更新。
3. デプロイ後、ECS が migrate init コンテナを起動 → マスターで (a) `CREATE DATABASE`(既存)、(b) `goose up`(既存)、(c) **ロール ensure + GRANT(新規)** を実行 → SUCCESS。
4. その後アプリコンテナが**スコープドロール**で接続開始。

順序 3→4 は既存の `dependsOn: SUCCESS`(実行時ゲート)が担保し、Terraform の apply 順序では制御しない。**`terraform apply` は agent が実行せず、`terraform plan` の結果を報告してユーザーに apply 判断を委ねる**(§4 手順に明記)。migrator イメージ push が前提である点も既存どおり(未 push だと init コンテナが pull 失敗、`modules/service/README.md:176-180`)。

---

## 2. 変更ファイル(stack ごと)

### app/iac(impl-iac)

- `app/iac/modules/db/`(main.tf / variables.tf / outputs.tf / README.md)
  - api/auth 各ロールの `random_password` + `aws_secretsmanager_secret` + `aws_secretsmanager_secret_version`(JSON `{username,password}`)を追加。ロール名は変数化(既定 `api_app`/`auth_app`)。secret ARN を outputs で公開。
  - README に「ロール分離・secret 分離・state にパスワードが載るトレードオフ・migrator がロールを実際に作る」旨を追記(既存の「権限境界ではない」節を更新)。
  - 代替案: 新 `random_password`/secret を `envs/dev` 側に置く形も可。**db モジュールに置く**方が RDS 資格情報群として凝集度が高い(planner 推奨)。impl-iac が最終判断。
- `app/iac/envs/dev/main.tf`
  - `module.service_api`: アプリ `secrets` の `DB_USER`/`DB_PASSWORD` を **api 専用 secret** 参照へ変更。`migration_secrets` にマスター(既存)+ `APP_DB_USER`/`APP_DB_PASSWORD`(api 専用 secret)を追加。`secret_read_arns` に api 専用 secret ARN を追加。同等の変更を `module.service_auth`(auth 専用 secret)にも。
  - コメント(現状「別 DB でも権限境界ではない」)を新設計に合わせて更新。
- `app/iac/envs/dev/variables.tf` / `terraform.tfvars`: 必要ならロール名変数を追加。
- `app/iac/modules/service/`: 既存の `secrets`/`migration_secrets`/`secret_read_arns` 変数で足りる想定(追加コンテナ env は `migration_secrets` に名前を増やすだけ)。**モジュールインターフェース変更は不要**であることを impl-iac が確認する。不要なら変更なし。

### app/migrator(impl-db)

- `domain/migration/`: ランタイムロールを表す VO(例 `AppRole`:名前 + 対象 `DatabaseName`、identifier 検証は既存 `DatabaseName` の allowlist + quoting パターンを踏襲)を追加。`Target` から対応ロール名を導出。ロール権限プロビジョニング用ポート(例 `Provisioner{EnsureRole(ctx, role, password) / GrantLeastPrivilege(ctx, role, db)}`。既存 `Database`/`Runner` と並置)を追加。
- `infra/postgres/`: 上記ポートの実装。`pg_roles` 存在確認 → `CREATE ROLE ... LOGIN` / `ALTER ROLE ... PASSWORD`、`REVOKE CONNECT ... FROM PUBLIC` + `GRANT CONNECT`、`GRANT USAGE`/DML on tables + sequences、`ALTER DEFAULT PRIVILEGES`。identifier はパラメータ化不可のため既存 `Quoted()` 方式でクォート、パスワードは `pgx` のクォート/リテラルエスケープを使い**ログに出さない**。冪等・競合安全(並行 init コンテナ)を `ensureDatabase` と同水準で担保。
- `service/migrate.go`: `goose up` の**後**にロール ensure + GRANT を呼ぶよう協調フローを拡張(EnsureExists → Run → EnsureRole/Grant)。`-command down`/`status` 時の扱い(GRANT はスキップ or up 時のみ)を定義。
- `cmd/migrator/env.go`: `APP_DB_USER`/`APP_DB_PASSWORD`(作成するロールの資格情報)を読み `Env`/`validate` に追加。**未設定時の後方互換**(ロール未指定なら従来どおりロール作成をスキップし CREATE DATABASE + goose のみ)を明記して既存経路を壊さない。
- `Dockerfile` は変更不要見込み(ロジック追加のみ)。

### app/api・app/auth(原則変更なし)

- ランタイムはロールに非依存(§0)。**アプリコード変更は不要**。もし「api_app で DDL 不可」を明示する統合テストをアプリ側に置く判断になった場合のみ、`infra/postgres` の integration test に追加(impl-db/tester が判断)。

### .github(impl-ci、必要な場合のみ)

- `api-integration`/`auth-integration` ジョブ(`.github/workflows/cicd.yml`)が migrator 経由でロール作成まで通るよう、必要なら `APP_DB_USER`/`APP_DB_PASSWORD` を job env に追加。権限境界の実効性検証を CI 統合テストで行う場合、その step 追加。

### docs(planner→issue-creator)

- 本ファイル。加えて実 Issue 反映(§4「対応」への要約 + 本 plan 参照、frontmatter `updated`)は **issue-creator が後で** `issue` skill 手順で実施(admin/planner は直接編集しない)。

---

## 3. 手順(agent 別・順序)

依存関係を明示。**フェーズ 1 と 2 は概ね並列可**(iac のロール名 / secret 出力名の契約だけ先に握る)。

1. **[planner→admin]** 本計画の合意。ロール名(`api_app`/`auth_app`)・secret 名・env 名(`APP_DB_USER`/`APP_DB_PASSWORD`)の命名を確定(impl-iac と impl-db の接続契約)。
2. **[tester]**(TDD 先行)
   - migrator の新ロジック(ロール冪等 ensure・GRANT・パスワード同期・`APP_DB_*` 未設定時スキップ)の Go 単体テストを、既存 `database_test.go` と同様の fake ポート / table-driven で先に作成。
   - 権限境界の統合テスト観点(scoped ロールで DML 可・DDL 不可・他 DB へ CONNECT 不可)を定義。
3. **[impl-db]**(app/migrator。フェーズ 1)
   - `domain/migration` に role VO / provisioner ポート追加 → `infra/postgres` に実装 → `service.Migrate` フロー拡張 → `cmd/migrator/env.go` に `APP_DB_*` 追加(未設定後方互換)。2 のテストを green に。
4. **[impl-iac]**(app/iac。フェーズ 2、3 と並列可)
   - `modules/db` に `random_password`/secret/outputs 追加 → `envs/dev/main.tf` の api/auth `secrets`/`migration_secrets`/`secret_read_arns` を差し替え → README/コメント更新。
5. **[impl-ci]**(必要時)CI 統合ジョブに `APP_DB_*` env / 権限境界検証 step を追加。
6. **[tester]** 実行: `app/migrator` の `make check` + 単体テスト、権限境界の統合テスト(CI の postgres service 上、§5)。iac は §5 の validate/plan。
7. **[checker]** `app/migrator` `make check`、`app/iac` `make check`(fmt-check/validate/lint/security)。**green まで 6→7 を回す。checker 未通過でレビューに進まない。**
8. **[review-security]**(+ 必要に応じ review-spec)権限境界(クロス DB 遮断・DDL 不可・secret 分離)、パスワードの非ログ / 非平文、state トレードオフの妥当性、残差(task execution role が master を読める点)の評価。
9. **[impl-*]** Blocker/Major 差し戻し対応(6→8 再実行)。今回対応しない指摘は issue-creator が別 Issue 化。
10. **[admin]** `terraform plan` 結果 + 変更点をユーザーに報告し、**apply 判断を仰ぐ**(agent は apply しない)。migrator イメージ push が apply 前提である点も併記。
11. **[issue-creator]** ISSUE-016 の §4 と経緯を `issue` skill 手順で更新。(R-c) 解消をもって本 Issue を close 可能(§7)。

---

## 4. テスト戦略

- **TDD**: migrator の新ロジックは §3-2 で**先行作成**。iac はコード生成物でないため validate/plan による確認(後追い)。
- **単体(impl-db / tester, Go)**: `app/migrator` の role ensure / GRANT / パスワード同期を table-driven + fake ポートで検証。観点 = 正常(新規作成)/ 異常(不正 identifier 拒否・接続失敗の伝播)/ 境界(既存ロール再実行の冪等・`APP_DB_*` 未設定でスキップ・並行実行の競合安全)。パスワードがエラーメッセージ / ログに出ないことを検証。
- **統合(CI, postgres service container)**: `cicd.yml` の `api-integration`/`auth-integration` を土台に、(i) migrator でロール作成 + GRANT を適用 → (ii) **scoped ロールで接続**し「対象テーブルへ DML 可」「`CREATE TABLE` 等 DDL が権限エラー」「他 DB へ `CONNECT` 不可」を assert。要件 (R-c) の実効性はここで担保。
- **iac(checker/tester)**: `app/iac` で `make fmt-check` / `make validate` / `make lint` / `make security`、`make plan`(**apply しない**)。plan の差分が「新 secret 追加」「task def の valueFrom 変更」のみで、**破壊的変更(RDS replace 等)が無い**ことを確認・報告。secret 追加や task def 変更が既存 api/auth の再作成を誘発しないかを重点確認。
- **実 RDS の最終実効性**: private RDS ゆえ CI では代替不可。**手動 runbook**(踏み台 or セッションマネージャ経由で psql、または一時的な検証タスク)で「api_app が auth DB に接続できない / DDL できない」を確認する手順を README/runbook に残す(§6 の到達性制約より、実 RDS 検証は手動が現実解)。

### 要件との対応

| 要件 | 手順 | テスト |
|---|---|---|
| (R-c) api/auth が自 DB のみ・DML のみのロールで接続 | §3-3(migrator ロール/GRANT)+ §3-4(iac secret 差し替え) | 統合(scoped ロールで DML 可 / DDL 不可 / 他 DB 不可)、実 RDS は手動 runbook |
| ロール資格情報の secret 分離・master 参照撤去(app) | §3-4(iac) | iac plan で valueFrom が専用 secret を指すこと |
| migrator が高権限を要する点の最小化検討 | §1.1 第 2 段(将来)として記録 | 対象外(残差) |

---

## 5. リスク / 未確定事項

- **[要ユーザー判断] state へのパスワード露出**: `random_password` 方式は Terraform state にロールパスワードが載る(現 backend は S3 + encrypt)。master の no-state 特性からの後退を許容するか、より重い「migrator が生成し PutSecretValue(AWS SDK 依存増)」を採るか、最終的にユーザーに確認する。planner 推奨は前者(pgx-only 原則維持)。
- **[要ユーザー判断] migrator の権限縮小の深さ**: 本計画は migrator をマスター継続(第 1 段)で、ランタイム 2 ロールの分離を優先。専用 migrator ロール / マイグレーション独立タスク化(IAM で master 読取を実行主体に限定)まで踏むかはスコープ判断。planner 推奨は第 1 段のみを本 Issue で実施し第 2 段を将来 Issue 化。
- **残差(IAM 層)**: api/auth の task execution role は migrate コンテナのため master secret を読める(§1.3)。app コンテナは master 資格情報を受け取らないが IAM 上は読取可。完全分離には独立マイグレーションタスクが必要。
- **`ALTER DEFAULT PRIVILEGES` の付与者**: 既定権限は「オブジェクトを作るロール」に紐づく。goose を master で流す限り master が作るテーブルに対する default privileges を設定する必要がある(`ALTER DEFAULT PRIVILEGES FOR ROLE <master> ...`)。migrator ロールを将来分離すると付与者も変わる点に注意。impl-db が実装時に検証。
- **DB_MAINTENANCE_NAME / CREATEROLE 前提**: `CREATE ROLE`/`GRANT` はマスター(`rds_superuser` 相当、CREATEROLE 保持)前提。RDS のマスターがこれらを行える前提を実 RDS で確認(SPEC-005 plan RF-a と同種の前提)。
- **既存デプロイへの影響**: 既に master で接続中の api/auth を scoped ロールに切替える初回は、migrate コンテナがロール作成 + GRANT を終える前にアプリが起動しないよう `dependsOn: SUCCESS` を再確認。ロールが未作成のまま app が起動すると接続失敗 → fail(fail-closed 側なので安全だが要認識)。
- **未確定**: ロール名 / secret 名 / env 名(`APP_DB_*`)の最終命名は §3-1 で impl-iac・impl-db 間の契約として確定する。
- **apply gate**: 本計画は `terraform plan` まで。**agent は `terraform apply` を実行しない**。plan 差分(新 secret・task def valueFrom 変更、破壊的変更の有無)と、migrator イメージ push が apply 前提である点を admin がユーザーに報告し、apply はユーザーが判断する。

---

## 6. ISSUE-016 への back-reference 方針

- 本 plan 確定後、**issue-creator が** `issue` skill の手順で ISSUE-016 を更新する(planner/admin は Issue を直接編集しない):
  - §4「実施内容(将来対応時のチェックリスト)」の (R-c / 最小権限)項目に、本計画の要約(migrator によるロール/GRANT プロビジョニング採用・scoped secret 分離・migrator は当面 master 継続)と `docs/plans/ISSUE-016-plan.md` への参照を追記。
  - 経緯セクションに新エントリ(日付・計画化した旨・採用方式・残差)を**追記のみ**で記録(過去エントリは編集しない)。
  - frontmatter は `status` を実装フェーズ入り時に `fixing` へ、`updated` を更新。
- (R-c) が実装・レビュー完了した時点で、(m-2) は既に解消済みのため **ISSUE-016 全体を close 可能**(経緯 2026-07-10 の「(R-c) 解消で本 Issue をクローズ可能」に対応)。
