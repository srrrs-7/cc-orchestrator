# SPEC-004 実装計画: app/auth・app/web の AWS デプロイ経路(SPEC-001 の 3 アプリ拡張)

- 起点: `docs/specs/20260709-004-auth-web-aws-deploy.md`(status: approved / ISSUE-014 起点)
- 前提: `docs/specs/20260708-001-aws-ecs-api-infra.md`(api 単体世代)と `docs/plans/SPEC-001-plan.md` の設計を土台に、既存 `app/iac`(network / db / app / cdn + envs/dev)を拡張する
- 対象 stack: `app/iac`(Terraform)を中心に、build-push ツーリング(`Makefile` / `.github`)。**app/auth・app/api のコード変更は不要**(R5 の結論。後述)
- 成果物: `terraform plan` に api・auth の 2 ECS サービスと web(S3+CloudFront)が現れる `app/iac` 一式 + ARM64 build-push / web デプロイ手順
- **`terraform apply` は行わない**(plan/静的検証まで。apply は人間判断)

---

## R5 の確定結論(最重要): `/auth/*` issuer 整合と app/auth 変更の要否

**結論: CloudFront で `/api` / `/auth` プレフィックスを剥がす(strip)方式を採用する。app/auth・app/api のコード変更は不要。SPEC-004 T2.5(impl-api の base-path 対応)は発火しない。**

### 根拠(調査で確定した事実)

- app/auth のルート(`app/auth/route/router.go`)は各エンドポイントを **ルート直下に登録**している: `GET /authorize` / `POST /token` / `GET /userinfo` / `GET /.well-known/openid-configuration` / `GET /.well-known/jwks.json`。
- discovery の絶対 URL は `app/auth/service/discovery_service.go` が **`issuer` 文字列に各パスを単純連結**して作る(`issuer + "/authorize"` 等)。`issuer` は `cmd/authz/main.go` が `ISSUER` env(既定 `http://localhost:8080`)から受ける。
- app/api も同様にルート直下(`/tasks` 等、`app/api/route/router.go`)。
- **DOCKER-001 のローカル契約が既に strip 方式**である: `app/web/nginx.conf` は `location /api/ { proxy_pass $api_upstream/; }`(末尾スラッシュ)で **`/api` を剥がして** api コンテナのルート `/tasks` に渡す。web SPA も `app/web/.../api/client.ts` で `DEFAULT_BASE_PATH = "/api"` を使う。つまり「エッジ(reverse proxy)が `/api` を剥がし、コンテナはルート実装のまま」という契約が確立済み。
- ALB のターゲットグループ **ヘルスチェックは ALB→ターゲットへ直接**行われ、CloudFront やリスナールールを通らない。api の Dockerfile は `/tasks`、auth の Dockerfile は `/.well-known/openid-configuration` をルートで叩く(いずれもルート実装前提)。

### 採用方式の整合(なぜ app/auth 変更が不要か)

- auth の ECS タスクに `ISSUER = "http://<cloudfront-domain>/auth"` を渡す。
- クライアントは discovery を `http://<domain>/auth/.well-known/openid-configuration` に取りに行く → CloudFront の `/auth/*` behavior が受け、CloudFront Function が **先頭セグメント `/auth` を剥がして** origin(ALB→auth コンテナ)へ `/.well-known/openid-configuration` を渡す → コンテナはルート実装のまま 200 を返す。
- discovery ボディの各 URL は issuer 連結で `http://<domain>/auth/authorize` `.../auth/token` `.../auth/userinfo` `.../auth/.well-known/jwks.json` となり、**いずれも `/auth/*` behavior 経由で到達可能**(剥がしてコンテナのルートに落ちる)。issuer と実アクセス経路が一致する(R5 充足)。
- api も対称に `/api/*` を剥がす。api は issuer を持たないので env 変更も不要。

### なぜ SPEC-004 §4 の第一候補(案 i: 剥がさず app/auth を base-path 対応)を採らないか

- 案 i(剥がさない)は **app/auth と app/api の両方**を base-path 対応(env 駆動 mount prefix)に改修する必要がある(no-strip では auth コンテナは `/auth/authorize`、api コンテナは `/api/tasks` を受けるため)。cross-stack のコード変更が 2 stack に波及する。
- strip 方式なら **両アプリともコード無改修**で、かつ **DOCKER-001 のローカル nginx 契約とも一致**する(AWS = CloudFront が nginx の役割を担うという Spec の「二重性」記述とも整合)。
- Spec §4 が案 ii(剥がす)を退けた理由は「issuer をドメイン直下にすると api と衝突」だったが、本方式は **issuer に `/auth` プレフィックスを残したまま(コンテナには剥がして渡す)**ため衝突しない。Spec が案 ii を「ドメイン直下 issuer」に限定して評価していた点を、本計画で「issuer は `/auth` プレフィックス付き・strip はエッジのみ」に精緻化して解決する。

