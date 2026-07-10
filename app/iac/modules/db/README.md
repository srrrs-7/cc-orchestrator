# db module

RDS PostgreSQL(single-AZ, db.t4g.micro を既定)、DB サブネットグループ、パラメータグループを
作成する。

## コスト上の選択理由

### RDS(db.t4g.micro, single-AZ)を採用し Aurora Serverless v2 を退けた理由

- Aurora Serverless v2 は最小 0.5 ACU でも 約$43/月(us-east-1 目安、東京リージョンはさらに
  高め)かかる。ACU あたりの単価が高く、常時起動する小規模サンプルでは割高
- RDS `db.t4g.micro`(Graviton/ARM ベース)は 約$12/月 前後(東京リージョン目安、単一 AZ)と
  大幅に安く、開発・検証用途のスループットには十分
- Multi-AZ にすると RDS のコストは概ね 2 倍になる。本サンプルでは可用性よりコストを優先し
  既定で `multi_az = false`(single-AZ)としている。可用性が必要な環境では
  `multi_az = true` に変数で切り替えるだけでよい(モジュールインターフェースは変更不要)

### `manage_master_user_password = true` を採用した理由(R4)

- マスターパスワードをコード・tfvars・state に平文で書かないため、RDS 自身が
  Secrets Manager にシークレットを生成・ローテーション管理する機能を使う
- アプリケーション側はタスク定義の `secrets` 経由でこの ARN を参照し、実行時にのみ
  複合値を取得する(タスク実行ロールに `secretsmanager:GetSecretValue` を付与。
  `service` モジュール(api インスタンス)の `secret_read_arns` 参照)
- 自前で `random_password` を生成し Secrets Manager にカスタムシークレットとして
  格納する代替案もあるが、RDS 標準機能で完結する方がシンプルでローテーションの
  面倒も RDS 側に任せられるため、サンプル実装として `manage_master_user_password` を採用した
  (この判断は**マスターユーザ**についてのもの。api/auth ランタイム用の最小権限ロール
  `api_app`/`auth_app` は、RDS 側にこの機能が無く自分でロールを発行する必要があるため
  `random_password` を使う。下記「最小権限ランタイムロール」参照)

### 暗号化・バックアップ方針

- `storage_encrypted = true`(既定 KMS キーである `aws/rds` を使用)。カスタマー管理キー(CMK)
  を使う場合は追加のキー管理コストと運用が発生するため、サンプルでは AWS 管理キーを採用
- `backup_retention_period` は既定 1 日(最小限)。本番相当の要件がある場合は延長すること
- `skip_final_snapshot = true`(既定)で dev 環境を使い捨てにしやすくしている。破棄時に
  最終スナップショットを残したい場合は変数で切り替える

## api・auth は同一 RDS インスタンス上の別データベースに分かれる(SPEC-005)

SPEC-004 時点では `app/api` は in-memory リポジトリのままで実際には RDS に接続せず、auth
インスタンスは RDS を全く使わなかった。SPEC-005 の初回実装では `app/api`・`app/auth` の双方が
`infra/postgres`(goose マイグレーション + sqlc 生成コード)を持ち、同一データベース内の別スキーマ
(`api` / `auth`、`search_path` で選択)に分かれて接続する形を採った。**その後のユーザー指示による
リファクタリングで、api / auth は同一 RDS インスタンス上の別データベース**(`DB_NAME` =
api=`"api"` / auth=`"auth"`)**に分離**され、スキーマ・`search_path` は廃止された
(バウンデッドコンテキストの分離をデータベース単位で表現。SPEC-005 plan RF.1.1)。
`var.db_name`(既定 `"app"`)はインスタンス作成時に RDS が用意する初期データベースで、api/auth
どちらのアプリからも直接使われない(migrator の `DB_MAINTENANCE_NAME` フォールバック候補として
残置。下記参照)。`api` / `auth` の各データベース自体は Terraform ではなく `app/migrator`
(共有マイグレーションイメージ)が起動時に `CREATE DATABASE` で作成する(下記「マイグレーション
実行」参照)。`envs/dev/main.tf` は `module.db` の出力(`db_endpoint` / `db_port` /
`master_user_secret_arn`)を `module.service_api` / `module.service_auth` の双方に配線し、
どちらのタスク定義にも `DB_HOST`/`DB_PORT`/`DB_SSLMODE` は同一値、`DB_NAME` だけが `"api"`/`"auth"`
で異なる値として渡る(sqlc/goose の DDL・クエリは変わらず非修飾のまま。接続先データベースの
`public` スキーマに素直に適用される)。

### 別データベースでも権限境界ではなかった(SPEC-005 plan RF.6.1 RF-c → ISSUE-016 R-c で解消)

SPEC-005 初回実装では api・auth はいずれも同じ RDS マスターユーザ(`var.master_username`、資格情報は
`manage_master_user_password` が管理する同一シークレット)で接続していた。そのため database 分離は
**名前空間の分離であって権限の分離ではなかった**: api の接続情報を使えば `auth` データベースにも
(逆も同様に)アクセスできてしまう(初回実装のスキーマ分離時点から変わらない性質。SPEC-005 plan
初回 §6.2 R-c / RF.6.1 RF-c、将来 Issue として送られていたものが ISSUE-016 として起票された)。
ISSUE-016 の対応でこの権限境界を実装した(下記「最小権限ランタイムロール」参照)。

