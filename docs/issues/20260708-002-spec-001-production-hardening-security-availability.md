---
id: ISSUE-002
title: SPEC-001 サンプル基盤を本番相当へ移行する際のセキュリティ・可用性強化項目
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-08
specs: [SPEC-001]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-002: SPEC-001 サンプル基盤を本番相当へ移行する際のセキュリティ・可用性強化項目

## 1. ユーザー価値への影響(なぜ対応するか)

> **SPEC-001 の Terraform 基盤(CloudFront → WAF → ALB → ECS → RDS)を本番相当の運用に転用しようとする開発者** の **セキュリティ(通信・保存時暗号化・監査ログ・サプライチェーン)と可用性** が **サンプル用途で意図的に省略された強化がそのまま残ると損なわれる**。

- **影響を受けるユーザー**: SPEC-001 をリファレンスとして本番相当(実データ・実トラフィックを扱う環境)へ横展開する、このリポジトリの開発者
- **損なわれる価値**:
  - セキュリティ: RDS 接続が平文になりうる、保存データ・イメージが AWS 管理鍵のみで CMK による鍵管理・失効ができない、侵害調査に使うアクセス/WAF ログが残らない、`:latest` 上書きによるイメージ改ざんを防げない
  - 可用性: ECS が実質単一タスク(既定 100% Spot)で、Spot 中断や AZ 障害でサービスが停止しうる
- **影響範囲・頻度**: **現時点(SPEC-001 は dev のみのサンプル)では顕在化しない**。SPEC-001 のスコープ(コスト効率の良いリファレンス実装・`terraform plan` が通ること)は本 Issue の項目が未対応でも達成される。本番相当へ移行したときにのみ実害となる(移行しなければ影響なし)。
- **回避策**: あり(本番移行時に本 Issue のチェックリストに沿って各項目を有効化する)。dev サンプルのまま使う限りは対応不要。

## 2. 現象(何が起きているか)

> 個別の不具合(退行)ではなく、本番相当移行時のチェックリストを 1 件に集約したもの。以下は「本番相当で期待される状態」と「サンプルの現状」の差分。

### 期待する動作(本番相当で満たしたい状態)

1. RDS への接続が TLS で強制される
2. 保存時暗号化がカスタマー管理 KMS キー(CMK)で行われ、鍵のローテーション・失効・アクセス制御ができる
3. CloudFront アクセスログと WAF ログが有効で、侵害調査・不正リクエスト分析ができる
4. ECR のタグが IMMUTABLE で、`:latest` 等の上書きによるイメージすり替えが起きない
5. state 用 S3 バケットが IaC 管理で、Public Access Block・バケットポリシー・バージョニングがコードで強制される
6. カスタムドメイン + ACM 利用時に CloudFront の最低 TLS バージョンが明示される(例: `TLSv1.2_2021`)
7. ECS が単一障害点にならない(on-demand ベースライン or 複数タスクで冗長化)

### 実際の動作(SPEC-001 のサンプル現状)

1. `aws_db_parameter_group.this` にパラメータが 1 つも設定されておらず、`rds.force_ssl` 未設定。TLS が強制されない(`app/iac/modules/db/main.tf:28-33`)
2. 保存時暗号化は AWS 管理鍵のみ。CloudWatch Logs は `kms_key_id` 未指定(`app/iac/modules/app/logs.tf:3-8`)、ECR は `encryption_type = "AES256"`(`app/iac/modules/app/ecr.tf:13-15`)。参考: RDS も `storage_encrypted = true` だが `kms_key_id` 未指定で AWS 管理鍵(`app/iac/modules/db/main.tf:44`)
3. CloudFront ディストリビューションに `logging_config` がなく、WAF に `aws_wafv2_web_acl_logging_configuration` もない(アクセスログ・WAF ログ無効。`app/iac/modules/cdn/main.tf`)
4. ECR が `image_tag_mutability = "MUTABLE"`(`app/iac/modules/app/ecr.tf:7`)
5. backend の S3 バケットはプレースホルダで、コメントに「`terraform init` の前に out-of-band で作成する」と明記(手動作成前提でコード管理外。`app/iac/envs/dev/versions.tf:15-28`)
6. `viewer_certificate` が `cloudfront_default_certificate = true` で、デフォルト証明書のため `minimum_protocol_version` を指定できない(`app/iac/modules/cdn/main.tf:167-171`)
7. `desired_count = 1` かつ `use_fargate_spot = true` / `fargate_base = 0` / `fargate_weight = 0` / `fargate_spot_weight = 1` で、既定では 1 タスクがすべて Spot 上で動く(`app/iac/envs/dev/terraform.tfvars` の該当値、`app/iac/modules/app/ecs.tf:89-118`、`app/iac/modules/app/variables.tf:50-77`)

### 再現手順(第三者が確認できる形)

