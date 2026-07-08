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
  `app` モジュール参照)
- 自前で `random_password` を生成し Secrets Manager にカスタムシークレットとして
  格納する代替案もあるが、RDS 標準機能で完結する方がシンプルでローテーションの
  面倒も RDS 側に任せられるため、サンプル実装として `manage_master_user_password` を採用した

### 暗号化・バックアップ方針

- `storage_encrypted = true`(既定 KMS キーである `aws/rds` を使用)。カスタマー管理キー(CMK)
  を使う場合は追加のキー管理コストと運用が発生するため、サンプルでは AWS 管理キーを採用
- `backup_retention_period` は既定 1 日(最小限)。本番相当の要件がある場合は延長すること
- `skip_final_snapshot = true`(既定)で dev 環境を使い捨てにしやすくしている。破棄時に
  最終スナップショットを残したい場合は変数で切り替える

## 既知の制約

- `app/api` は現状 in-memory リポジトリで実際には RDS に接続しない(ISSUE-001 参照)。
  本モジュールは Spec の要件どおり RDS を構築し、接続情報(エンドポイント / シークレット ARN)を
  `app` モジュールのタスク定義に配線するところまでを行う。実際の DB 接続実装は将来の
  `app/api` 側の対応(別 Issue)に委ねる
