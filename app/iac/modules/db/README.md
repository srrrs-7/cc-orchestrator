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

### 暗号化・バックアップ方針

- `storage_encrypted = true`(既定 KMS キーである `aws/rds` を使用)。カスタマー管理キー(CMK)
  を使う場合は追加のキー管理コストと運用が発生するため、サンプルでは AWS 管理キーを採用
- `backup_retention_period` は既定 1 日(最小限)。本番相当の要件がある場合は延長すること
- `skip_final_snapshot = true`(既定)で dev 環境を使い捨てにしやすくしている。破棄時に
  最終スナップショットを残したい場合は変数で切り替える

## api・auth の両方が同一データベースを共有する(SPEC-005)

SPEC-004 時点では `app/api` は in-memory リポジトリのままで実際には RDS に接続せず、auth
インスタンスは RDS を全く使わなかった。SPEC-005 で `app/api`・`app/auth` の双方が
`infra/postgres`(goose マイグレーション + sqlc 生成コード)を持ち、**同一 RDS インスタンスの
単一データベース(`var.db_name`)内で、別スキーマ(`api` / `auth`)に分かれて接続する**ように
なった(バウンデッドコンテキストの分離をスキーマ単位で表現。ユーザー確定・SPEC-005 R3)。
`envs/dev/main.tf` は `module.db` の出力(`db_endpoint` / `db_port` / `db_name` /
`master_user_secret_arn`)を `module.service_api` / `module.service_auth` の双方に配線し、
どちらのタスク定義にも `DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_SSLMODE` は同一値、`DB_SCHEMA` だけが
`"api"`/`"auth"` で異なる値として渡る(接続の `search_path` でスキーマを選択する。sqlc/goose の
DDL・クエリは非修飾のまま)。

### スキーマ分離は権限境界ではない(SPEC-005 plan §6.2 R-c)

api・auth はいずれも同じ RDS マスターユーザ(`var.master_username`、資格情報は
`manage_master_user_password` が管理する同一シークレット)で接続する。そのため `DB_SCHEMA` /
`search_path` による分離は **名前空間の分離であって権限の分離ではない**: api の接続情報を使えば
`auth` スキーマにも(逆も同様に)アクセスできてしまう。スキーマごとに専用の DB ロールを発行し、
最小権限で権限境界を作る改善は本 Spec のスコープ外とし、将来 Issue として扱う(review-security
の明示評価対象、SPEC-005 plan §6.2 R-c)。

### マイグレーション実行(SPEC-005 R5)

goose によるスキーマ作成・マイグレーション適用は Terraform(本モジュール)の責務ではなく、
`modules/service` が提供する init コンテナ経由で ECS タスク自身が行う。詳細・設計判断
(スキーマブートストラップの方式・並行実行時の注意・代替案)は
`modules/service/README.md` の「マイグレーション init コンテナ」を参照。