### strip に伴う ALB ルーティングの帰結(方針で詳述)

先頭セグメントを CloudFront で剥がすと、ALB に届くパスは api / auth とも **ルート相対**になりパスで判別できない。よって **ALB はパスではなくヘッダで target group を振り分ける**:

- CloudFront は ALB オリジンを **2 つ**定義する(同一 ALB DNS・別 origin_id)。api 用オリジンは `X-Origin-Verify: <secret>` のみ、auth 用オリジンは `X-Origin-Verify: <secret>` + `X-Target-Service: auth` を custom header で付与。
- ALB リスナールール: 優先度 10 = `X-Origin-Verify` 一致 **かつ** `X-Target-Service=auth` → auth TG、優先度 20 = `X-Origin-Verify` 一致 → api TG、default = fixed 403(既存の二層防御 R3/R6 を維持)。

> 退けた別案: ALB を「剥がさず」パスベースで振り分ける(案 i)。上記のとおりアプリ 2 本の改修が必要になり不採用。header ベースなら無改修。

---

## 方針

SPEC-001 のサンプルグレード(HTTP・デフォルトドメイン・NAT なし・低コスト)を崩さず、既存モジュール資産を拡張して auth/web を「重ねる」。要点は 4 つ。

### 1. モジュール分割: 共有プラットフォーム + 汎用サービスモジュール(2 回呼ぶ)

現 `modules/app` は「ALB + リスナー + ECS クラスタ + ECR + タスク定義 + サービス + IAM + Logs」を api 専用に一体化している。これを **責務で 2 分割**する:

- **`modules/platform`(新規・現 `modules/app` から抽出)**: 共有 ALB / HTTP リスナー(default action = fixed-response 403)/ ECS クラスタ / capacity providers。**サービス非依存**。出力: `alb_dns_name` / `alb_arn` / `listener_arn` / `ecs_cluster_id`。
- **`modules/service`(新規・汎用)**: サービス 1 本ぶんの ターゲットグループ + リスナールール(`listener_arn` に付与)+ ECR リポジトリ + CloudWatch Logs + タスク定義(ARM64)+ ECS サービス + IAM ロール。**api と auth で 1 回ずつ計 2 回呼ぶ**(Spec の「汎用化して 2 回呼ぶ」)。差分(api=DB env/secret + health `/tasks`、auth=ISSUER env + health `/.well-known/openid-configuration`)は呼び出し側の変数で表現。

**なぜ「共有 + 汎用モジュール 2 呼び出し」か(退けた代替案):**

| 代替案 | 退けた理由 |
|---|---|
| 現 `modules/app` を 1 モジュールのまま `services` map + `for_each` で 2 サービス定義 | **モジュール間の循環依存**が発生する。auth の `ISSUER` は CloudFront ドメイン(`module.cdn` 出力)を要し、`module.cdn` は ALB DNS(app モジュール出力)を要する。ALB と ECS を同一モジュールに置くと `module.app ↔ module.cdn` が相互参照になり `terraform` が解決不能。ALB(共有プラットフォーム)を ECS サービスから切り離すことで、依存を一方向 DAG(`platform → cdn → service`)に開ける(下記「循環回避」)。 |
| `modules/app` を api 専用のままコピーして `modules/auth` を新設 | 定義の二重化。iac.md の変数駆動方針・Spec §4 代替案表で明示的に退けられている。汎用 `modules/service` を 2 回呼ぶことで DRY を保つ。 |
| auth/web ごとに ALB / CloudFront を増やす | R6・コスト方針違反(Spec §4 代替案表)。ALB 1 本共用・単一 CloudFront を維持。 |

**循環回避の DAG(この分割が必須である理由):**

```
network → platform(ALB+listener+cluster) → cdn(S3+CloudFront, 依存は ALB DNS のみ)
                    │                          │
                    └──────────────┬───────────┘
                                   ▼
                        service_api / service_auth
   (listener_arn・cluster を platform から、cloudfront ドメインを cdn から受ける。
    api は cloudfront ドメイン不要=issuer 無し。auth のみ ISSUER に注入)
```

