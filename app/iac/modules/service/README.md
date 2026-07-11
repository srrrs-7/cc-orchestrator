# service module

汎用の「共有 ALB 配下で動く Fargate サービス 1 本ぶん」モジュール。ターゲットグループ +
リスナールール、ECR リポジトリ、ECS タスク定義/サービス、IAM ロール、CloudWatch Logs を
まとめて作成する。ALB / HTTP リスナー / ECS クラスタ自体は `modules/platform` が持ち、この
モジュールは `listener_arn` / `ecs_cluster_id` を受け取って自分のリソースを紐付けるだけの
「箱」である(SPEC-004)。

`envs/dev` から **api / auth それぞれ 1 回ずつ、計 2 回**呼ばれる(`module.service_api` /
`module.service_auth`)。差分(DB env・secrets の有無、ヘルスチェックパス、ルーティング条件)は
すべて呼び出し側の変数で表現し、モジュール定義自体は 2 つのアプリで共通(旧 `modules/app` を
api 専用のままコピーして `modules/auth` を新設する案は定義の二重化を招くため退けた)。

## なぜ ALB/クラスタと同じモジュールに置かなかったか

`docs/plans/SPEC-004-plan.md` の「循環回避の DAG」を参照。ALB を ECS サービスと同じモジュールに
置くと、auth の `ISSUER` が要求する `module.cdn`(CloudFront ドメイン)出力と、`module.cdn` が
要求する ALB DNS 名(旧 `modules/app` 出力)が相互参照になり `platform ↔ cdn` の循環が生まれる。
ALB/リスナー/クラスタを `modules/platform` に切り出し `platform → cdn → service` の一方向 DAG に
することで、単一 `apply` で auth の issuer に実 CloudFront ドメインを注入できる。

## ヘッダベースルーティングを使う理由(listener_rule.tf)

`modules/cdn` は CloudFront Function で `/api` `/auth` の先頭パスセグメントを剥がしてから ALB
オリジンへ転送する(R5 の strip 方式。詳細は `modules/cdn/README.md`)。剥がした後は api も auth も
ALB にはルート相対パスで届くため、**パスでは判別できない**。そこで `var.route_conditions` で
ALB リスナールールの `condition` を複数指定し(同一ルール内の複数 `condition` ブロックは AND)、
ヘッダで振り分ける:

- api: `[{header_name = "X-Origin-Verify", values = [<secret>]}]`(優先度 20)
- auth: `[{header_name = "X-Origin-Verify", values = [<secret>]}, {header_name = "X-Target-Service", values = ["auth"]}]`(優先度 10)

`X-Origin-Verify` は CloudFront が生成するカスタムオリジンヘッダで、`network` モジュールの
プレフィックスリスト SG(IP レベルの制限)に加えアプリケーション層でも CloudFront 経由である
ことを検証する **二重防御**になる(R3)。プレフィックスリストは「CloudFront 全体の送信元 IP 帯」
であり別ディストリビューションからのアクセスも通過し得るため、ヘッダ検証がこれを補完する。
`X-Target-Service` はセキュリティ境界ではなく、あくまで api/auth を区別するためのルーティング
専用ヘッダである。ALB の HTTP リスナー(`modules/platform`)の default action は `fixed-response`
で 403 を返すため、いずれの条件にもマッチしないリクエストは forward されない。

## コスト上の選択理由

`modules/app`(旧)から引き継いだ判断はそのまま有効:

### Fargate(ARM64/Graviton)+ Fargate Spot 併用を採用し EC2 起動タイプを退けた理由

- ECS の EC2 起動タイプは、コンテナ以外にホスト EC2 インスタンスの管理(パッチ適用・
  スケーリング・キャパシティプランニング)という運用負荷がかかる。小規模なサンプルでは
  Fargate のサーバーレス運用のメリットがコスト差を上回る
- Fargate は ARM64(Graviton)の方が x86_64 より vCPU/メモリ単価が 約20% 安い。
  `runtime_platform.cpu_architecture = "ARM64"` を採用しているため、ECR に push する
  イメージは **必ず linux/arm64 でビルド**する必要がある(R4)。api / auth ともに同じ制約を受ける
