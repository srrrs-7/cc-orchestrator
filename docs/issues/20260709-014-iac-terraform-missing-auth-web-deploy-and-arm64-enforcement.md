---
id: ISSUE-014
title: app/iac(Terraform)が SPEC-001(app/api 単体世代)のまま取り残され、コンテナ化された app/auth / app/web を AWS へデプロイする経路が存在しない(IaC ⇄ 3アプリ構成の乖離)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-09
specs: [SPEC-001]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-014: app/iac(Terraform)が SPEC-001(app/api 単体世代)のまま取り残され、コンテナ化された app/auth / app/web を AWS へデプロイする経路が存在しない(IaC ⇄ 3アプリ構成の乖離)

## 1. ユーザー価値への影響(なぜ対応するか)

> **`app/iac` をリファレンスに、このリポジトリの 3 アプリ(api / auth / web)を AWS 上へ展開しようとする開発者** の **「IaC を apply すればアプリ一式がデプロイできる」という期待** が **IaC が app/api 単体世代のまま取り残されており、app/auth / app/web を配置する経路が存在しないことで損なわれる**。

- **影響を受けるユーザー**: `app/iac` を参照して、コンテナ化済みの 3 アプリ(`app/api` / `app/auth` / `app/web`)を AWS 上に展開しようとする、このリポジトリの開発者。
- **損なわれる価値**:
  - (a) 構成の完全性: `app/iac` を apply しても ECS 上に立ち上がるのは `app/api` のみ。`app/auth`(OAuth 2.0 + OIDC 認可サーバー・独立バイナリ)と `app/web`(SPA)を配置する ECS service / ECR / 配信経路が IaC に無く、3 アプリ体制を IaC だけでは再現できない。
  - (b) デプロイ運用の安全性: ECS タスク定義は ARM64(Graviton)前提だが、Dockerfile / compose / Makefile が `linux/arm64` を強制していない。amd64 の開発機で素朴に build → push すると ECS タスクがアーキ不一致で起動せず、原因が分かりにくいフットガンになる(README の手動注意書きに依存)。
  - (c) 手動運用ギャップ: ECR への image build / push を自動化する CI 経路が無く、SPEC-001 が明示的にスコープ外とはしているものの、実運用では手動 push が前提として残る。
- **影響範囲・頻度**: `app/iac` を実際に apply して 3 アプリを展開しようとした時点で顕在化する。現状はインフラが plan / サンプル段階のため潜在的(常時ではなく、3 アプリの AWS 展開に踏み出したときに顕在化)。
- **回避策**:
  - (a) なし — auth / web の AWS 配置は IaC に定義が無い以上、`app/iac` だけでは代替できない(別途手動構築か新 Spec での IaC 追加が必要)。ローカルでの 3 アプリ同時起動は `compose.yml`(DOCKER-001)で代替可能だが、これは AWS デプロイの回避策にはならない。
  - (b) あり — README(`app/iac/envs/dev/README.md:50`、`app/iac/modules/app/README.md:14-15`)の手動注意に従って `linux/arm64` でビルドする運用でカバーできる(コードによる強制ではない)。
  - (c) あり — 手動で ECR へ build / push する(SPEC-001 のスコープ外方針どおり)。

**注**: これは現行ランタイムのバグ(退行)ではなく、Spec のスコープが現状のプロジェクト構成(3 アプリ + コンテナ化)に追いついていない設計乖離・技術的負債として記録する。

## 2. 現象(何が起きているか)

> 個別の不具合(退行)ではなく、「IaC が想定する対象」と「実プロジェクトの現状構成」の差分。

### 期待する動作

- `app/iac` が、リポジトリの実体(`compose.yml` が示す api / auth / web の 3 サービス)に対応するデプロイ先を AWS 上に定義している。あるいは、IaC の対象範囲(app/api のみ)が Spec で明示され、読み手が「IaC = 3 アプリ全部をカバー」と誤解しない。
- ARM64(Graviton)前提が、README の注意書きだけでなくビルド経路(Dockerfile / compose / CI)でも担保され、amd64 環境からの誤ったイメージ push を構造的に防げる。

### 実際の動作

