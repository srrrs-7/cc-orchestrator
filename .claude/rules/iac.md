---
paths:
  - "app/iac/**"
---

# app/iac — Terraform 規約

## コマンド

`app/iac/Makefile` の make ターゲット経由で実行することを推奨する(環境は `ENV=<env>` で指定、既定は `dev`)。直接 terraform を叩く場合の同等コマンドと実行場所も併記する。checker はこれらを実行する。

実行場所には 2 種類ある:
- **format(fmt / fmt-check)は `app/iac` ルートで全体(modules + envs)を対象に実行する。** `terraform fmt` はディレクトリ構造・モジュール解決に依存せず全 `.tf` を整形するため、env ディレクトリ基点で実行すると `modules/` を再帰対象から外し、整形崩れを見逃す。
- **validate / plan / init / lint / security scan は対象環境ディレクトリ(`app/iac/envs/<env>`)で実行する**(モジュール解決や env 単位の検証のため)。

| 目的 | make ターゲット | 直接コマンド | 実行場所 |
|---|---|---|---|
| format(チェック) | `make fmt-check` | `terraform fmt -check -recursive` | `app/iac` ルート |
| format(自動修正) | `make fmt` | `terraform fmt -recursive` | `app/iac` ルート |
| init(backend なし) | `make init-local` | `terraform init -backend=false` | `envs/<env>` |
| validate | `make validate` | `terraform validate` | `envs/<env>` |
| lint | `make lint` | `tflint --recursive` | `envs/<env>` |
| security scan | `make security` | `trivy config .` | `envs/<env>` |
| plan(差分確認) | `make plan` | `terraform plan` | `envs/<env>` |
| 一括チェック | `make check` | fmt-check → validate → lint → security | — |

- 認証情報や実 backend が無い環境で検証するときは `make init-local`(`-backend=false`)を使う。`envs/dev` の backend は S3 プレースホルダのため、素の `terraform init` は失敗する。
- `make lint` / `make security` はツール(tflint / trivy)未導入だと失敗する。導入は環境側の前提。

**`terraform apply`(`make apply`)は agent からは実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

## レイアウト

- `modules/<module>/` — 再利用可能なモジュール(main.tf / variables.tf / outputs.tf / README.md)
- `envs/<env>/` — 環境ごとのルートモジュール(dev / prod)。環境差分は tfvars と変数で表現し、環境ごとにリソース定義をコピーしない

## コーディング

- リソース名・変数名は snake_case。リソースのタグ/ラベルには環境名と管理主体(`ManagedBy = "terraform"`)を付与する
- state は remote backend(S3 等)+ lock を前提とし、backend 設定をコードに含める
- `variable` には必ず `type` と `description` を書く。secrets は変数経由でも tfvars に平文で書かず、Secrets Manager / SSM 参照にする
- provider・module のバージョンは固定する(`required_version`, `required_providers`, module の `version`)
- `count` より `for_each` を優先する(リソースの増減で意図しない再作成が起きにくい)
