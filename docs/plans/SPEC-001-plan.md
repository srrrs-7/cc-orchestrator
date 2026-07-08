# SPEC-001 実装計画: AWS ECS API インフラ(CloudFront → WAF → ECS → PostgreSQL)

- 起点: `docs/specs/20260708-001-aws-ecs-api-infra.md`(status: approved)
- 対象 stack: `app/iac`(Terraform)。他 stack のコードは変更しない
- 成果物: `app/iac` 配下の modules / envs/dev 一式(`terraform plan` 可能なリファレンス実装)

## 方針

### 採用アプローチ

Spec 4.「方針」(安全側の設計は保ちつつ固定費の大きい要素を削る)を、以下の具体的な設計で実装する。

1. **モジュール分割は Spec 想定の 4 分割(network / db / app / cdn)を採用**する。ただし責務境界を以下のように明確化する:
   - `network`: VPC / IGW / public・private サブネット / ルートテーブル / **セキュリティグループ(ALB・ECS・RDS の 3 つ)とそのルール** / CloudFront origin-facing マネージドプレフィックスリストの data source。
   - `db`: RDS PostgreSQL インスタンス、DB サブネットグループ、パラメータグループ。
   - `app`: ALB / Target Group / Listener(+ カスタムヘッダ検証ルール)/ ECR / ECS クラスタ / タスク定義 / サービス / IAM ロール / CloudWatch Logs。
   - `cdn`: WAFv2 Web ACL(CLOUDFRONT scope, us-east-1)/ CloudFront ディストリビューション。

2. **セキュリティグループは network モジュールに集約**する(下記「退けた代替案」参照)。SG 本体は `aws_security_group`、ルールは `aws_vpc_security_group_ingress_rule` / `aws_vpc_security_group_egress_rule` を**分離**して定義し、SG 間の相互参照による循環依存を回避する。
   - ALB SG: ingress = CloudFront origin-facing プレフィックスリストのみ(80/tcp)、egress = ECS SG 宛(8080/tcp)。
   - ECS SG: ingress = ALB SG のみ(8080/tcp)、egress = RDS SG 宛(5432/tcp)+ 0.0.0.0/0(ECR / CloudWatch / Secrets Manager への outbound。NAT 不使用のため IGW 経由で必要)。
   - RDS SG: ingress = ECS SG のみ(5432/tcp)。

3. **CloudFront → ALB のオリジン保護は「プレフィックスリスト SG + カスタムヘッダ検証」の二重防御**とする(R3)。カスタムヘッダの秘密値は `envs/dev` ルートで `random_password` により生成し、`app`(ALB Listener ルールの条件)と `cdn`(CloudFront オリジンカスタムヘッダ)の**両モジュールに変数として渡す**。これにより両モジュール間の直接依存を作らず、秘密値の生成元を 1 箇所に保つ。tfvars には平文で書かない(state はセンシティブ扱い)。
   - ALB Listener の default action は `fixed-response 403`、カスタムヘッダが一致した場合のみ Target Group へ forward する Listener ルールを追加する。

4. **WAF の us-east-1 要件は provider alias で解決**する。`envs/dev` に default provider(`var.region`、例: ap-northeast-1)と alias provider `aws.us_east_1` を定義し、`cdn` モジュールに両方を `providers` で渡す。`cdn` モジュールは `required_providers` に `configuration_aliases = [aws.us_east_1]` を宣言し、WAFv2(CLOUDFRONT scope)を us_east_1 provider で、CloudFront ディストリビューションを default provider で作成する。

5. **ECS タスクのヘルスチェックとポート**:
   - コンテナポート = **8080**(`app/api/cmd/api/main.go` の `defaultPort`)。
   - **app/api には専用ヘルスチェックエンドポイントが存在しない**(`route/router.go` のルートは `/tasks` 系のみ)。ALB Target Group のヘルスチェックパスは **`GET /tasks`(200 を返す)** を使用し、matcher = `200` とする。この判断はリスク欄にも記載し、将来 `app/api` に `/healthz` を追加する改善は別 Issue とする(本計画では app/api を変更しない)。
   - コンテナイメージはビルド対象外(Spec スコープ外: ECR リポジトリの用意まで)。タスク定義の image は `var.container_image`(プレースホルダ既定値、ECR リポジトリ URL:tag を想定)で参照する。

