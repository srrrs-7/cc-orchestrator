# envs/dev

`app/iac` の dev 環境ルートモジュール。`modules/network` / `modules/db` / `modules/platform` /
`modules/cdn` / `modules/service`(api・auth の 2 回呼び出し)を呼び出し、次の 3 アプリの配信経路
一式を構成する(SPEC-001 の api 単体世代を SPEC-004 で auth/web に拡張):

```
Internet → CloudFront(単一ディストリビューション, + WAFv2)
   ├─ default (/*)  → S3(web SPA, OAC・非公開)
   ├─ /api/*        → ALB(共用)→ api  ECS Fargate(ARM64) → RDS PostgreSQL
   └─ /auth/*       → ALB(共用)→ auth ECS Fargate(ARM64)
```

呼び出しの依存順序(循環回避の DAG。詳細は `modules/platform/README.md`):

```
network → platform(ALB+listener+cluster) → cdn(S3+CloudFront, 依存は ALB DNS のみ)
                    │                          │
                    └──────────────┬───────────┘
                                   ▼
                        service_api / service_auth
```

## モジュール構成の変遷(SPEC-001 → SPEC-004)

旧 `modules/app`(ALB + ECS 1 サービスを一体化)は、api/auth の 2 サービス化にあたり
`modules/platform`(共有 ALB/リスナー/ECS クラスタ)+ `modules/service`(汎用サービス、
api/auth で 2 回呼ぶ)に分解した。api 関連リソースのアドレス変更は `moved.tf` の
cross-module `moved` ブロックで吸収しており、`terraform plan` 上は `move`(create/destroy を
伴わない)として現れる想定(非退行。詳細は `moved.tf` のコメントと
`docs/plans/SPEC-004-plan.md`)。

`modules/service` は既定でリソース名に `service_name`(`api` / `auth`)を含めるため
(`"<name_prefix>-<service_name>-*"`)、そのままでは api の一部 ForceNew リソース名
(ターゲットグループ / タスク実行ロール / タスクロール / シークレット読み取りインラインポリシー)
が旧 `modules/app` 時代の名前(サービス修飾なし)からズレ、`moved` があっても実質 replace に
なってしまう。これを避けるため、`module.service_api` 呼び出しでは
`target_group_name` / `task_execution_role_name` / `task_role_name` / `secrets_policy_name`
の 4 変数に旧名を明示的に渡し、api の全リソース名が SPEC-001 時点と文字列一致するようにしている
(詳細は `modules/service/README.md` の「ForceNew なリソース名を呼び出し側から上書きできる理由」)。
auth はこれらの変数を渡さず既定(`<name_prefix>-auth-*`)のままでよい(新規リソースのため
replace の概念が無い)。

## 事前準備: S3 backend バケットの作成(bootstrap)

`versions.tf` の `backend "s3"` はプレースホルダ値(`bucket = "REPLACE_WITH_TERRAFORM_STATE_BUCKET_NAME"`)
のままではそのまま `terraform init` に使えない。実際に運用する場合は以下を手動で行うこと:

1. state 保存用の S3 バケットを作成する(バージョニング有効・デフォルト暗号化推奨)
2. `versions.tf` の `bucket` / `key` / `region` を実際の値に書き換える
   (アカウント固有値のためコードに直書きせず、書き換えは各自の環境で行う)
3. ロックには S3 のネイティブロック機能(`use_lockfile = true`, Terraform 1.10+)を使用しており、
   DynamoDB テーブルは不要。より古い Terraform を使う場合は `use_lockfile` の代わりに
   `dynamodb_table` を使う構成に読み替えること
4. `terraform init` を実行する

## 認証情報なしでの検証手順(checker が実行するコマンド)

AWS 認証情報が無い環境でも、以下は実行できる(`app/iac/envs/dev` で実行):

```sh
terraform init -backend=false   # backend を無効化し provider のみ取得(認証情報不要)
terraform fmt -check -recursive
terraform validate
tflint --recursive               # 未導入の場合はその旨を報告する
trivy config .                    # 未導入の場合はその旨を報告する
```

## 認証情報がある環境での追加検証・適用手順(人間が実施)

```sh
terraform init
terraform plan
```

- `terraform plan` で、NAT Gateway に該当するリソースが計画に現れないこと(R6)、
  auth の ECS サービス・auth 用ターゲットグループ・auth 用 ECR・S3・OAC・3 behavior が
  新規に現れること、api 関連リソースが `moved`(move)として扱われ意図しない `replace` /
  `destroy` が無いことを確認する
- **`terraform apply` は agent からは実行しない。** plan の内容を確認したうえで、
  人間が判断して apply を実行すること

## コンテナイメージについて

`var.container_image`(api)/ `var.auth_container_image`(auth)はいずれも既定で空文字列
(`""`)であり、その場合対応する `modules/service` インスタンスはこの環境自身の ECR
リポジトリ(`module.service_api.ecr_repository_url` / `module.service_auth.ecr_repository_url`、
`envs/dev` の output では `api_ecr_repository_url` / `auth_ecr_repository_url`)の `:latest`
タグを参照する。ただし **Terraform はイメージのビルド・push を行わない**(Spec スコープ外)
ため、実際にイメージを `docker push` するまで該当 ECS サービスは健全な状態にならない。
push するイメージは `runtime_platform = ARM64` に合わせて **必ず linux/arm64 でビルド**する
こと(R4。build-push 手順は root `Makefile` を参照)。

