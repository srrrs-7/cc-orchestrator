---
paths:
  - "app/iac/**"
---

# app/iac — Terraform 規約

## コマンド

実行は対象環境ディレクトリ(`app/iac/envs/<env>`)で行う。checker はこれを実行する。

| 目的 | コマンド |
|---|---|
| format(チェック) | `terraform fmt -check -recursive` |
| format(自動修正) | `terraform fmt -recursive` |
| validate | `terraform validate` |
| lint | `tflint --recursive` |
| security scan | `trivy config .` |
| plan(差分確認) | `terraform plan` |

**`terraform apply` は agent からは実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

## レイアウト

- `modules/<module>/` — 再利用可能なモジュール(main.tf / variables.tf / outputs.tf / README.md)
- `envs/<env>/` — 環境ごとのルートモジュール(dev / prod)。環境差分は tfvars と変数で表現し、環境ごとにリソース定義をコピーしない

## コーディング

- リソース名・変数名は snake_case。リソースのタグ/ラベルには環境名と管理主体(`ManagedBy = "terraform"`)を付与する
- state は remote backend(S3 等)+ lock を前提とし、backend 設定をコードに含める
- `variable` には必ず `type` と `description` を書く。secrets は変数経由でも tfvars に平文で書かず、Secrets Manager / SSM 参照にする
- provider・module のバージョンは固定する(`required_version`, `required_providers`, module の `version`)
- `count` より `for_each` を優先する(リソースの増減で意図しない再作成が起きにくい)