1. `app/iac/modules/db/main.tf` の `aws_db_parameter_group "this"`(28-33 行)を開き、`parameter` ブロックが無い(= `rds.force_ssl` 未設定)ことを確認する
2. `app/iac/modules/app/logs.tf` に `kms_key_id` が無いこと、`app/iac/modules/app/ecr.tf` の `encryption_configuration` が `AES256`(13-15 行)であることを確認する
3. `app/iac/modules/cdn/main.tf` の `aws_cloudfront_distribution "this"` に `logging_config` が無いこと、同ファイルに WAF の logging configuration リソースが無いことを確認する
4. `app/iac/modules/app/ecr.tf:7` が `image_tag_mutability = "MUTABLE"` であることを確認する
5. `app/iac/envs/dev/versions.tf:15-28` の `backend "s3"` がプレースホルダで、コメントに out-of-band 作成前提と書かれていることを確認する
6. `app/iac/modules/cdn/main.tf:167-171` の `viewer_certificate` が `cloudfront_default_certificate = true` であることを確認する
7. `app/iac/envs/dev/terraform.tfvars` の `desired_count = 1` / `use_fargate_spot = true` / `fargate_base = 0` と、`app/iac/modules/app/ecs.tf:99-112` の capacity provider strategy を確認する

### 環境・条件

- 対象: `app/iac`(Terraform、SPEC-001 のサンプル基盤)。現状 `envs/dev` のみ。
- 発見文脈: SPEC-001 のレビュー(review-security / review-performance)で Minor として挙がった「サンプルスコープでは対応せず、本番相当移行時に検討すべき強化項目」を 1 件に集約したもの。

## 3. 原因(なぜ起きているか)

### 調査ログ

各項目は実装ファイルで事実確認済み(根拠は「2. 現象 > 実際の動作」に file:line で記載)。要約:

- 事実: RDS の parameter group が空で `rds.force_ssl` 未設定のため、パラメータとしては no-op(`db/main.tf:28-33`)
- 事実: CloudWatch Logs / ECR / RDS はいずれも `kms_key_id` を指定しておらず AWS 管理鍵で暗号化(`logs.tf`、`ecr.tf:13-15`、`db/main.tf:44`)
- 事実: CloudFront / WAF ともにログ設定リソースが存在しない(`cdn/main.tf`)
- 事実: ECR タグは MUTABLE(`ecr.tf:7`)
- 事実: state バケットは手動 bootstrap 前提でコード管理外(`versions.tf:15-28`)
- 事実: デフォルト証明書利用のため CloudFront の最低 TLS バージョンは指定不能(`cdn/main.tf:167-171`)
- 事実: 既定構成では ECS が実質 1 タスク・全 Spot で単一障害点(`terraform.tfvars`、`ecs.tf:89-118`)
- 事実: これらは SPEC-001 の設計方針「サンプルとして安全側は保ちつつ固定費の大きい要素を削る」およびスコープ外(カスタムドメイン / ACM、監視・アラート、prod 実体化)に沿った意図的な省略で、モジュール README にもコスト理由が記録されている(`docs/specs/20260708-001-aws-ecs-api-infra.md` の 4. 設計 / スコープ外)

### 根本原因

**退行バグではない。** SPEC-001 が「コスト効率の良い dev 専用リファレンス実装」であることに合わせ、固定費・運用コストの大きい強化(CMK 管理、ログ保管、冗長化など)や、スコープ外要素(カスタムドメイン / ACM)に依存する設定を、Terraform 実装が **意図的に見送っている** ことによる。サンプルとしては要件どおりで、本番相当に転用する場合にのみ追加対応が必要になる。

## 4. 対応(どう解決するか)

### 対応方針

- **今回のサンプルスコープ(SPEC-001 / dev)では対応しない。** 本 Issue は本番相当移行時のチェックリストとして記録・追跡する。
- 本番移行を決めた時点で planner に計画化を依頼し、impl-iac が各項目を実装、tester(`terraform validate` / `plan` の差分確認)・checker(fmt / validate / tflint / trivy)・review-* を通す。**`terraform apply` は実行せず plan 結果をユーザーに委ねる**(`.claude/rules/iac.md`)。
- 一部の項目はコスト・運用トレードオフを伴うため、実施可否は本番の要件に応じて個別判断する(特に 2:CMK、3:ログ保管費、7:冗長化のコスト増)。

### 実施内容(本番相当移行時のチェックリスト)

