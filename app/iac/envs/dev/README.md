# envs/dev

`app/iac` の dev 環境ルートモジュール。`modules/network` / `modules/db` / `modules/app` /
`modules/cdn` を呼び出し、CloudFront → WAF → ALB → ECS(Fargate)→ RDS PostgreSQL の
一式を構成する(SPEC-001)。

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
  および意図しない `replace` / `destroy` が無いことを確認する
- **`terraform apply` は agent からは実行しない。** plan の内容を確認したうえで、
  人間が判断して apply を実行すること

## コンテナイメージについて

`var.container_image` は既定で空文字列(`""`)であり、その場合 `modules/app` はこの環境自身の
ECR リポジトリ(`module.app.ecr_repository_url`)の `:latest` タグを参照する。ただし
**Terraform はイメージのビルド・push を行わない**(Spec スコープ外)ため、実際にイメージを
`docker push` するまで ECS サービスは健全な状態にならない。push するイメージは
`runtime_platform = ARM64` に合わせて **linux/arm64 でビルド**すること。

## 概算コスト目安

dev 環境で概ね $50/月 前後を想定(非機能要件)。主な内訳(東京リージョン目安、詳細な
選定理由は各モジュールの README を参照):

- RDS `db.t4g.micro`(single-AZ): 約$12/月
- Fargate(ARM64, 256/512, Spot 中心): 数$/月〜(稼働時間・Spot 単価による)
- Public IPv4(ECS タスクに付与。タスク数に比例、1タスクあたり約$3.65/月): 既定の
  `desired_count=1` では約$3.65/月。NAT Gateway の固定費と異なりタスク数に応じて線形に増える
  (詳細は `modules/network/README.md`)
- ALB: 約$16〜20/月(固定 + LCU)
- CloudFront + WAF: 数$/月(リクエスト量・マネージドルール数による)
- NAT Gateway は使用しないため 0 円(R6)

実測値は `terraform plan` では出力されないため、AWS Pricing Calculator 等での別途見積りを
推奨する。