## 最小権限ランタイムロール(ISSUE-016 R-c)

api/auth の**アプリコンテナ**は、RDS マスターユーザではなく**自分の database のみに閉じた
最小権限ロール**(`api_app` / `auth_app`、既定値。`var.api_app_role_name` /
`var.auth_app_role_name` で変更可)で接続する。マスターユーザは**マイグレーション実行
(migrate init コンテナ)専用**として残す(`CREATE DATABASE` / `CREATE ROLE` / `GRANT` に
`CREATEROLE` 相当の権限が必要なため。詳細は `modules/service/README.md` の
「database ブートストラップ」)。

### ロールの作成場所: Terraform ではなく app/migrator

RDS は `publicly_accessible = false` の private subnet 内にあり、`terraform apply`/`plan` を
実行する場所(CI ランナー・開発者端末)から直接到達できない(既存の「データベース自体は
Terraform では作れない」制約と同じ理由。後述「マイグレーション実行」参照)。そのため
`cyrilgdn/postgresql` のような provider で ROLE/GRANT を宣言的に管理する方式は採らず、
**本モジュールが生成・保管するのはロールの資格情報(パスワード + 専用 Secrets Manager
シークレット)のみ**で、ロールそのものの作成(`CREATE ROLE` / `ALTER ROLE ... PASSWORD` /
クロス DB `CONNECT` の REVOKE/GRANT / DML の GRANT)は、既に VPC 内でマスター接続を行っている
`app/migrator` の migrate init コンテナが冪等に行う(`envs/dev` の `migration_secrets` に
`APP_DB_USER`/`APP_DB_PASSWORD` としてこの secret を渡す。詳細は
`docs/plans/ISSUE-016-plan.md` §1.2/§1.3)。

### secret 分離

api/auth 各ロールぶんに専用の `aws_secretsmanager_secret`(+ `secret_version`)を作成し、
master user secret と同じ JSON 形状 `{"username","password"}` で格納する(既存の
`:username::`/`:password::` valueFrom 構文をそのまま流用できる)。`envs/dev` の配線:

- api アプリコンテナ: `DB_USER`/`DB_PASSWORD` を **`api_app_secret_arn`** から注入(master
  secret 参照は撤去)。auth も同様に **`auth_app_secret_arn`**。
- migrate コンテナ: `DB_USER`/`DB_PASSWORD` は引き続き **`master_user_secret_arn`**(CREATE
  DATABASE/ROLE/GRANT に必要)。加えて、作るべきロールの資格情報を `APP_DB_USER`/
  `APP_DB_PASSWORD` として同じ **scoped secret**(api なら `api_app_secret_arn`、auth なら
  `auth_app_secret_arn`)から注入する。

### state にパスワードが載るトレードオフ

`random_password` はパスワードが Terraform **state に平文で載る**(現行 backend は S3 +
`encrypt=true`、`envs/dev/versions.tf`)。マスターユーザは `manage_master_user_password` により
state 非搭載だったため、この点は後退する。代替として「`app/migrator` 自身がパスワードを生成し
Secrets Manager に `PutSecretValue` で書く」方式も検討したが、`app/migrator` に AWS SDK 依存が
入り「runtime 依存は pgx のみ」原則(`.claude/rules/db.md`)を破るため退けた。state の暗号化・
アクセス制御(S3 バケットポリシー・IAM)でこのトレードオフを許容する(ISSUE-016 で明示合意
済み)。

### 残差(将来 Issue へ)

- **IAM 層**: api/auth の task execution role は migrate コンテナのために master secret を
  読めてしまう(app コンテナ自身は master 資格情報を env として受け取らないが、IAM 上は読取可)。
  真の IAM 分離にはマイグレーションを独立 ECS タスク(`RunTask`)に切り出し、その task execution
  role だけが master secret を読める構成が必要(`modules/service/README.md`「代替案」参照)。
- **migrator ロールの絞り込み**: migrator(migrate init コンテナ)は当面マスター資格情報を
  継続使用する。専用 `migrator` ロール(`CREATEDB` + api/auth 両 DB の owner、非 superuser)へ
  縮小する案は将来検討(`docs/plans/ISSUE-016-plan.md` §1.1 第 2 段)。

### マイグレーション実行(SPEC-005 R5 / RF.1.2)

対象データベースの作成(`CREATE DATABASE`)・マイグレーション適用(goose)はいずれも
Terraform(本モジュール)の責務ではなく、`modules/service` が提供する init コンテナ経由で
共有 `app/migrator` イメージ(単一 Go バイナリ、`-target api`/`-target auth` で対象を切り替え)が
ECS タスク自身の中で行う。詳細・設計判断(database ブートストラップの方式・並行実行時の注意・
代替案)は `modules/service/README.md` の「マイグレーション init コンテナ」を参照。