- dev 環境では `use_fargate_spot = true`(既定)とし、`capacity_provider_strategy` で
  `FARGATE`(on-demand, 既定 weight=0/base=0)と `FARGATE_SPOT`(既定 weight=1)を
  併用する。既定値では **タスクは実質すべて Spot 容量で起動**し、on-demand Fargate 比で
  最大 70% 程度のコスト削減が見込める。可用性を優先したい場合は
  `fargate_base` を 1 以上に設定し、最低 1 タスクを on-demand で確保できる

#### トレードオフ: 既定値は単一障害点(SPOF)であることを明示

- 既定値(`fargate_base = 0`, `fargate_weight = 0`, `fargate_spot_weight = 1`,
  `desired_count = 1`)を組み合わせると、実質 **100% Spot・タスク数 1** の構成になる。
  これは意図的な設定であり、dev サンプルではコスト最小化を可用性より優先する方針による
- この既定構成では、AWS が Spot 容量を回収(interrupt)した瞬間に稼働中の唯一のタスクが
  失われ、後続タスクが起動するまで対象サービスが全断する。つまり **既定値は単一障害点
  (SPOF)を許容する設計**であり、可用性が必要な用途にそのまま使うべきではない
- 単一障害点を解消する選択肢(いずれか、または両方を組み合わせる):
  - `fargate_base = 1` に設定し、最低 1 タスクを on-demand(Spot 回収の影響を受けない)で
    確保する。on-demand 1 タスク分の増分コストは概算 約$6/月(ARM64, 256/512 の場合)
  - `desired_count = 2` 以上に設定し、Spot タスクが同時に複数稼働するようにする(Spot でも
    同時に全タスクが回収される確率は下がるが、ゼロにはならない点に注意)
  - 本番相当の可用性が必要な環境では、上記に加えて `fargate_base = 1` かつ
    `desired_count >= 2` の組み合わせを推奨する
- タスクサイズは既定 `task_cpu=256`(0.25 vCPU)/ `task_memory=512`(0.5GiB)の最小構成。
  実際のワークロードに応じて `envs/dev` の tfvars で調整する

### auth を `desired_count = 1` に固定する理由(サービス固有の運用制約)

`app/auth` はプロセス起動ごとに RSA 署名鍵を生成し、発行したトークンはそのインスタンスでしか
検証できない(`cmd/authz/main.go` のコメント参照)。複数タスクにすると JWKS/kid がタスク間で
不一致になり token/userinfo 検証が破綻するため、`envs/dev` は auth 呼び出しで
`desired_count = 1` を既定にする。Spot 中断・再起動時は発行済みトークンが失効するが、
サンプルとして許容する既知の制約(将来、鍵の外部化・マルチタスク化は `app/auth` 側の別課題)。
api はこの制約を受けないため複数タスク可。

### ヘルスチェックパスはサービスごとに呼び出し側が変数で渡す

- api: `GET /health`(認証不要の liveness プローブ。task API は SPEC-015 以降 Bearer JWT 必須)
- auth: `GET /.well-known/openid-configuration`(discovery エンドポイント。`app/auth` はルート
  直下に実装しているため、CloudFront の strip 後に ALB → ターゲットへ直接届くこのパスで判定できる。
  ALB のヘルスチェックは CloudFront/リスナールールを経由せず ALB→ターゲットへ直接行われるため、
  ヘッダ条件の影響を受けない)

いずれも `var.health_check_path` として `envs/dev` が指定する。モジュール自体は特定アプリの
パスをハードコードしない。

### IAM ロールを実行ロール/タスクロールに分離した理由

- 実行ロール(`task_execution`)は `AmazonECSTaskExecutionRolePolicy` に加え、
  `var.secret_read_arns` が非空のときのみ `secretsmanager:GetSecretValue` のインラインポリシーを
  付与する(api は DB マスターシークレットの ARN を渡す、auth は空リストで付与なし)
- タスクロール(`task`)は api・auth いずれもランタイムで AWS API を呼ばない(api は
  in-memory リポジトリ、auth はプロセス内で鍵生成)ため、権限を一切付与しない最小権限構成
  としている。将来 S3 等を使う場合はここに追加する

### ECR はサービスごとに 1 リポジトリ