CloudFront は **ALB DNS だけ**に依存し、TG/ECS/サービス登録には依存しない(リスナーの default action は fixed-response 403 で TG を参照しない。TG とリスナールールは `modules/service` 側でリスナー ARN を参照して後付けする)。これで一方向 DAG になり、単一 apply で issuer に実 CloudFront ドメインを注入できる。

### 2. web = S3(非公開)+ CloudFront(OAC)を `modules/cdn` に拡張

`modules/cdn` に S3 バケットと OAC を追加し、単一 CloudFront に 3 オリジン + 3 behavior を持たせる(`modules/web` 新設ではなく `cdn` 拡張。バケットポリシーが distribution ARN を要し、OAC と密結合するため 1 モジュールに閉じる方が循環・ARN 受け渡しを避けられる)。

- オリジン: `s3-web`(S3, OAC)/ `alb-api`(ALB DNS, `X-Origin-Verify`)/ `alb-auth`(同 ALB DNS, `X-Origin-Verify` + `X-Target-Service: auth`)。
- behavior:
  - `default`(`*`)→ `s3-web`。cache policy = Managed-CachingOptimized、`default_root_object = "index.html"`。**SPA フォールバック用 CloudFront Function を viewer-request に関連付け**。
  - `/api/*` → `alb-api`。CachingDisabled + AllViewerExceptHostHeader。**strip 用 Function** を関連付け。
  - `/auth/*` → `alb-auth`。CachingDisabled + AllViewerExceptHostHeader、`allowed_methods` に POST を含む(token は POST)。**strip 用 Function** を関連付け。
- 既存 WAFv2 Web ACL(CLOUDFRONT scope / us-east-1)はそのまま同 distribution に関連付け続ける(R6 の非退行)。

**SPA フォールバック方式の確定: CloudFront Function(default behavior 限定)を採用。distribution 全体の `custom_error_response` は退ける。**

- 理由: `custom_error_response`(403/404 → `/index.html`, 200)は **distribution 全体に効く**ため、`/api/*` `/auth/*` の正当な 404/403(API エラー・OIDC エラー)まで SPA の index.html に化けて API セマンティクスを壊す。
- 採用: default(S3)behavior にのみ関連付けた CloudFront Function(viewer-request)で、**拡張子を持たないパス(=クライアントルート)を `/index.html` に書き換える**(nginx の `try_files $uri /index.html` と等価)。`/api/*` `/auth/*` は別 behavior でマッチするため影響しない。ハッシュ付きアセット(`.js`/`.css` 等)は素通り。

CloudFront Function は 2 本(JS ソースを `modules/cdn/functions/` に置く):
- `strip_prefix`: 先頭パスセグメントを 1 つ除去(`/api/tasks`→`/tasks`、`/auth/.well-known/...`→`/.well-known/...`、`/api`→`/`)。`/api/*` と `/auth/*` の両 behavior で **同一 Function を再利用**。
- `spa_fallback`: 拡張子なしパスを `/index.html` に書き換え。default behavior のみ。

### 3. ECR は api / auth ごとに 1 リポジトリ

各 `modules/service` インスタンスが自分の ECR リポジトリ(`${name_prefix}-<service>`、image scanning on、lifecycle policy)を 1 つ作る(結果として api/auth の 2 リポジトリ)。root で `for_each` して渡す形も等価だが、サービス関連リソースをモジュールに閉じる方が凝集度が高い。

### 4. build-push は Make/CI の手順として分離(IaC は「箱」まで)

Terraform は ECR リポジトリと S3 バケットの用意まで(SPEC-001 の「build/push は対象外」を踏襲)。実ビルド/push/sync は手順化:

- `docker buildx build --platform linux/arm64 --push` で api / auth を **ARM64 明示ビルド**(`runtime_platform = ARM64` と齟齬させない = ISSUE-014 のフットガンを塞ぐ)。
- web: `bun run build` → `aws s3 sync dist s3://<bucket> --delete` → `aws cloudfront create-invalidation`。
- 実体は Makefile ターゲット + 任意の手動 `.github` workflow(`workflow_dispatch`)。**自動 apply / 自動デプロイはしない**(Spec スコープ外)。

### auth の運用上の制約(desired_count = 1)

app/auth は **プロセス起動ごとに RSA 署名鍵を生成**し、発行トークンはそのインスタンスでのみ検証可能(`cmd/authz/main.go` のコメント)。複数タスクだと JWKS/kid が不一致になり token/userinfo 検証がタスク間で破綻する。よって **auth サービスは `desired_count = 1`** を既定とする。Fargate Spot 中断や再起動で発行済みトークンが失効するのは app/auth の既知の性質(サンプルとして許容。リスク欄)。api は複数タスク可。

