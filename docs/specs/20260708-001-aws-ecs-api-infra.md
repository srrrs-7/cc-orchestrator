---
id: SPEC-001
title: AWS ECS API インフラ(CloudFront → WAF → ECS → PostgreSQL)のサンプル実装
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-08
updated: 2026-07-08
issues: [ISSUE-001, ISSUE-002]
supersedes: null
---

# SPEC-001: AWS ECS API インフラ(CloudFront → WAF → ECS → PostgreSQL)のサンプル実装

## 1. ユーザー価値(なぜ作るか)

> **このリポジトリの開発者** が **コスト効率を重視した AWS の標準的な API 基盤(CloudFront → WAF → ECS → PostgreSQL)を Terraform コードとして参照・再利用できるようになり**、**インフラ構築の設計判断と実装の手間を大幅に削減できる** 価値を得る。

- **対象ユーザー**: このリポジトリで API(`app/api`)を AWS 上に展開したい開発者
- **解決する課題**: 現状 `app/iac` は空であり、API をデプロイする基盤が存在しない。また、CloudFront + WAF + ECS + RDS の構成は選択肢(NAT Gateway の要否、Fargate か EC2 か、RDS か Aurora か等)が多く、コストと安全性のバランスを取った構成を毎回ゼロから設計するのは負担が大きい
- **得られる価値**: コスト最適化の判断(理由付き)が織り込まれた、そのまま `terraform plan` できるリファレンス実装
- **価値の検証方法**: `envs/dev` で `terraform fmt -check` / `terraform validate` / `tflint` / `trivy config` が全て通り、(AWS 認証情報がある環境で)`terraform plan` がエラーなく差分を出力できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 開発者として、`app/iac/envs/dev` で `terraform plan` を実行して構成全体の差分を確認したい。なぜなら apply 前にコストと構成を把握して判断したいから。
- 開発者として、環境差分(dev / prod)を tfvars の変数だけで表現したい。なぜならリソース定義のコピーはメンテナンスコストと乖離を生むから。
- 開発者として、各モジュールの README でコスト上の選択理由を読みたい。なぜなら構成を変更する際にトレードオフを再検討できるようにしたいから。

### 利用フロー

1. 開発者が `app/iac/envs/dev` で `terraform init` を実行する
2. 開発者が `terraform plan` を実行し、作成されるリソースと概算コストを確認する
3. 開発者(人間)が判断して `terraform apply` を実行する(agent は実行しない)
4. CloudFront のドメイン経由で API(ECS 上の `app/api`)にアクセスできる

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: リクエスト経路が CloudFront → WAF → ALB → ECS(Fargate 上の API)→ RDS PostgreSQL となる構成を Terraform で定義する
- [ ] R2: WAF(WAFv2, CLOUDFRONT スコープ)に AWS マネージドルール(Common / IP Reputation)とレート制限ルールを適用する
- [ ] R3: ALB へのアクセスを CloudFront 経由のみに制限する(CloudFront マネージドプレフィックスリストによる SG 制限 + カスタムヘッダ検証)
- [ ] R4: DB は RDS PostgreSQL とし、認証情報は Secrets Manager 管理(`manage_master_user_password`)でコード・tfvars に平文を書かない
- [ ] R5: ECS は Fargate(ARM64 / Graviton)とし、dev では Fargate Spot を併用できる
- [ ] R6: NAT Gateway を使わない構成とする(ECS タスクはパブリックサブネット + SG 制限、RDS はプライベートサブネット)
- [ ] R7: `modules/` + `envs/dev` のレイアウトとし、環境差分は変数・tfvars のみで表現する
- [ ] R8: 各モジュールに README を置き、コスト上の選択理由(採用・不採用)を記録する

### 非機能要件

- 月額コスト目安: dev 環境で概ね $50/月 前後(NAT Gateway 不使用・単一 AZ DB・最小タスクサイズによる)
- `.claude/rules/iac.md` の規約に準拠する(バージョン固定、`for_each` 優先、type/description 必須、タグ付与、remote backend 前提)
- セキュリティ: DB は非公開、ALB は CloudFront 経由のみ、秘密情報の平文記載なし

### スコープ外(やらないこと)

