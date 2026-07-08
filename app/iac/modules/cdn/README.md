# cdn module

WAFv2 Web ACL(CLOUDFRONT スコープ)と、ALB をオリジンとする CloudFront ディストリビューション
を作成する。

## provider alias(us-east-1)が必要な理由

WAFv2 で `scope = "CLOUDFRONT"` の Web ACL を作るには、**必ず us-east-1 リージョンの
API エンドポイント**で作成する必要がある(CloudFront 自体がグローバルサービスのため)。
本モジュールは `required_providers.aws.configuration_aliases = [aws.us_east_1]` を宣言し、
呼び出し側(`envs/dev`)から `providers = { aws = aws, aws.us_east_1 = aws.us_east_1 }` として
default provider(リージョンは `var.region`、例: ap-northeast-1)と us-east-1 alias provider の
両方を受け取る。`aws_wafv2_web_acl` は `provider = aws.us_east_1` を明示し、
`aws_cloudfront_distribution` はグローバルサービスのため default provider のまま作成する。

alias の配線ミス(呼び出し元で `providers` ブロックを渡し忘れる等)は `terraform validate` では
検出しづらいため、レビュー時に重点的に確認すること。

## コスト上の選択理由

### WAF マネージドルール + レート制限(R2)

- `AWSManagedRulesCommonRuleSet`(OWASP Top 10 相当の一般的な攻撃パターン)と
  `AWSManagedRulesAmazonIpReputationList`(既知の悪性 IP)の 2 つの AWS マネージドルールを
  適用する。自前でルールを書くよりも運用コストが低く、AWS 側で継続的に更新される
- `rate_based_statement`(既定 `waf_rate_limit = 2000` リクエスト/5分/IP)でレート制限を行い、
  単純な DoS 的アクセスを IP 単位でブロックする
- マネージドルールグループは 1 グループあたり課金があるため、必要最小限の 2 グループ +
  レート制限 1 ルールに絞っている。追加のマネージドルールグループ(SQLi 専用など)は
  要件に応じて追加を検討する

### CloudFront デフォルトドメインを使うトレードオフ

- カスタムドメイン / ACM 証明書は Spec スコープ外のため、`viewer_certificate` は
  `cloudfront_default_certificate = true` とし、`*.cloudfront.net` のデフォルトドメインを
  使用する。独自ドメインでの提供が必要な場合は Route53 + ACM(us-east-1)証明書を追加し
  `aliases` / `viewer_certificate.acm_certificate_arn` を設定すること
- `price_class` は既定 `PriceClass_100`(北米・欧州のエッジロケーションのみ)とし、
  全世界配信の `PriceClass_All` より配信コストを抑えている。アジアなど他リージョンからの
  レイテンシを優先する場合は `PriceClass_200` 以上に変更する
- API レスポンスはキャッシュしない(`Managed-CachingDisabled` ポリシー)。静的アセット等
  キャッシュ可能なコンテンツが増えた場合は、パスパターンごとに `ordered_cache_behavior` を
  追加しキャッシュ有効なポリシーを使うと配信コストとオリジン負荷をさらに下げられる

## 既知の制約

- CloudFront のアクセスログ(`logging_config`)は有効化していない(S3 ログバケットの
  追加コスト・運用を避けるため)。本番運用ではアクセスログや WAF ログ(Kinesis Firehose 経由)
  を有効化することを推奨する
- CloudFront → ALB 間は HTTP(`origin_protocol_policy = "http-only"`)。詳細は `app` モジュールの
  README を参照