6. **DB 認証は `manage_master_user_password = true`**(R4)。RDS が Secrets Manager にマスターユーザーシークレットを自動生成する。tfvars にパスワードを書かない。
   - **注記(重要)**: 現状の `app/api` は in-memory リポジトリ(`infra/memory`)で、RDS に接続しない。Spec の要件どおり RDS は参照アーキテクチャの一部として作成し、DB 接続情報(シークレット ARN / エンドポイント)はタスク定義に env / secret として配線する(将来のアプリ拡張のための布石)。この乖離はリスク欄に明記する。

7. **NAT Gateway 不使用**(R6)。ECS サービスの `network_configuration` で `assign_public_ip = true`、subnets = public、security_groups = ECS SG。RDS は private サブネット(DB サブネットグループ)に配置。

8. **バージョン固定・タグ・backend**(iac.md 準拠):
   - `required_version`(例: `>= 1.9.0`)、`required_providers`(aws `~> 5.x`、random `~> 3.x`)を固定。
   - 全リソースに共通タグ(`Environment`・`Project`・`ManagedBy = "terraform"`)を `default_tags` またはローカル変数で付与。
   - S3 backend 設定を `versions.tf` にコードとして含める(バケット名等はプレースホルダ。詳細はリスク欄)。
   - `count` より `for_each` 優先(サブネット・SG ルール等)。

### 退けた代替案

| 案 | 退けた理由 |
|---|---|
| SG を各所有モジュール(app に ALB/ECS SG、db に RDS SG)に配置 | ALB↔ECS↔RDS の SG が相互参照するため、モジュール間で循環依存が発生しやすい。network に集約し ingress/egress ルールを分離定義することで循環を断ち、境界資源の所有を 1 モジュールに寄せる |
| カスタムヘッダの秘密値を各モジュール内で個別生成 | app と cdn で値が一致しないと検証が成立しない。生成元をルートに 1 本化し両モジュールへ配布する |
| カスタムヘッダ秘密値を Secrets Manager に格納 | サンプルでは random_password の state 管理で十分。Secrets Manager 化は将来の改善余地としてリスク欄に記載 |
| WAF を default provider で作成 | WAFv2 CLOUDFRONT scope は us-east-1 必須のため不可。provider alias で対応 |
| ALB Listener を HTTPS 化(ACM 証明書) | Spec スコープ外(カスタムドメイン/ACM なし)。CloudFront → ALB 間 HTTP のトレードオフは README に明記 |
| tester(terratest 等)で TDD | AWS 認証情報がなく、Go テストハーネスも未整備。検証は checker(fmt/validate/tflint/trivy)に集約(テスト戦略欄参照) |

## 変更ファイル(すべて新規作成、`app/iac` 配下)

各モジュールは iac.md のレイアウトどおり `main.tf / variables.tf / outputs.tf / README.md` を持つ。`main.tf` は責務ごとに複数ファイル(例: `alb.tf` / `ecs.tf` / `iam.tf`)へ分割してよい。

### modules/network/
- `main.tf`: VPC、IGW、public サブネット(for_each で複数 AZ)、private サブネット(for_each)、ルートテーブル(public は IGW 向けデフォルトルート、private はローカルのみ)+ 関連付け、`aws_security_group`(alb / ecs / rds)+ `aws_vpc_security_group_ingress_rule` / `_egress_rule`、`data "aws_ec2_managed_prefix_list" "cloudfront"`(name = `com.amazonaws.global.cloudfront.origin-facing`)。
- `variables.tf`: `vpc_cidr`、`azs`、`public_subnet_cidrs`、`private_subnet_cidrs`、`name_prefix`、`tags`、`container_port`(8080)、`db_port`(5432)。
- `outputs.tf`: `vpc_id`、`public_subnet_ids`、`private_subnet_ids`、`alb_sg_id`、`ecs_sg_id`、`rds_sg_id`。
- `README.md`: NAT 不使用(パブリックサブネット + public IP + SG 制限)のコスト理由、SG を network に集約した理由、プレフィックスリストによる ALB 保護の説明。