---

## 変更ファイル

### app/iac(impl-iac)

**`modules/platform/`(新規。現 `modules/app` から ALB・リスナー・クラスタを抽出)**
- `alb.tf`: `aws_lb`(既存定義を移設)、`aws_lb_listener`(port80, default `fixed-response 403`)。**TG とリスナールールは持たない**(service 側に移す)。
- `ecs.tf`: `aws_ecs_cluster` + `aws_ecs_cluster_capacity_providers`。
- `variables.tf`: `name_prefix` / `vpc_id` / `public_subnet_ids` / `alb_sg_id` / `tags`。
- `outputs.tf`: `alb_dns_name` / `alb_arn` / `listener_arn` / `ecs_cluster_id`。
- `README.md`: 共有 ALB/クラスタを service から分離した理由(循環回避)、CloudFront→ALB 間 HTTP のトレードオフ(SPEC-001 から継承)。

**`modules/service/`(新規・汎用。現 `modules/app` の TG/ECR/taskdef/service/iam/logs を一般化)**
- `target_group.tf`: `aws_lb_target_group`(port=`var.container_port`, health_check path=`var.health_check_path`, matcher 200)。
- `listener_rule.tf`: `aws_lb_listener_rule`(`listener_arn` に付与、`priority`=`var.listener_priority`、`condition` は `var.route_conditions`(header name→values の list)を `dynamic` で展開。api=`[{X-Origin-Verify=[secret]}]`、auth=`[{X-Origin-Verify=[secret]},{X-Target-Service=[auth]}]`)。
- `ecr.tf`: `aws_ecr_repository` + `aws_ecr_lifecycle_policy`(現 `modules/app/ecr.tf` を汎用名 `${name_prefix}-${service_name}` に)。
- `logs.tf`: `aws_cloudwatch_log_group`(`/ecs/${name_prefix}-${service_name}`)。
- `iam.tf`: task execution role(`AmazonECSTaskExecutionRolePolicy` + `var.secret_read_arns` が非空なら `secretsmanager:GetSecretValue` インラインポリシー)、task role(最小)。
- `ecs.tf`: `aws_ecs_task_definition`(ARM64、container_definitions は `var.environment` / `var.secrets` を注入、`awslogs`)、`aws_ecs_service`(capacity provider mix、`network_configuration` public+assign_public_ip+`var.ecs_sg_id`、`load_balancer` は自分の TG、`depends_on` = リスナールール)。
- `variables.tf`: `name_prefix` / `service_name` / `vpc_id` / `public_subnet_ids` / `ecs_sg_id` / `ecs_cluster_id` / `listener_arn` / `listener_priority` / `container_image` / `container_port` / `health_check_path` / `task_cpu` / `task_memory` / `desired_count` / `use_fargate_spot` / `fargate_*` / `route_conditions`(list(object({header_name, values}))) / `environment`(list(object({name,value}))) / `secrets`(list(object({name,valueFrom}))) / `secret_read_arns`(list(string)) / `log_retention_days` / `tags`。全て type/description 付き、secrets は sensitive。
- `outputs.tf`: `ecr_repository_url` / `target_group_arn` / `ecs_service_name` / `log_group_name`。
- `README.md`: 汎用サービスモジュールの使い方、api/auth の差分(DB 有無・health path・issuer)、ARM64 とイメージ整合、header ベースルーティングの意図。

**`modules/cdn/`(変更)**
- `s3.tf`(新規): `aws_s3_bucket`(web)、`aws_s3_bucket_public_access_block`(全 true)、`aws_s3_bucket_server_side_encryption_configuration`(AES256)、`aws_cloudfront_origin_access_control`(OAC/sigv4/s3)、`aws_s3_bucket_policy`(`cloudfront.amazonaws.com` に `s3:GetObject`、`AWS:SourceArn`=distribution ARN 条件)。
- `functions.tf`(新規)+ `functions/strip_prefix.js` / `functions/spa_fallback.js`: `aws_cloudfront_function` × 2(runtime `cloudfront-js-2.0`、default provider)。
- `main.tf`(変更): distribution に S3/alb-api/alb-auth の 3 オリジン、default→S3(+ spa_fallback function、`default_root_object`)、`ordered_cache_behavior` `/api/*`→alb-api(+ strip)、`/auth/*`→alb-auth(+ strip, POST 許可)。WAF 関連付けは維持。
- `variables.tf`(変更): `alb_dns_name` は維持。追加: `origin_verify_header_name` / `origin_verify_header_value`(sensitive、既存)/ `auth_route_header_name`(既定 `X-Target-Service`)/ `auth_route_header_value`(既定 `auth`)/ web バケット命名用 `name_prefix`(既存)。
- `outputs.tf`(変更): `web_bucket_name` / `web_bucket_arn` を追加(build-push が sync 先に使う)。`cloudfront_domain_name` / `cloudfront_distribution_id` は既存(issuer・invalidation に使う)。
- `README.md`(変更): S3+OAC を ECS(nginx コンテナ)で退けた理由(Spec §4)、SPA フォールバックを Function にした理由(distribution 全体 custom_error_response を退けた理由)、strip Function と header ルーティングの意図、**nginx はローカル compose 専用で AWS では不要**という二重性。