- `terraform apply` の実行(plan まで。apply はユーザー判断)
- カスタムドメイン / Route53 / ACM 証明書(サンプルは CloudFront デフォルトドメインを使用。CloudFront → ALB 間は HTTP とし、トレードオフを README に明記)
- CI/CD パイプライン、監視・アラート、prod 環境の実体化(`envs/dev` のみ作成)
- アプリケーションイメージのビルド・プッシュ(ECR リポジトリの用意まで)

## 4. 設計(どう実現するか)

### 方針

「サンプルとして安全側の設計は保ちつつ、固定費の大きい要素(NAT Gateway・Multi-AZ・大きいインスタンス)を削る」をコスト最適化の軸とする。

### アーキテクチャ / データ / インターフェース

```
Internet → CloudFront(+ WAFv2 CLOUDFRONT scope, us-east-1)
        → ALB(public subnet, SG: CloudFront prefix list のみ許可 + カスタムヘッダ検証)
        → ECS Fargate Service(ARM64, public subnet + public IP ※NAT 回避, SG: ALB のみ許可)
        → RDS PostgreSQL(private subnet, single-AZ, db.t4g.micro, SG: ECS のみ許可)
```

- モジュール分割(想定): `network`(VPC / subnets / SG)、`cdn`(CloudFront + WAF)、`app`(ALB / ECS / ECR)、`db`(RDS)。詳細は実装計画(planner)に委ねる
- WAF は CLOUDFRONT スコープのため us-east-1 の provider alias が必要
- state backend は S3(+ lock)をコードに含める(バケット名は変数/プレースホルダで表現)

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| NAT Gateway + プライベートサブネットでタスク実行 | 固定費 約$32/月 + データ転送費。サンプルの規模では SG で十分に絞れるため、パブリックサブネット + public IP を採用 |
| VPC Interface Endpoint(ECR/Logs 等)で NAT 回避 | エンドポイント 3〜4 個で約$22〜29/月 かかり NAT 回避のコストメリットが薄い |
| Aurora Serverless v2 | 最小 0.5 ACU でも約$43/月 と db.t4g.micro(約$12/月)より高い。サンプル用途では過剰 |
| API Gateway + Lambda | 要件が「ECS を使用した構成」のため対象外 |
| EC2 起動タイプの ECS | インスタンス管理の運用負担。小規模では Fargate(ARM64 + Spot)で十分安い |

## 5. 実装計画

planner が `docs/plans/SPEC-001-plan.md` を作成する。タスク概要:

- [ ] T1: planner が実装計画を作成(モジュール分割・変数設計・作業手順・検証戦略)
- [ ] T2: impl-iac が `app/iac` に modules / envs/dev を実装
- [ ] T3: checker が fmt / validate / tflint / trivy を実行
- [ ] T4: review-security / review-performance / review-spec がレビュー
- [ ] T5: 指摘対応と Spec ステータス更新

## 6. 経緯(時系列・追記のみ)

### 2026-07-08

- 初版作成。ユーザーから「CloudFront → WAF → API(ECS)→ Postgres のコストパフォーマンスが良い構成を Terraform で組む」という依頼を受けたもの。実装着手の指示を伴うため、作成時点で status: approved とした
- planner が実装計画 `docs/plans/SPEC-001-plan.md` を作成。impl-iac による実装に着手したため status: in-progress に更新
- 計画策定中に判明した app/api の課題(ヘルスチェック専用エンドポイント不在・RDS 未接続)を ISSUE-001 として起票し、frontmatter に相互リンク。ALB ヘルスチェックは暫定で `GET /tasks` を使用し、`/healthz` 追加と PostgreSQL 接続は ISSUE-001 で別途対応する
- レビュー(review-security / review-performance)で挙がった、サンプルスコープでは意図的に見送ったセキュリティ・可用性強化 7 項目(RDS TLS 強制・CMK 暗号化・CloudFront/WAF ログ・ECR IMMUTABLE・state バケット IaC 化・CloudFront 最低 TLS 明示・ECS 単一障害点解消)を、本番相当移行時のチェックリストとして ISSUE-002 に集約起票し、frontmatter に相互リンク。いずれも退行ではなくサンプルの設計方針どおりの省略で、本 Spec のスコープ(dev のサンプル)では対応しない