`${name_prefix}-${service_name}` の名前で `image scanning on` + 14 日 lifecycle policy 付きの
リポジトリを 1 つ作る。api / auth のインスタンスがそれぞれ 1 つ持つため、結果として 2 リポジトリ
になる(`for_each` でリポジトリを複数まとめて定義する形も等価だが、サービス関連リソースを
モジュールに閉じたほうが凝集度が高いためこの形を採る)。これはアプリ本体イメージ用であり、
migrate コンテナ用の共有 `app/migrator` イメージのリポジトリは(api/auth で共用のため)この
モジュールではなく `envs/dev` 側(`aws_ecr_repository.migrator`)に 1 つだけ作る(上記
「マイグレーション init コンテナ」参照)。

## ForceNew なリソース名を呼び出し側から上書きできる理由

ターゲットグループ名(`aws_lb_target_group.name`)、タスク実行/タスク IAM ロール名
(`aws_iam_role.name`)、実行ロールのインラインシークレット読み取りポリシー名
(`aws_iam_role_policy.name`)はいずれも AWS プロバイダ上 ForceNew(rename API が無く、
`name` を変えると replace になる)。既定ではすべて `"<name_prefix>-<service_name>-*"` の
命名だが、`target_group_name` / `task_execution_role_name` / `task_role_name` /
`secrets_policy_name` の 4 変数(いずれも既定 `null`)で個別に上書きできる。

これは、`modules/app`(旧, SPEC-001)を `modules/platform` + `modules/service` に分解した際、
api の一部リソース名にサービス修飾(`-api-`)が入ることで意図せず旧名からズレ、`moved` を
使っても実 `terraform plan` では replace になってしまう問題への対処(`envs/dev/main.tf` の
`module.service_api` 呼び出しで 4 変数すべてに旧 `modules/app` 時代の名前
(`<name_prefix>-tg` 等、サービス修飾なし)を明示的に渡している)。ECR リポジトリ名 / ロググループ名 /
ECS サービス名・タスクファミリは元々 `service_name = "api"` を含んでいたため旧名と一致しており、
上書きは不要。auth は新規リソースなので replace の概念が無く、4 変数とも既定
(`<name_prefix>-auth-*`)のままでよい。**単一の base 変数で一括置換する設計は採らない**:
リソースごとに旧サフィックス規則が異なり(ECR やロググループは元々 `-api` を含む一方、TG は
含まない、等)、一括置換では正確な旧名復元ができないため。

## マイグレーション init コンテナ(SPEC-005 R5)

`var.migration_environment` が非空のとき、`aws_ecs_task_definition.this` の
`container_definitions` に `"<service_name>-migrate"` という非 essential なコンテナが
アプリコンテナより前に追加され、アプリコンテナ側に
`dependsOn = [{ containerName = "<service_name>-migrate", condition = "SUCCESS" }]`
が付与される(`ecs.tf`)。ECS はまずこの migrate コンテナを起動し、**exit code 0(SUCCESS)で
終了するまでアプリコンテナを起動しない**(non-zero 終了時はタスク自体が失敗する)。ECS のタスク
定義には Kubernetes の `initContainers` に相当する専用フィールドが無いため、この
`dependsOn`/`condition` によるサイドカー順序制御が事実上の init コンテナ実装になる。
`var.migration_environment` が空(既定)の場合は `local.migration_container_defs` が `[]` になり、
`aws_ecs_task_definition.this` は SPEC-005 以前と完全に同じ 1 コンテナ構成のまま変化しない。

### database ブートストラップをどこでやるか(SPEC-005 リファクタリング: app/migrator)

初回実装(同一データベース内の別スキーマ)は、goose 自身のバージョン管理表(`goose_db_version`)を
置く対象スキーマ自体を誰が事前に作るかという鶏卵問題を migrate イメージのエントリポイントで
解いていた。ユーザー指示によるリファクタリングで **api / auth は同一 RDS インスタンス上の
別データベース**(`DB_NAME` = api=`"api"` / auth=`"auth"`)に分離され、スキーマ・`search_path`は
廃止された。データベース自体も Terraform(例: `postgresql` プロバイダ)では作れない事情は変わらない
(RDS が `publicly_accessible = false` の private subnet 内にあり、Terraform を実行する場所(CI
ランナー・開発者端末)から直接到達できないため。VPN/踏み台が別途必要になり運用コストが高い)。