**`modules/app/`(削除)**: `modules/platform` + `modules/service` へ分解して撤去(`git mv` ベースで移設し、差分を最小化)。

**`modules/network/` / `modules/db/`(原則変更なし)**: auth の ECS タスクは api と同じ `ecs_sg`(ALB SG からの ingress 8080、egress 0.0.0.0/0)で足りる(新 SG 不要)。auth は RDS を使わない。網羅確認のみ。

**`envs/dev/`(変更)**
- `main.tf`: `module "network"` → `module "platform"`(network 出力を渡す)→ `module "cdn"`(platform の `alb_dns_name`)→ `module "service_api"`(platform の listener/cluster、db.*、`environment`=DB 群、`secrets`=`[DB_CREDENTIALS]`、`route_conditions`=`[{X-Origin-Verify}]`、`health_check_path="/tasks"`、priority 20)→ `module "service_auth"`(platform の listener/cluster、`environment`=`[{ISSUER="http://${module.cdn.cloudfront_domain_name}/auth"},{PORT}]`、`secrets`=`[]`、`route_conditions`=`[{X-Origin-Verify},{X-Target-Service=auth}]`、`health_check_path="/.well-known/openid-configuration"`、`desired_count=1`、priority 10)。`random_password.origin_verify` は流用。
- `variables.tf`: 追加 — `auth_container_image` / `auth_task_cpu` / `auth_task_memory` / `auth_desired_count`(既定 1)/ `auth_use_fargate_spot` / `auth_route_header_name` / `auth_route_header_value` / `web_price_class`(既存 `price_class` 流用可)等。api 側は既存流用。全て type/description。
- `outputs.tf`: 追加 — `web_url`(= `https://<cloudfront_domain>`)/ `api_ecr_repository_url` / `auth_ecr_repository_url` / `web_bucket_name` / `cloudfront_distribution_id`。既存 `cloudfront_domain_name` / `alb_dns_name` は維持。
- `terraform.tfvars`: auth の最小タスクサイズ・`auth_desired_count=1` 等の dev 値。**秘密情報は書かない**。
- `versions.tf`: provider 追加なし(CloudFront Function は aws provider 内)。既存の aws/random 固定を維持。
- `README.md`: 3 アプリ構成の使い方、`init-local` + `validate` 手順、apply は人間判断、**web は apply 後に build-push が別途必要**、auth issuer が CloudFront ドメイン依存で単一 apply で解決する旨。

**`moved` ブロック(非退行の担保)**: `modules/app` 撤去に伴い api リソースのアドレスが `module.app.*` → `module.service_api.*`(および `module.platform.*`)へ移る。実 state を持つ利用者のために `envs/dev` に **cross-module `moved` ブロック**(例: `moved { from = module.app.aws_ecs_service.this to = module.service_api.aws_ecs_service.this }` 等、ALB/リスナー/クラスタ/TG/ECR/taskdef/IAM/logs ぶん)を用意し、破壊的再作成ではなく move として計画されるようにする。SPEC-001 は未 apply 想定だが、安全側に倒す。

### build-push ツーリング(impl-ci)

- **`Makefile`(リポジトリ root、既存の compose 用に追記)**:
  - `push-images`: `terraform -chdir=app/iac/envs/dev output -raw {api,auth}_ecr_repository_url` を読み、`aws ecr get-login-password | docker login` 後、`docker buildx build --platform linux/arm64 --push -t <repo>:<tag> app/api`(auth も同様)。**ARM64 固定**(amd64 誤ビルド防止 = R4)。
  - `deploy-web`: `cd app/web && bun run build` → `aws s3 sync dist s3://$(terraform output -raw web_bucket_name) --delete` → `aws cloudfront create-invalidation --distribution-id $(terraform output -raw cloudfront_distribution_id) --paths '/*'`。
  - いずれも **手動実行前提**(help に「apply/認証情報が前提。agent は実行しない」と明記)。