- **app/auth の AWS 配置が無い**: `app/iac/modules/app/ecs.tf:31-131` は単一 ECS タスク定義(family `${var.name_prefix}-api`、L32)+ 単一 ECS service(`${var.name_prefix}-api`、L90)+ 単一コンテナ(`name = "api"`、L47)のみ。`app/iac/modules/app/ecr.tf:5-18` の ECR リポジトリも `${var.name_prefix}-api` の 1 個だけ。一方 `app/auth` は独立バイナリ(`app/auth/cmd/authz`)として `app/auth/Dockerfile` でコンテナ化済みで、`compose.yml:12-22` でも別サービス `auth`(host 8082 / ISSUER=http://localhost:8082)として稼働する実体がある。IaC 側に auth 用の ECS service / ECR / タスク定義が存在しない。
- **app/web の配信経路が無い**: `app/iac/modules/cdn/main.tf:120-146` の CloudFront distribution は origin が ALB のみ(`origin_id = "alb-origin"`、L128)。SPA(静的アセット)を AWS で配信する経路(S3 + CloudFront 静的ホスティング等)が存在しない。一方 `app/web` は `app/web/Dockerfile` + `app/web/nginx.conf` で nginx 配信されるコンテナの実体があり、`compose.yml:24-35` でも別サービス `web`(host 8080 / nginx)として稼働する。
- **ARM64 のビルド強制が無い**: `app/iac/modules/app/ecs.tf:40-43` は `runtime_platform.cpu_architecture = "ARM64"`(コメント L4-6 で「ECR に push するイメージは linux/arm64 でビルドすること」と明記)だが、`app/api/Dockerfile` / `app/auth/Dockerfile` / `app/web/Dockerfile` / `compose.yml` / `Makefile` のいずれにも `--platform` / `linux/arm64` / `arm64` の指定が無い(grep で 0 件)。README(`app/iac/envs/dev/README.md:50`、`app/iac/modules/app/README.md:14-15`)に手動注意書きはあるが、コードでは強制していない。
- **ECR への build/push CI 経路が無い**: `.github/workflows/cicd.yml` に ecr / ecs / image push の記述が 0 件(grep で該当は `.github/workflows/contract-drift.yml:46` の Bun/Dockerfile に関するコメントのみ)。SPEC-001 が「アプリケーションイメージのビルド・プッシュ(ECR リポジトリの用意まで)」をスコープ外と明記している(`docs/specs/20260708-001-aws-ecs-api-infra.md:61`)ため矛盾ではないが、手動 push 前提のまま自動化経路が無い運用ギャップとして併記する。

### 再現手順

1. `app/iac/modules/app/ecs.tf` を開く → タスク定義 family(L32)・service 名(L90)・コンテナ名(L47)がすべて `api` 単一で、auth 用の定義が無いことを確認する。
2. `app/iac/modules/app/ecr.tf` を開く → ECR リポジトリが `${var.name_prefix}-api`(L6)の 1 個のみで、auth / web 用が無いことを確認する。
3. `app/iac/modules/cdn/main.tf` を開く → CloudFront の origin が ALB(`alb-origin`、L126-146)のみで、S3 等の静的配信 origin が無いことを確認する。
4. リポジトリルートで `grep -rni "arm64\|linux/arm\|platform" app/api/Dockerfile app/auth/Dockerfile app/web/Dockerfile compose.yml Makefile` を実行する → 0 件(ARM64 強制が無い)を確認する。
5. `app/iac/modules/app/ecs.tf:40-43` を開く → `cpu_architecture = "ARM64"` が設定されている(=イメージは arm64 でなければ ECS タスクが起動しない)ことを確認する。
6. `grep -rni "ecr\|ecs" .github/workflows/cicd.yml` を実行する → 0 件(ECR への build/push CI が無い)を確認する。
7. `compose.yml` を開く → api / auth / web が別サービスとして定義され、IaC が対象とするのは api のみであることと対比する。

### 環境・条件

- 対象: `app/iac`(Terraform、SPEC-001)。関連: `app/auth`・`app/web`・`compose.yml`・Dockerfile 群(DOCKER-001)、`.github/workflows/cicd.yml`。
- 発見文脈: プロジェクト全体レビューで、IaC と実プロジェクト構成(3 アプリ + コンテナ化)の突き合わせ中に、IaC が app/api 単体世代のまま取り残されている構造的乖離として判明。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/iac/modules/app/ecs.tf` のタスク定義・service・コンテナはすべて `api` 単一(family L32、service L90、container `name = "api"` L47)。auth 用の定義は存在しない。
- 事実: `app/iac/modules/app/ecr.tf:6` の ECR リポジトリは `${var.name_prefix}-api` の 1 個のみ。
- 事実: `app/iac/modules/cdn/main.tf:126-146` の CloudFront origin は ALB のみ(`custom_origin_config`、`origin_id = "alb-origin"`)。S3 オリジン・OAC・静的配信の behavior が無い。
- 事実: `app/auth`(`app/auth/cmd/authz` + `app/auth/Dockerfile`)と `app/web`(`app/web/Dockerfile` + `app/web/nginx.conf`)はコンテナ化済みで、`compose.yml` に `auth`(L12-22)・`web`(L24-35)として別サービスで定義されている。
- 事実: `app/iac/modules/app/ecs.tf:40-43` で `cpu_architecture = "ARM64"`。ビルド経路(Dockerfile 3 つ / compose.yml / Makefile)に `--platform` / `arm64` の指定は grep 0 件。README には手動注意あり(`app/iac/envs/dev/README.md:50`、`app/iac/modules/app/README.md:14-15`)。
- 事実: `.github/workflows/cicd.yml` に ecr / ecs / image push 記述は 0 件。SPEC-001 はイメージの build/push をスコープ外と明記(`docs/specs/20260708-001-aws-ecs-api-infra.md:61`)。
- 事実(時系列): `app/iac`(SPEC-001)は 2026-07-08 に作成(`docs/specs/20260708-001-aws-ecs-api-infra.md` の created / 経緯)。`app/auth` / `app/web` を含む 3 アプリのコンテナ化(DOCKER-001)は 2026-07-09 に計画・実装(`docs/plans/DOCKER-001-plan.md`、`compose.yml` 等のファイル日付 Jul 9)。IaC は 3 アプリ体制になる前に作られている。
- 事実: SPEC-001 の対象ユーザーは「API(`app/api`)を AWS 上に展開したい開発者」(`docs/specs/20260708-001-aws-ecs-api-infra.md:17`)。ただし「スコープ外」節(同 L56-61)は apply 実行・カスタムドメイン・CI/CD・prod 実体化・イメージ build/push を挙げるのみで、「auth / web は対象外」とは明示していない。読み手が「IaC は 3 アプリ全部をカバーする」と誤解しうる。
- 仮説: 本乖離は不具合(退行)ではなく、`app/iac`(SPEC-001)が app/api 単体世代として先に設計・実装され、その後 DOCKER-001 で auth / web を含む 3 アプリがコンテナ化されたことで、IaC が現行構成に追従できていない技術的負債。SPEC-001 のスコープが app/api 単体である事実が Spec に明記されていないため、乖離が「意図的な範囲設定」なのか「未対応の欠落」なのか読み手が判別しづらい状態になっている。

### 根本原因

`app/iac`(SPEC-001)が app/api 単体を対象とする世代のまま作られ、その後(翌日)DOCKER-001 で auth / web を含む 3 アプリがコンテナ化されたが、IaC 側が現行構成に追従していない。加えて SPEC-001 のスコープ(app/api のみ)が Spec 本文で明示されていないため、乖離の意図が読み手に伝わらない。ARM64 前提のビルド強制と ECR への push 自動化も、README の手動注意 / スコープ外方針に依存しており、コード / CI で担保されていない。

## 4. 対応(どう解決するか)

### 対応方針

> 以下は候補であり、本 Issue の時点で確定はしない。(a) は spec-owner(admin + spec skill)判断、(b)(c) は別途 Spec / 計画化を要する。優先度は (a) > (b) ≒ (c) の目安。

- **(a) SPEC-001 のスコープ明記(まず着手推奨・小さい)**: SPEC-001 の「スコープ外」節に「本 Spec は `app/api` のみを AWS デプロイ対象とし、`app/auth` / `app/web` の AWS インフラ化は別 Spec とする」と明記する。現状は対象ユーザー記述からの推測依存で、読み手が「IaC は 3 アプリ全部をカバー」と誤解しうるのを防ぐ(spec-owner 判断。本 Issue の担当外)。
- **(b) auth / web 用インフラの新 Spec 設計**: auth / web を AWS へ展開するインフラを新 Spec として設計する。特に auth は HTTPS issuer + カスタムドメイン / ACM が必要になり(discovery の issuer と実アクセス URL を一致させる必要がある。cf. `docs/plans/DOCKER-001-plan.md:139` の ISSUER 整合)、SPEC-001 の「CloudFront デフォルトドメイン + HTTP」方針(`docs/specs/20260708-001-aws-ecs-api-infra.md:59`)とは別途設計を要する。web は S3 + CloudFront 静的ホスティング等の配信経路の追加を要する。
- **(c) ARM64 強制の担保**: `--platform=linux/arm64` 指定または buildx により、ビルド経路(Dockerfile / CI)で ARM64 を強制する。あわせて ECR への build/push を CI 経路として用意するかは、SPEC-001 のスコープ外方針との整合を含めて別途判断する。

いずれも本 Issue(起票)の範囲では実装しない。担当分担・順序は planner が計画化する。

### 実施内容

- [ ] (a) SPEC-001 の「スコープ外」節に、AWS デプロイ対象が `app/api` のみである旨を明記する(spec-owner 判断)
- [ ] (b) auth / web の AWS インフラ化を新 Spec として起票・設計する(auth の HTTPS issuer + ACM / カスタムドメイン、web の静的配信経路を含む)
- [ ] (c) ARM64 ビルド強制(Dockerfile / CI での `--platform=linux/arm64` or buildx)を検討・導入する
- [ ] (c') ECR への image build/push を自動化する CI 経路の要否を判断する(SPEC-001 のスコープ外方針との整合を確認)

### 再発防止

- Spec には「対象 stack / 対象アプリ」を明示し、対象外を「スコープ外」節に列挙する運用を徹底する(IaC 系 Spec では特に、どのアプリを対象とするかを曖昧にしない)。
- プロジェクト構成が変わる大きな変更(例: DOCKER-001 の 3 アプリ化)を入れる際は、既存 Spec / IaC の前提と突き合わせ、追従が必要な箇所を Issue 化する。本 Issue はその突き合わせで検出されたもの。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。プロジェクト全体レビューで、`app/iac`(Terraform, SPEC-001)が実プロジェクト構成(api / auth / web の 3 アプリ + コンテナ化, DOCKER-001)に追従していない構造的乖離として判明したものを記録。
- 確認した事実:
  - ECS / ECR は api 単体のみ(`app/iac/modules/app/ecs.tf:31-131`、`ecr.tf:5-18`)。auth 用の ECS service / ECR / タスク定義が無い。auth は独立バイナリ・別サービスで稼働する実体がある(`app/auth/cmd/authz`、`app/auth/Dockerfile`、`compose.yml:12-22`)。
  - CloudFront origin は ALB のみ(`app/iac/modules/cdn/main.tf:120-146`)。web(SPA)の AWS 配信経路が無い。web はコンテナ配信の実体がある(`app/web/Dockerfile`、`app/web/nginx.conf`、`compose.yml:24-35`)。
  - ARM64 前提(`app/iac/modules/app/ecs.tf:40-43`)だが、Dockerfile 3 つ / compose.yml / Makefile に `--platform` / `arm64` 指定が無い(grep 0 件)。README に手動注意あり(`app/iac/envs/dev/README.md:50`、`app/iac/modules/app/README.md:14-15`)。
  - ECR への build/push CI 経路が無い(`.github/workflows/cicd.yml` に ecr/ecs 記述 0 件)。SPEC-001 が明示的にスコープ外(`docs/specs/20260708-001-aws-ecs-api-infra.md:61`)のため矛盾ではないが運用ギャップとして併記。
  - 時系列: `app/iac`(SPEC-001)は 2026-07-08 作成、DOCKER-001(3 アプリのコンテナ化)は 2026-07-09。IaC は 3 アプリ体制になる前に作られている。
- 切り分け: 現行ランタイムのバグ(退行)ではなく、Spec のスコープが現状に追いついていない設計乖離・技術的負債として記録した。DB 未接続(`modules/db` の RDS が配線済みだが app/api は in-memory のみ)は ISSUE-001 で既に追跡中のため、本 Issue では重複起票せず関連参照に留めた。ISSUE-002 は単一 api アーキテクチャ内のセキュリティ・可用性強化項目であり、auth / web の未デプロイという本乖離とは別。
- severity は **medium** と判定。判定根拠: 現行ランタイム・SPEC-001 のスコープ(app/api の dev サンプルが `terraform plan` 通ること)の達成は本 Issue で阻害されないため critical / high ではない。一方、(a)3 アプリの AWS 展開に踏み出すと auth / web の配置経路が IaC に無く回避策が無い、(b)ARM64 誤ビルドで ECS タスクが起動しないフットガンが残る、という将来の実害があるため low ではなく medium とした。
- 次にやること: (a) SPEC-001 の「スコープ外」節へのスコープ明記(spec-owner 判断・本 Issue 担当外)、(b) auth / web の AWS インフラ化の新 Spec 化、(c) ARM64 ビルド強制の検討。planner による計画化を推奨。SPEC-001 側 frontmatter の `issues` への相互リンク追記と経緯追記は連動して実施。