### modules/db/
- `main.tf`: `aws_db_subnet_group`(private サブネット)、`aws_db_instance`(engine postgres、`db.t4g.micro`、single-AZ、`manage_master_user_password = true`、`storage_encrypted = true`、`deletion_protection`/`skip_final_snapshot` は変数化)、必要なら `aws_db_parameter_group`。
- `variables.tf`: `name_prefix`、`private_subnet_ids`、`rds_sg_id`、`instance_class`(db.t4g.micro)、`allocated_storage`、`engine_version`、`db_name`、`multi_az`(既定 false)、`deletion_protection`、`skip_final_snapshot`、`tags`。
- `outputs.tf`: `db_endpoint`、`db_port`、`db_name`、`master_user_secret_arn`(`master_user_secret[0].secret_arn`)。
- `README.md`: RDS(db.t4g.micro / single-AZ)を選び Aurora Serverless v2 を退けたコスト理由、`manage_master_user_password` による Secrets Manager 管理、暗号化方針。

### modules/app/
- `main.tf`(必要に応じ `alb.tf` / `ecs.tf` / `iam.tf` に分割):
  - ALB(internet-facing、public サブネット、ALB SG)、Target Group(target_type=ip、port 8080、health_check path=`/tasks` matcher=200)、HTTP Listener(port 80、default action=fixed-response 403)、Listener ルール(条件: `http_header` = カスタムヘッダ名/値、action: forward → TG)。
  - ECR リポジトリ(`aws_ecr_repository`、image scanning on)。
  - CloudWatch Logs ロググループ。
  - IAM: タスク実行ロール(`AmazonECSTaskExecutionRolePolicy` + Secrets Manager シークレット読取許可)、タスクロール(最小)。
  - ECS クラスタ、タスク定義(Fargate、`runtime_platform` cpu_architecture=ARM64 / os=LINUX、cpu/memory 変数、container_definitions に image・port 8080・log 設定・DB シークレット/エンドポイントの env・secrets 配線)、ECS サービス(`capacity_provider_strategy` で FARGATE / FARGATE_SPOT を dev では Spot 重み付き、network_configuration: public サブネット + assign_public_ip=true + ECS SG)。
- `variables.tf`: `name_prefix`、`vpc_id`、`public_subnet_ids`、`alb_sg_id`、`ecs_sg_id`、`container_image`、`container_port`(8080)、`task_cpu`、`task_memory`、`desired_count`、`use_fargate_spot`(bool)、`spot_weight`/`ondemand_weight`、`origin_verify_header_name`、`origin_verify_header_value`(sensitive)、`db_secret_arn`、`db_endpoint`、`db_name`、`health_check_path`(既定 `/tasks`)、`log_retention_days`、`tags`。
- `outputs.tf`: `alb_dns_name`、`alb_arn`、`ecr_repository_url`、`ecs_cluster_name`、`ecs_service_name`、`target_group_arn`。
- `README.md`: Fargate ARM64 + Spot(dev)を選び EC2 起動タイプを退けた理由、カスタムヘッダ検証(fixed-response 403 + forward ルール)の仕組み、CloudFront → ALB 間 HTTP のトレードオフ、ヘルスチェックに `/tasks` を使う理由と `/healthz` 追加の将来課題。

### modules/cdn/
- `main.tf`:
  - `required_providers` に aws の `configuration_aliases = [aws.us_east_1]` を宣言。
  - `aws_wafv2_web_acl`(provider = aws.us_east_1、scope=CLOUDFRONT):AWS マネージドルール `AWSManagedRulesCommonRuleSet` / `AWSManagedRulesAmazonIpReputationList`、`rate_based_statement`(レート制限)、`visibility_config`。
  - `aws_cloudfront_distribution`(default provider):オリジン=ALB DNS(custom_origin_config: HTTP only、port 80)、カスタムオリジンヘッダ(検証用ヘッダ名/値)、default_cache_behavior(viewer_protocol_policy=redirect-to-https、キャッシュ無効の managed policy、AllViewerExceptHostHeader 相当の origin request policy)、`web_acl_id` = Web ACL ARN、`price_class`(PriceClass_100)。
- `variables.tf`: `name_prefix`、`alb_dns_name`、`origin_verify_header_name`、`origin_verify_header_value`(sensitive)、`waf_rate_limit`、`price_class`、`tags`。
- `outputs.tf`: `cloudfront_domain_name`、`cloudfront_distribution_id`、`web_acl_arn`。
- `README.md`: WAF が us-east-1 provider alias を要する理由、適用マネージドルールとレート制限の意図、CloudFront デフォルトドメイン利用(カスタムドメイン/ACM 非対応)のトレードオフ。