- **`.github/workflows/deploy.yml`(新規・任意)**: `on: workflow_dispatch` のみ。QEMU(`docker/setup-qemu-action`)+ buildx(`docker/setup-buildx-action`)で arm64、OIDC ロール前提で ECR push / S3 sync / invalidation。既存 `cicd.yml`(CI only・apply 無し)と分離し、**自動トリガーを持たない**。プレースホルダ(ロール ARN 等)は変数/secrets 参照でコードに平文を書かない。

> build-push 系のコマンド契約は将来 `.claude/rules/iac.md` の「コマンド」表に追記する余地があるが、本計画では触れない(orchestration のメタ作業。必要なら別途 admin が判断)。

### app/auth・app/api(変更なし)

R5 の結論により **コード変更不要**。SPEC-004 T2.5 は発火しない。

---

## 手順

依存と非退行(既存 api 経路を壊さない)を踏まえた順序。並列可能箇所を明示する。

1. **(前提)admin: tester を先行させるか判定** — 成果物は Terraform サンプルでロジック単体テスト対象を持たない(SPEC-001 と同様)。**TDD は行わず checker の静的検証に集約**(テスト戦略参照)。tester は「api 経路非退行の確認観点」レビューに回す(手順 5 に併記)。

2. **impl-iac(単一 agent・逐次): `app/iac` の再構成と拡張** — モジュール間で型・出力の整合(platform→cdn→service、cdn 出力の issuer 注入、`moved` の網羅)を取り合うため **1 agent に一括委譲**する。実装順の推奨(依存の下流から上流へ、かつ **既存 api を先に無退行で通す**):
   1. `modules/platform` を現 `modules/app` から抽出(ALB/リスナー/クラスタ)。
   2. `modules/service` を汎用実装(現 `modules/app` の TG/ECR/taskdef/service/iam/logs を一般化)。
   3. `envs/dev` を配線: まず **api だけ**を `platform` + `service_api` + 既存 `cdn`(default→ALB のまま)で組み直し、`moved` を入れて **既存 api 経路が move として無退行で validate/plan 差分に出る**ことを確認できる状態にする(非退行チェックポイント)。
   4. `modules/cdn` を拡張(S3+OAC、3 オリジン、behaviors、Function 2 本、SPA フォールバック、`web_bucket_*` 出力)。
   5. `envs/dev` に `service_auth` を追加し、`cdn` の behavior を `default→S3` / `/api/*→alb-api` / `/auth/*→alb-auth` に切替、auth の `ISSUER` に `module.cdn.cloudfront_domain_name` を注入。
   6. 各 module の README(コスト理由・トレードオフ・nginx 二重性、R7)を更新。

3. **impl-ci(単一 agent・逐次): build-push ツーリング** — root `Makefile` に `push-images` / `deploy-web`、`.github/workflows/deploy.yml`(workflow_dispatch)を追加。**手順 2 とはファイルが独立**(`app/iac/*.tf` に触れず、terraform output 名にのみ依存)なので **手順 2 と並列可**。ただし output 名(`web_bucket_name` / `*_ecr_repository_url` / `cloudfront_distribution_id`)は手順 2 の `envs/dev/outputs.tf` と契約するため、admin が output 名を先に確定して両 agent へ共有する。

4. **checker(逐次): 静的検証** — `app/iac` で `make check`(= `fmt-check`(root)→ `validate`(`init-local`)→ `lint`(tflint)→ `security`(trivy))。CloudFront Function の JS 構文誤りは validate では出ないため、`aws_cloudfront_function` の `publish=false` 化や別途 lint は不要だが、JS は手動レビューで確認。失敗は 2/3 に差し戻して反復。**5(レビュー)に進む前提**。

