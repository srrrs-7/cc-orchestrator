# cdn module

WAFv2 Web ACL(CLOUDFRONT スコープ)と、単一の CloudFront ディストリビューションを作成する。
このディストリビューションは 3 オリジン(web SPA 用 S3 / api 用 ALB / auth 用 ALB)・3 behavior
(`default` / `/api/*` / `/auth/*`)を持ち、web・api・auth の配信を 1 つのディストリビューションで
まとめる(SPEC-004・R3)。

## provider alias(us-east-1)が必要な理由

WAFv2 で `scope = "CLOUDFRONT"` の Web ACL を作るには、**必ず us-east-1 リージョンの
API エンドポイント**で作成する必要がある(CloudFront 自体がグローバルサービスのため)。
本モジュールは `required_providers.aws.configuration_aliases = [aws.us_east_1]` を宣言し、
呼び出し側(`envs/dev`)から `providers = { aws = aws, aws.us_east_1 = aws.us_east_1 }` として
default provider(リージョンは `var.region`、例: ap-northeast-1)と us-east-1 alias provider の
両方を受け取る。`aws_wafv2_web_acl` は `provider = aws.us_east_1` を明示し、CloudFront・S3
関連リソースはグローバル/リージョナルサービスのため default provider のまま作成する。

alias の配線ミス(呼び出し元で `providers` ブロックを渡し忘れる等)は `terraform validate` では
検出しづらいため、レビュー時に重点的に確認すること。

## web を `modules/web` 新設ではなく `modules/cdn` 拡張にした理由

S3 バケットポリシーは CloudFront ディストリビューションの ARN(`AWS:SourceArn` 条件)を要し、
ディストリビューションは S3 バケットの regional domain name と OAC を要する。この循環は
**同一モジュール内**であれば Terraform がリソース参照から依存順序を解決できるが、モジュールを
分けると出力の受け渡しが二重・循環になり複雑化する。S3(バケット/OAC/ポリシー)を
CloudFront と同じ `modules/cdn` に閉じることで、この循環を回避している(s3.tf)。

## S3(非公開)+ OAC を採用し、ECS(nginx コンテナ)配信を退けた理由

web(SPA)の配信先として ECS 上に 3 つ目の常時稼働コンテナ(nginx)を立てる案は、Fargate タスクが
1 本恒常的に増えるためコストが増加する。SPA の静的アセット配信は S3 + CloudFront が標準的かつ
安価であり、OAC(Origin Access Control)により **S3 バケットを一切公開せず** CloudFront 経由での
み読み取りを許可できる(バケットポリシーの `AWS:SourceArn` 条件でこのディストリビューション以外
からの OAC 署名リクエストも拒否する)。DOCKER-001 の nginx リバースプロキシ役は、AWS では
CloudFront の behavior(strip Function 込み)が代替する。**`app/web/nginx.conf` と web の
Dockerfile(nginx ランタイム)はローカル `docker compose` 専用**であり、AWS デプロイでは
一切使用しない。両者を混同しないこと(「AWS でも nginx コンテナが要る」という誤解を避ける)。

## Web SPA のセキュリティヘッダー(Response Headers Policy)

`aws_cloudfront_response_headers_policy.web_security`(`${var.name_prefix}-web-security`)を
`default_cache_behavior`(S3/web SPA)にのみ関連付け、CSP・`X-Content-Type-Options: nosniff`・
`X-Frame-Options: DENY`・`Strict-Transport-Security`・`Referrer-Policy` を全 SPA レスポンスに付与する。
`/api/*` / `/auth/*` の ordered behaviors には意図的に適用しない(それぞれのバックエンドが独自の
`Content-Type` や CORS ヘッダを返すため、上書きすると問題が生じる可能性があるため)。

## SPA フォールバックを CloudFront Function にし、`custom_error_response` を退けた理由

CloudFront ディストリビューション全体に効く `custom_error_response`(403/404 → `/index.html`,
200)は、`default`(S3)behavior だけでなく `/api/*` `/auth/*` behavior の応答にも及ぶ。api の
404(存在しないタスク)や auth の OIDC エラー(不正な `redirect_uri` 等での 400/403)まで
`/index.html` に化けてしまい、API/OIDC のセマンティクスを壊す。

そこで **`default` behavior にのみ関連付けた CloudFront Function**(`functions/spa_fallback.js`、
viewer-request)を採用した。拡張子を持たない最後のパスセグメント(=クライアントサイドルート)を
`/index.html` に書き換える、nginx の `try_files $uri /index.html` と等価な処理を行う。`/api/*` /
`/auth/*` は別 behavior でマッチするため一切影響を受けない。

制約: 「拡張子付きだが実在しないアセット」は書き換えられず 404 のまま返る(nginx の
`try_files` と厳密には挙動が異なる)。深いクライアントルート(拡張子なしパス)は想定どおり
`/index.html` に落ちるため通常利用では問題ないが、レビューで妥当性を確認する対象とした。

## strip Function と header ベースルーティングの意図(R5)