### envs/dev/
- `versions.tf`: `required_version`、`required_providers`(aws `~> 5.x`、random `~> 3.x`)、`backend "s3"`(bucket / key / region / dynamodb_table または use_lockfile。値はプレースホルダ、リスク欄参照)。
- `providers.tf`: `provider "aws"`(region = var.region、default_tags)、`provider "aws" { alias = "us_east_1", region = "us-east-1" }`。
- `main.tf`: `random_password "origin_verify"`、`locals`(name_prefix・共通タグ)、`module "network"` / `module "db"` / `module "app"` / `module "cdn"`(cdn には `providers = { aws = aws, aws.us_east_1 = aws.us_east_1 }` を渡す)。
- `variables.tf`: 全変数に type/description(region、project、environment、vpc_cidr、azs、subnet cidrs、instance_class、task_cpu/memory、desired_count、use_fargate_spot、container_image、waf_rate_limit、price_class、log_retention_days 等)。
- `outputs.tf`: `cloudfront_domain_name`、`alb_dns_name`、`ecr_repository_url`、`rds_endpoint`、`ecs_cluster_name` 等。
- `terraform.tfvars`: dev 値(region=ap-northeast-1、azs=2、db.t4g.micro、最小タスクサイズ、use_fargate_spot=true 等)。**秘密情報は書かない**。
- `README.md`: 環境の使い方(`terraform init` / `plan`)、S3 backend バケットの事前作成が必要な旨(bootstrap 手順)、`terraform apply` は人間判断で行うこと、認証情報なしでの検証手順(`init -backend=false` + `validate`)。

## 手順

依存関係を踏まえ、以下の順で進める。フェーズ内の並列可否を明示する。

1. **impl-iac(単一 agent、逐次)— 全モジュールと envs/dev の実装**
   - 実装対象がモジュール間で型・出力の整合を取り合う(network の SG/subnet 出力 → app/db、app の alb_dns 出力 → cdn、ルートの random_password → app/cdn)ため、**1 つの impl-iac に一括委譲**する(モジュールを別 agent に割ると interface 不整合が起きやすい)。
   - 実装順の推奨: `network` → `db` → `app` → `cdn` → `envs/dev`(依存の下流から上流へ)。各モジュールの README(コスト理由・トレードオフ、R8)も同時に作成する。
2. **checker(逐次)— 静的検証**(下記テスト戦略のコマンド)。fmt / validate / tflint / trivy を `app/iac/envs/dev` 起点で実行。失敗があれば impl-iac に差し戻して 1↔2 を反復。
3. **review-security / review-performance / review-spec(3 者並列)— レビュー**
   - review-security: SG の最小権限、ALB の CloudFront 限定(R3)、WAF ルール(R2)、Secrets Manager 平文なし(R4)、public IP 露出の妥当性、S3 backend 暗号化。
   - review-performance: タスクサイズ・desired_count・Spot 構成(R5)、CloudFront キャッシュ方針、単一 AZ DB のコスト/可用性トレードオフ、$50/月 目安との整合。
   - review-spec: R1〜R8 の充足確認(下記トレーサビリティ表)、スコープ外項目に踏み込んでいないか。
   - **前提**: checker(2)が通過してからレビューへ進む(フェーズ飛ばし禁止)。
4. **指摘対応(逐次)**: Blocker / Major は impl-iac に差し戻し、2 → 3 を再実行。今回対応しない指摘は issue-creator が Issue 起票。
5. **完了処理**: admin が Spec の frontmatter status を `done` に更新し、「6. 経緯」に skill 手順で追記(planner の担当外。ここでは手順として記載)。

### 要件トレーサビリティ(R → 実装 / 検証)

| 要件 | 実装箇所 | 検証 |
|---|---|---|
| R1 経路 CF→WAF→ALB→ECS→RDS | network / cdn / app / db 全体 | validate / plan(要 creds)/ review-spec |
| R2 WAF マネージド+レート制限 | cdn(wafv2 web acl) | validate / trivy / review-security |
| R3 ALB を CF 経由のみ | network(prefix list SG)+ app(header ルール)+ cdn(origin header) | validate / review-security |
| R4 RDS Secrets Manager 管理 | db(`manage_master_user_password`) | trivy(平文検出なし)/ review-security |
| R5 Fargate ARM64 + Spot | app(runtime_platform / capacity_provider) | validate / review-performance |
| R6 NAT 不使用 | network(IGW のみ)+ app(assign_public_ip) | plan(NAT 資源なし)/ review-spec |
| R7 modules + envs/dev、tfvars 差分 | 全体レイアウト | fmt / validate / review-spec |
| R8 各モジュール README にコスト理由 | 各 module/README.md | review-spec |