## Postgres 永続化・マイグレーション init コンテナについて(SPEC-005、RF リファクタリング後)

`module.service_api` / `module.service_auth` はいずれも `module.db`(RDS PostgreSQL、同一
インスタンス上に api/auth それぞれ専用のデータベースを持つ。SPEC-005 plan RF.1.1)への接続情報を
`DB_HOST` / `DB_PORT` / `DB_NAME`(api=`"api"` / auth=`"auth"`、per-service)/ `DB_SSLMODE`
(`var.db_sslmode`、既定 `"require"`)の plain env と、`DB_USER` / `DB_PASSWORD`
(`module.db.master_user_secret_arn` の JSON key を ECS `secrets` の `:username::` /
`:password::` で個別参照)として受け取る(R6。`DB_SCHEMA`/`search_path` は初回実装のみで、この
リファクタリング後は使わない。詳細な設計判断は `modules/db/README.md` を参照)。

さらに両サービスとも、アプリ本体の起動前に対象データベースを作成(`CREATE DATABASE`、未存在時
のみ)し `goose up` を実行する **migrate init コンテナ**(`var.migration_environment` /
`migration_secrets` / `migration_image` / `migration_command`)を配線している(R5。api/auth は
**同一の共有 `app/migrator` イメージ**(`aws_ecr_repository.migrator`、`migrator.tf`)を
`migration_command = ["-target", "api"|"auth"]` の違いだけで実行する。`migration_environment`
には `DB_MAINTENANCE_NAME`(既定 `"postgres"`。`CREATE DATABASE` 用の接続先で、RDS で
`"postgres"` が使えない場合は `module.db.db_name` へ差し替え可能)も含む。方式・並行実行の注意・
代替案は `modules/service/README.md` の「マイグレーション init コンテナ」を参照)。

**apply 前の前提条件**: `var.migration_image`(両サービスとも `aws_ecr_repository.migrator` の
`:latest` タグ)は Terraform が作るのはリポジトリのみで、イメージ自体はまだ push されていない
(push 経路は本 Spec の範囲外として後続に委ねられている、SPEC-005 plan RF.6.1 RF-b)。**共有
migrator イメージを push してから `apply` すること**(api/auth それぞれに別イメージを push する
必要はない。1 つの push で両サービスの migrate コンテナに反映される)。push せずに `apply` すると、
新しいデプロイのタスクが migrate コンテナのイメージ pull に失敗して起動できず、ロールアウトが
詰まる(既存の running タスクはそのまま残るため、既存の可用性への直接影響は無い)。

## web(SPA)のデプロイについて

Terraform は web 用 S3 バケット(`web_bucket_name` output)と CloudFront の「箱」までしか
用意しない。`apply` 後、別途 `app/web` を `bun run build` して `dist/` を
`aws s3 sync dist s3://$(terraform output -raw web_bucket_name) --delete` で同期し、
`aws cloudfront create-invalidation --distribution-id $(terraform output -raw cloudfront_distribution_id) --paths '/*'`
でキャッシュを無効化する必要がある(root `Makefile` の `deploy-web` ターゲット参照)。

## auth の issuer が単一 `apply` で解決する理由

`ISSUER` 環境変数(`http://<cloudfront-domain>/auth`)は `module.cdn.cloudfront_domain_name`
に依存し、`module.cdn` は `module.platform.alb_dns_name` にのみ依存する(ECS サービスの
登録状態には依存しない)。そのため `platform → cdn → service_auth` の一方向 DAG が成立し、
初回 `apply` 一発で auth タスクに実際の CloudFront ドメインを注入できる(`module.app` を
経由した循環にならない設計。詳細は `modules/platform/README.md`)。auth は
プロセス起動ごとに RSA 署名鍵を生成するため(`app/auth` の既知の性質)、`desired_count = 1`
を既定にしている(`modules/service/README.md` 参照)。

## 概算コスト目安

dev 環境で概ね $55〜60/月 前後を想定(非機能要件。SPEC-001 の api 単体世代($50/月 前後)に
auth Fargate タスク 1 本(数$/月)+ S3/CloudFront 追加分(数$/月未満)が上乗せされる)。主な
内訳(東京リージョン目安、詳細な選定理由は各モジュールの README を参照):

- RDS `db.t4g.micro`(single-AZ): 約$12/月
- Fargate(ARM64, api + auth 2 サービス、256/512、Spot 中心): 数$/月〜(稼働時間・Spot 単価による)
- Public IPv4(ECS タスクに付与。タスク数に比例、1タスクあたり約$3.65/月): api・auth 各
  `desired_count=1` では合計 約$7.30/月。NAT Gateway の固定費と異なりタスク数に応じて
  線形に増える(詳細は `modules/network/README.md`)
- ALB: 約$16〜20/月(固定 + LCU)。api/auth で共用のため 1 本ぶんのみ
- S3(web): 数十セント/月(サンプル規模の静的アセット容量・リクエスト数では小さい)
- CloudFront + WAF: 数$/月(リクエスト量・マネージドルール数による)。CloudFront Function の
  実行コストは非常に小さい(Lambda@Edge より安価)
- NAT Gateway は使用しないため 0 円(R6)

実測値は `terraform plan` では出力されないため、AWS Pricing Calculator 等での別途見積りを
推奨する。