`app/auth`・`app/api` はいずれもルート直下にエンドポイントを実装しており(base-path 対応なし)、
コード変更なしに `/api/*` `/auth/*` プレフィックス配下で公開するため、CloudFront Function
(`functions/strip_prefix.js`、viewer-request)で **先頭パスセグメントを剥がしてから ALB
オリジンへ転送**する(`/api/tasks` → `/tasks`、`/auth/.well-known/openid-configuration` →
`/.well-known/openid-configuration`)。DOCKER-001 のローカル nginx が `location /api/ { proxy_pass
$api_upstream/; }` で行っている剥がしと同型の契約であり、「エッジ(reverse proxy/CDN)がプレフィッ
クスを剥がし、コンテナはルート実装のまま」という設計が AWS でも一貫する。

剥がした後は api・auth いずれのリクエストも ALB にはルート相対パスで届き、**パスでは区別でき
ない**。そのため `alb-api` / `alb-auth` の 2 オリジンにそれぞれ異なるカスタムヘッダを付与する:

- `alb-api`: `X-Origin-Verify: <secret>` のみ
- `alb-auth`: `X-Origin-Verify: <secret>` + `X-Target-Service: auth`

ALB リスナールール(`modules/service`)はこのヘッダの組み合わせで forward 先のターゲットグループ
を決定する。`X-Origin-Verify` は CloudFront 経由であることの検証を兼ねる(R3、network モジュール
のプレフィックスリスト SG と合わせた二層防御)。`X-Target-Service` はセキュリティ境界ではなく、
あくまでルーティング用の判別子である。

auth の `ISSUER` 環境変数(`http://<cloudfront-domain>/auth`)は discovery の絶対 URL 生成に使われ
るが(`issuer` 文字列連結)、`/auth/*` behavior を経由すれば剥がされて実際に到達可能なため、
issuer と実アクセス経路が一致する(R5 充足。詳細は `docs/plans/SPEC-004-plan.md` の
「R5 の確定結論」)。

## コスト上の選択理由

### WAF マネージドルール + レート制限

- `AWSManagedRulesCommonRuleSet`(OWASP Top 10 相当の一般的な攻撃パターン)と
  `AWSManagedRulesAmazonIpReputationList`(既知の悪性 IP)の 2 つの AWS マネージドルールを
  適用する。自前でルールを書くよりも運用コストが低く、AWS 側で継続的に更新される
- `rate_based_statement`(既定 `waf_rate_limit = 2000` リクエスト/5分/IP)でレート制限を行い、
  単純な DoS 的アクセスを IP 単位でブロックする
- マネージドルールグループは 1 グループあたり課金があるため、必要最小限の 2 グループ +
  レート制限 1 ルールに絞っている。追加のマネージドルールグループ(SQLi 専用など)は
  要件に応じて追加を検討する
- WAF は単一ディストリビューション全体(web/api/auth すべて)に関連付けられ続ける(R6 の非退行)

### CloudFront デフォルトドメインを使うトレードオフ

- カスタムドメイン / ACM 証明書は Spec スコープ外のため、`viewer_certificate` は
  `cloudfront_default_certificate = true` とし、`*.cloudfront.net` のデフォルトドメインを
  使用する。独自ドメインでの提供が必要な場合は Route53 + ACM(us-east-1)証明書を追加し
  `aliases` / `viewer_certificate.acm_certificate_arn` を設定すること
- `price_class` は既定 `PriceClass_100`(北米・欧州のエッジロケーションのみ)とし、
  全世界配信の `PriceClass_All` より配信コストを抑えている。アジアなど他リージョンからの
  レイテンシを優先する場合は `PriceClass_200` 以上に変更する
- web(S3)は `Managed-CachingOptimized` でキャッシュし、CloudFront のエッジでヒットさせて
  S3/データ転送コストを抑える。api/auth のレスポンスはキャッシュしない
  (`Managed-CachingDisabled` ポリシー。token エンドポイント等は特にキャッシュ厳禁)
- CloudFront Function は Lambda@Edge よりリクエスト単価が大幅に安く、単純な URI 書き換え
  (strip / SPA フォールバック)にはオーバースペックな Lambda@Edge を使わず Function で足りる

## S3 バケット名の一意性について

`aws_s3_bucket.web` のバケット名は `${var.name_prefix}-web` のみから決まる(`name_prefix` は
既存変数を流用し、追加のサフィックス変数は持たない)。S3 バケット名は **全 AWS アカウント間で
グローバルに一意**である必要があるため、同名バケットが既に存在する環境では `apply` が
`BucketAlreadyExists` で失敗し得る。その場合は `envs/dev` の `name_prefix`(`project` /
`environment` 変数由来)にアカウント固有のサフィックスを加えるなどして重複を回避すること
(アカウント ID 等の秘密情報をコードに平文で書かないこと)。

## 既知の制約

- CloudFront のアクセスログ(`logging_config`)は有効化していない(S3 ログバケットの
  追加コスト・運用を避けるため)。本番運用ではアクセスログや WAF ログ(Kinesis Firehose 経由)
  を有効化することを推奨する
- web バケットの S3 バージョニング・アクセスログも有効化していない(サンプルとして意図的に
  省略。本番移行時のチェックリストに追加すべき項目)
- CloudFront → ALB 間は HTTP(`origin_protocol_policy = "http-only"`)。詳細は `modules/platform`
  および `modules/service` の README を参照