## テスト戦略

**結論: tester による先行テスト(TDD)は行わず、checker による静的検証に集約する。**

- 理由:
  1. 成果物は AWS リソースを宣言する Terraform サンプルで、単体テスト対象のロジックを持たない。
  2. terratest 等の Go テストハーネスは未整備で、実リソース検証には AWS 認証情報が必要だが本環境には無い。
  3. testing.md の iac 方針(`terraform validate` + `terraform plan` で差分確認)に沿い、静的検証 + plan を検証手段とする。plan は creds 依存のため、agent は plan まで自動化せず、apply/plan 判断は人間に委ねる(iac.md)。
- **AWS 認証情報なしで実行可能な検証(checker が実行)**:
  - `terraform init -backend=false`(S3 backend を無効化し、provider のみ取得。creds 不要)
  - `terraform fmt -check -recursive`
  - `terraform validate`
  - `tflint --recursive`
  - `trivy config .`
  - 実行場所は `app/iac/envs/dev`。モジュール単体も同様に validate 可能。
- **creds がある環境での追加検証(ユーザー実施、agent は実行しない)**:
  - S3 backend バケット作成後 `terraform init` → `terraform plan` で差分確認。NAT Gateway が計画に現れないこと、破壊的変更(replace)がないことを確認。
- review-spec が R1〜R8 の充足を上記トレーサビリティ表で突き合わせ、テストで機械的に検証できない要件(README の記載内容など)を補完する。

## リスク / 未確定事項

- **S3 backend バケットが未作成**: backend 設定はコードに含める(iac.md 要件)が、state 用 S3 バケット / ロック(DynamoDB or `use_lockfile`)は事前に手動作成が必要。バケット名・key・region は tfvars では設定できない(backend はプレースホルダ直書き)。**チェック時は `terraform init -backend=false` を用いる**旨を envs/dev README に明記する。バケット名の具体値・命名規約はユーザー判断が必要。
- **AWS アカウント ID / リージョン**: プレースホルダで実装する。実 plan 時にユーザーが tfvars / backend を埋める前提。秘密情報・アカウント ID をコードに直書きしない(project.md)。
- **CloudFront → ALB 間が HTTP**: Spec スコープ(ACM/カスタムドメインなし)による意図的なトレードオフ。CloudFront → ALB 区間は AWS ネットワーク内だが暗号化されない。cdn/app の README に明記。カスタムヘッダ検証 + プレフィックスリスト SG で ALB 直叩きは防ぐが、ヘッダ値は HTTP で流れる点は許容範囲としてレビューで確認。
- **WAF の us-east-1 provider alias**: CLOUDFRONT scope は us-east-1 必須。alias provider をルートで定義し cdn に渡す。alias の配線ミスは validate では検出しづらいため review で重点確認。
- **tflint / trivy が未インストールの可能性**: checker 実行時に未導入なら、checker がその旨を報告する(インストールは環境側の課題)。fmt/validate は Terraform 本体のみで実行可能。
- **app/api がヘルスチェック専用エンドポイントを持たない / RDS に接続しない**: ヘルスチェックは `GET /tasks`(200)で代替。app/api は in-memory リポジトリのため RDS を実際には使用しないが、Spec 要件どおり RDS は構築し接続情報をタスクに配線する。`/healthz` 追加と RDS 接続実装は **app/api 側の別 Issue** とし、本計画では app/api を変更しない(issue-creator への起票を推奨)。
- **コンテナイメージ未提供**: ECR リポジトリは作るがイメージは Spec スコープ外。タスク定義 image はプレースホルダ変数。plan は通るが、実 apply 後にサービスが安定するにはイメージ push が別途必要(README に明記)。
- **Fargate ARM64 とイメージアーキテクチャの整合**: `runtime_platform = ARM64` のため、将来 push するイメージも ARM64(linux/arm64)ビルドが必要。README に注記する。
- **概算コスト $50/月 の検証**: 実測はできない(plan は課金額を出さない)。review-performance が構成要素ベースで妥当性を定性評価する。