5. **review-security / review-performance / review-spec(3 並列): レビュー** — checker 通過後。
   - review-security: ALB の CloudFront 限定(`X-Origin-Verify` + prefix list SG の二層防御が auth 経路でも維持されるか)、S3 が非公開 + OAC のみ許可か、バケットポリシーの `SourceArn` 限定、WAF 継続関連付け、strip Function がパストラバーサル(`/auth/../`)を許さないか、秘密の平文なし(R6)。
   - review-performance: 追加タスク(auth 1 本)/ CloudFront キャッシュ方針(S3=CachingOptimized、API=Disabled)/ Function 実行コスト / desired_count / Spot 構成 / 追加固定費が小さいこと(Spec 非機能)。
   - review-spec: R1〜R7 の充足(下記トレーサビリティ表)、スコープ外(apply/HTTPS issuer/カスタムドメイン)に踏み込んでいないか、**既存 api 経路の非退行**(`/api/*` で api に到達し、TG/ECS/ECR が破壊再作成でなく move)。
   - review 補助として **api 経路非退行の観点**(手順 1 の tester 分)をここで突き合わせる。

6. **指摘対応(逐次)**: Blocker/Major は impl-iac / impl-ci に差し戻し、4→5 を再実行。今回対応しない指摘(例: S3 access logging / versioning・CloudFront ログ等のサンプル省略)は **ISSUE-002 の本番移行チェックリストに追記 or 新規 Issue** を issue-creator に委譲。

7. **完了処理(admin)**: SPEC-004 の frontmatter `status` を `in-progress`(本計画で更新済み)から、レビュー完了後に `done` へ。ISSUE-014 のステータス・経緯も skill 手順で更新。各 module README 反映確認。

> 並列構造まとめ: **手順 2(impl-iac)と手順 3(impl-ci)は並列**(output 名の契約を先に固定)。手順 5 のレビュー 3 種は並列。手順 4(checker)は 2/3 の後・5 の前で直列。

### 要件トレーサビリティ(R → 実装 / 検証)

| 要件 | 実装箇所 | 検証 |
|---|---|---|
| R1 auth を ECS Fargate(ARM64)で ALB 配下に追加(専用 TG・health `/.well-known/openid-configuration`・専用 ECR) | `modules/service`(auth インスタンス)+ `envs/dev` service_auth | validate / plan(要 creds)/ review-spec |
| R2 web を S3(非公開)+ CloudFront(OAC)、SPA フォールバック | `modules/cdn`(s3.tf・spa_fallback function) | validate / trivy(S3 非公開)/ review-security |
| R3 単一 CloudFront で default→S3 / `/api/*`→api / `/auth/*`→auth | `modules/cdn`(3 オリジン+behaviors+strip)+ `modules/service`(listener rule header 振り分け) | validate / plan / review-spec |
| R4 ARM64 ビルド強制 + ECR push、web dist ビルド + S3 sync + invalidation | root `Makefile`(push-images/deploy-web)+ `.github/deploy.yml` | 手順のコマンドレビュー / review-spec |
| R5 auth OIDC issuer/discovery 絶対 URL 整合(`/auth/*`) | strip 方式(cdn Function)+ auth `ISSUER=http://<cf>/auth`(**app/auth 無改修**) | validate / review-spec /(要 creds 時)discovery 実取得 |
| R6 秘密平文なし・NAT 不使用・ALB 1 本共用 | 既存 network/db + `modules/platform`(単一 ALB)+ random_password 流用 | trivy / review-security |
| R7 modules+envs/dev 維持・環境差分は変数・README にコスト理由 | 分割後の各 module + `envs/dev` + README | fmt / validate / review-spec |

---

## テスト戦略

**結論: TDD(tester 先行)は行わず、checker の静的検証に集約する(SPEC-001 と同方針。testing.md の iac 方針)。**

- 理由: 成果物は AWS リソース宣言の Terraform で単体テスト対象のロジックを持たず、実リソース検証には AWS 認証情報が要る(本環境に無い)。
- **認証情報なしで実行(checker)**: `app/iac` で `make check` = `terraform fmt -check -recursive`(root)/ `terraform validate`(`init -backend=false`)/ `tflint --recursive`(`envs/dev`)/ `trivy config .`(`envs/dev`)。CloudFront Function の JS は validate 対象外のため手動レビューで確認する。
- **api 経路の非退行確認**:
  1. 手順 2-iii の中間状態(api のみ再配線 + `moved`)で `terraform validate` が通り、`moved` により api リソースが **move(create/destroy でない)**として扱われることをコードレビューで確認。
  2. 最終状態で、api の TG/ECS サービス/ECR のリソース名・identifier が SPEC-001 から変わらない(name_prefix 由来で不変)ことを diff で確認。
  3. `/api/*` behavior + strip Function により web SPA の `DEFAULT_BASE_PATH="/api"` 契約が維持される(nginx の strip と等価)ことを review-spec が突き合わせ。