採用した方式は、**共有 `app/migrator` イメージ**(単一 Go バイナリ、api/auth 双方の migrate
コンテナが `-target api` / `-target auth` の違いだけで同じイメージを実行する)が、(a)
`DB_MAINTENANCE_NAME`(既定 `postgres`)へ接続して対象データベース(`DB_NAME`)が無ければ
`CREATE DATABASE` を実行し、(b) 対象データベースへ再接続して goose を **ライブラリとして**
呼び出し `db/migrations` を適用する、というもの。この migrate コンテナは ECS タスクの中(= RDS
と同じ VPC 内)で実行されるため到達性の問題が無く、データベース作成も goose 実行前に完結するため
鶏卵問題も起きない。`envs/dev` はこのコンテナに渡す `DB_*` 環境変数(接続先・対象データベース名・
メンテナンス用データベース名)・`migration_command`(`-target` 選択)・secrets(master 資格情報の
JSON key)を配線するところまでを担い、実際の DB 作成・マイグレーション実行ロジックは
`app/migrator` 側の責務として本モジュールは関知しない。

### イメージ・並行実行・代替案

- `var.migration_image` に own-ECR デフォルトは無い(`container_image` と異なる)。api と auth の
  migrate コンテナはどちらも**同じ共有 `app/migrator` イメージ**(`envs/dev` の
  `aws_ecr_repository.migrator`、1 リポジトリのみ)を参照し、`var.migration_command` の
  `-target api` / `-target auth` だけで挙動を分ける設計のため、呼び出し側(`envs/dev`)が
  `migration_image` を明示的に渡す(SPEC-005 plan RF.1.2)。**このイメージの push 経路
  (CI/Makefile 拡張)は本 Spec の範囲外として後続に委ねられている**。`envs/dev` にこのコンテナを
  配線した時点では `:latest` タグはまだ存在しないため、そのまま `apply` すると新しいデプロイの
  タスクがイメージ pull に失敗して起動できず、ロールアウトが詰まる(既存タスクは running のまま
  残る)。**共有 migrator イメージを push してから apply すること**
- 並行実行の競合(SPEC-005 plan RF.6.1 RF-f 系譜): 同一サービスで `desired_count` を 2 以上に
  すると、ローリングデプロイの瞬間に複数の新タスクが同時に起動し、それぞれの migrate コンテナが
  同じデータベースに対して `CREATE DATABASE`(冪等: 重複時の `SQLSTATE 42P04` を成功扱い)/
  `goose up` を並行実行し得る。`envs/dev` の既定 `desired_count = 1`(auth は JWKS 単一化の制約
  からも 1 固定)ではこの競合は発生しない。`desired_count` を増やす場合は、goose 側の
  アドバイザリロック(app/migrator 側で有効化するかどうかは impl-db の責務)か、下記の代替案への
  切り替えを検討すること。api の migrate コンテナは `api` データベースのみ、auth の migrate
  コンテナは `auth` データベースのみを触るため、**api と auth の migrate コンテナ同士が競合する
  ことは無い**(別データベースへの `CREATE DATABASE`/`goose up` は独立)
- 代替案として **一回限りの ECS タスク**(`aws_ecs_service` を持たない `aws_ecs_task_definition`
  を用意し、デプロイパイプラインや手動で `RunTask` する方式)も検討した。init コンテナ方式より
  「デプロイの度に自動適用される」利便性は劣るが、並行実行の競合を「実行主体を 1 回に限定する」
  ことで構造的に避けられる。本モジュールでは実装せず、必要になった場合の設計選択肢として
  ここに記録するに留める(SPEC-005 plan §1.2 の採用理由参照)

## CloudFront ⇔ ALB 間が HTTP である点のトレードオフ

本サンプルはカスタムドメイン / ACM 証明書を使わない(Spec スコープ外)ため ALB は HTTP(80)の
みで、CloudFront → ALB 間は AWS バックボーン内とはいえ TLS 終端されない。カスタムヘッダの値も
この区間では平文で流れる。ACM 証明書とカスタムドメインを用意できる環境では、ALB に HTTPS
リスナーを追加し `cdn` モジュールの `custom_origin_config` を `https-only` に変更することを
推奨する(詳細は `modules/platform/README.md` および `modules/cdn/README.md`)。api・auth とも
同じ制約を受ける。