- [ ] 1. **RDS 接続の TLS 強制** — `app/iac/modules/db/main.tf` の `aws_db_parameter_group` に `parameter { name = "rds.force_ssl", value = "1" }` を追加。理由: 現状パラメータ未設定で TLS が強制されず、平文接続を許容しうる
- [ ] 2. **保存時暗号化の CMK 切替** — `app/iac/modules/app/logs.tf`(CloudWatch Logs)・`app/iac/modules/app/ecr.tf`(ECR)に CMK(`kms_key_id` / `encryption_type = "KMS"` + `kms_key`)を指定(必要に応じて `app/iac/modules/db/main.tf` の RDS も同様に)。理由: 現状は AWS 管理鍵のみで、鍵のローテーション・失効・アクセス制御を自前で管理できない
- [ ] 3. **CloudFront アクセスログ・WAF ログの有効化** — `app/iac/modules/cdn/` にログ出力先(S3 等)と `logging_config` / `aws_wafv2_web_acl_logging_configuration` を追加。理由: 現状コスト理由で無効。侵害調査・不正リクエスト分析にログが必要
- [ ] 4. **ECR タグの IMMUTABLE 化** — `app/iac/modules/app/ecr.tf` の `image_tag_mutability` を `IMMUTABLE` に。理由: 現状 MUTABLE で `:latest` 等の上書きによるイメージすり替え(サプライチェーンリスク)を防げない
- [ ] 5. **state 用 S3 バケットの IaC 化** — `app/iac/envs/dev/versions.tf` の backend が前提とする state バケットを bootstrap 用の IaC(別 state / スクリプト)で作成し、Public Access Block・バケットポリシー・バージョニングをコードで強制。理由: 現状は手動作成前提でコード管理外のため、公開設定や誤削除の保護がコードで保証されない
- [ ] 6. **CloudFront 最低 TLS バージョンの明示** — カスタムドメイン + ACM 導入時に `app/iac/modules/cdn/main.tf` の `viewer_certificate` で `minimum_protocol_version = "TLSv1.2_2021"` 等を指定。理由: 現状はデフォルト証明書のため下限を指定できず、古い TLS を許容しうる(ACM 前提のため項目 5/6 は本番ドメイン整備とセット)
- [ ] 7. **ECS の単一障害点解消** — `app/iac/modules/app` / `app/iac/envs/dev/terraform.tfvars` で `fargate_base = 1`(on-demand ベースライン確保)または `desired_count >= 2` を設定。理由: 現状は既定 100% Spot × `desired_count = 1` で、Spot 中断や AZ 障害でサービスが停止しうる

### 再発防止

- 「サンプルとして意図的に見送った強化項目」は、その場で本 Issue のようなチェックリストとして残し、本番移行時に必ず参照する運用を継続する(モジュール README のコスト理由記録と併せてトレーサビリティを維持)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。SPEC-001(`docs/specs/20260708-001-aws-ecs-api-infra.md`)のレビュー(review-security / review-performance)で Minor として挙がった、本番相当移行時に検討すべきセキュリティ・可用性強化 7 項目を 1 件に集約して記録。
- 7 項目すべてを実装ファイルで事実確認し、file:line の根拠を本文に記載(TLS 強制 no-op:`db/main.tf:28-33`、AWS 管理鍵:`logs.tf`/`ecr.tf:13-15`/`db/main.tf:44`、ログ無効:`cdn/main.tf`、ECR MUTABLE:`ecr.tf:7`、state 手動 bootstrap:`versions.tf:15-28`、デフォルト証明書:`cdn/main.tf:167-171`、単一タスク全 Spot:`terraform.tfvars`/`ecs.tf:89-118`)。
- severity は **low** と判定。判定根拠: これらは SPEC-001 のスコープ(dev 専用・コスト効率重視のリファレンス実装)で **意図的に見送ったトレードオフであり退行バグではない**。サンプルの目標(plan が通る参照実装)は本 Issue の未対応でも達成され、現時点の実害はない(回避策=本番移行時に有効化、あり)。ただし本番相当へ転用する際に未対応だとセキュリティ・可用性の実害につながるため、記録・追跡が必要と判断し low とした(critical/high/medium ではないのは、現行スコープで機能・価値が損なわれていないため)。
- 次にやること: 本番相当移行を決めた時点で planner に計画化を依頼し、各項目を impl-iac が実装 → tester/checker/review-* を通す(`terraform apply` はユーザー判断)。SPEC-001 側 frontmatter `issues` への相互リンク追記済み。

### 2026-07-09(関連 Issue の相互参照)

- 関連課題として **ISSUE-010**(app/api の全 HTTP ハンドラでリクエストボディサイズ上限と `http.Server` の防御設定が無い、緩やかな DoS への堅牢化不足)を相互参照する。「サーバ堅牢化」という趣旨は本 Issue と近いが、**別課題**として扱う。理由: 本 Issue は `app/iac`(Terraform のインフラ層、修正担当 impl-iac)に閉じたチェックリスト、ISSUE-010 は `app/api`(Go アプリ層の HTTP 受け口、修正担当 impl-api)の堅牢化で、対象コード・修正ファイル・オーナーがいずれも重ならないため。本エントリは相互参照の記録のみで、本 Issue の内容・ステータス(open / low)に変更はない。