- **creds がある環境での追加検証(ユーザー実施・agent は実行しない)**: `terraform plan` で「auth ECS サービス・auth TG・auth ECR・S3・OAC・3 behavior」が新規、api 関連が move/no-op、NAT 不在を確認。apply 後に discovery(`/auth/.well-known/openid-configuration`)の issuer と各 endpoint URL が `http://<cf>/auth/...` で一致することを実取得で確認。
- 観点(正常/異常/境界)は静的検証 + レビューで代替: 正常=validate/plan 差分、異常=trivy(S3 公開・平文秘密の検出なし)、境界=strip Function の空パス(`/api`→`/`)・拡張子判定の SPA フォールバックをレビューで確認。

---

## リスク / 未確定事項

- **R5 の残余**: strip 方式で issuer 整合は取れるが、web SPA を OIDC RP として **実際に auth と繋ぐ配線(redirect_uri 登録・client 設定)は本 Spec のスコープ外**。app/auth の demo client の `redirect_uri` は `http://localhost:3000/callback` 固定(`cmd/authz/main.go` の seed)で、CloudFront 経由の web からのログインは別途 seed/設定が要る。SPEC-004 は「デプロイ経路」までとし、RP 連携は将来 Issue とする(推測で seed を書き換えない)。
- **auth の署名鍵が揮発 / desired_count=1 制約**: app/auth はプロセス毎に RSA 鍵を生成し、トークンは発行インスタンスでのみ有効(app/auth の既知の性質)。よって auth は `desired_count=1` 既定・Spot 中断/再起動でトークン失効。マルチタスク化・鍵の外部化(KMS/Secrets)は app/auth 側の別課題(サンプル範囲外)。impl-iac は 1 タスク前提で配線し、README/リスクに明記。
- **SPA フォールバック方式**: default behavior 限定の CloudFront Function を採用(distribution 全体 `custom_error_response` は API エラーを化けさせるため不採用)。拡張子ベースの判定は「拡張子付きだが実在しないアセット」を index.html に落とさない(404 のまま)ため厳密には nginx の `try_files` と挙動が微妙に異なる。深いクライアントルート(拡張子なし)は index.html に落ちる想定で問題ないが、レビューで妥当性を確認。
- **ARM64 buildx が amd64 ランナーでエミュレーション**: `.github/deploy.yml` を GitHub の amd64 ランナーで動かすと `--platform linux/arm64` は QEMU エミュレーションになりビルドが遅い(場合により失敗)。`docker/setup-qemu-action` を前提にし、恒常運用なら arm64 ランナー(self-hosted / larger runner)を推奨(README に注記)。ローカルの Apple Silicon なら native。
- **nginx の二重性**: `app/web/nginx.conf` と web の Dockerfile(nginx runtime)は **ローカル compose 専用**。AWS では S3+CloudFront が静的配信 + reverse proxy(strip)を担い **nginx は使わない**。両者は別物である旨を `modules/cdn/README.md` と `envs/dev/README.md` に明記(混同で「AWS でも nginx コンテナが要る」と誤解しないように)。
- **`moved` の網羅漏れ**: `modules/app` 分解で move 漏れがあると当該リソースが destroy/create になり api を壊す。impl-iac は分解対象(ALB/リスナー/クラスタ/TG/ECR/taskdef/service/IAM×2/logs)を列挙して `moved` を網羅。実 state 未 apply 前提だが安全側。
- **trivy の新規指摘(サンプル省略)**: web S3(アクセスログ・バージョニング未設定)や CloudFront(アクセスログ未設定)で trivy が指摘し得る。SPEC-001 の ISSUE-002(本番移行チェックリスト)と同性質の意図的サンプル省略として扱い、対応しない指摘は ISSUE-002 追記 or 新規 Issue に回す(checker/review で握る)。
- **S3+OAC バケットポリシーの循環**: バケットポリシーは distribution ARN を要し、distribution はバケットの regional domain + OAC を要する。**同一モジュール(cdn)内**なら Terraform が順序解決するため 1 モジュールに閉じる(modules/web 分割を退けた理由でもある)。
- **AWS アカウント ID / リージョン / backend バケット**: SPEC-001 と同じくプレースホルダ。web バケット名はグローバル一意が要るため `name_prefix` + サフィックス(account_id 等)で衝突回避する設計をユーザーが最終確認(コードに平文アカウント ID を書かない)。
- **build-push コマンド契約の rules 反映**: `push-images` / `deploy-web` を `.claude/rules/iac.md` のコマンド表に載せるかは admin のメタ判断(本計画では未反映)。
