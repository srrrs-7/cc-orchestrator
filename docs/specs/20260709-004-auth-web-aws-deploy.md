---
id: SPEC-004
title: app/auth・app/web の AWS デプロイ経路(SPEC-001 の 3 アプリ拡張)
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-09
updated: 2026-07-09
issues: [ISSUE-014]       # 関連Issue ID
supersedes: null # 置き換える旧Spec ID
---

# SPEC-004: app/auth・app/web の AWS デプロイ経路(SPEC-001 の 3 アプリ拡張)

## 1. ユーザー価値(なぜ作るか)

> **このリポジトリで 3 アプリ(api / auth / web)を AWS 上に展開したい開発者** が **`app/auth`(OIDC 認可サーバー)と `app/web`(SPA)も `app/iac` の Terraform だけでデプロイできるようになり**、**現在 api 単体世代で止まっている IaC が実プロジェクト(3 アプリ)全体をカバーし、web/auth を AWS でどう出すか分からないという空白** を無くす。

- **対象ユーザー**: `app/iac` を使って cc-orchestrator の 3 アプリを AWS に展開・参照したい開発者(および同じ構成を再利用する人)
- **解決する課題**: SPEC-001 で作った `app/iac` は **app/api 単体世代**のまま。その後 DOCKER-001 で `app/auth` / `app/web` がコンテナ化・ローカル(compose)では 3 アプリ完結する一方、AWS 側は auth 用 ECS/ECR も web 用の静的配信経路も存在しない(ISSUE-014)。この IaC だけでは現リポジトリの 3 アプリを本番相当にデプロイできない。
- **得られる価値**:
  - `app/iac/envs/dev` の `terraform plan` に auth サービス(ECS)と web(S3 + CloudFront)が現れ、3 アプリを一貫した 1 つの IaC で扱える
  - ARM64/Graviton 向けイメージのビルド強制 + ECR への push、web の `dist` ビルド + S3 配置が手順化され、「amd64 で誤ビルドして ECS タスクが起動しない」フットガン(ISSUE-014)が塞がる
  - SPEC-001 のサンプルグレード方針(HTTP・デフォルトドメイン・低コスト)と一貫した拡張になり、既存モジュールの資産を活かせる
- **価値の検証方法**: 以下がすべて満たされたら成功とみなす。
  1. `envs/dev` で `terraform fmt -check` / `terraform validate` / `tflint --recursive` / `trivy config .` が全て通る
  2. (AWS 認証情報がある環境で)`terraform plan` が **api・auth の 2 ECS サービス**と **web の S3 + CloudFront 配信**を含む差分をエラーなく出力する
  3. 単一 CloudFront ディストリビューションで `default → web(S3)` / `/api/* → api` / `/auth/* → auth` のルーティングが定義されている
  4. ARM64 イメージの build-push 手順(api / auth)と web の `dist` ビルド + S3 配置手順が Makefile/CI として提供され、ECS の `runtime_platform`(ARM64)と齟齬しない
  5. ISSUE-014 が指す「auth/web の AWS デプロイ経路が無い」乖離が解消したと確認できる

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- **インフラ利用者**として、`app/iac/envs/dev` で `terraform plan` を打つと api だけでなく auth と web の配信リソースも差分に出てほしい。なぜなら 3 アプリを 1 つの IaC で一貫して管理したいから。
- **デプロイ担当**として、`make` 系の 1 コマンドで ARM64 イメージ(api / auth)をビルドして ECR に push し、web の `dist` を S3 に配置したい。なぜならアーキ不一致や手動手順の抜けで起動失敗したくないから。
- **レビュアー**として、auth/web の追加リソースが既存モジュールの延長(ALB 共用・単一 CloudFront)として最小コストで定義されていることをコードで確認したい。なぜならサンプルのコスト方針(SPEC-001)を崩したくないから。

### 利用フロー

1. 開発者が api / auth の ARM64 イメージをビルドして ECR に push、web を `bun run build` して `dist` を得る(build-push 手順)
2. 開発者が `app/iac/envs/dev` で `terraform plan` を実行し、api/auth の ECS サービスと web の S3+CloudFront を確認する
3. 開発者(人間)が判断して `terraform apply` を実行する(agent は実行しない)
4. web の `dist` を S3 に配置(sync)し、CloudFront をインバリデートする
5. CloudFront のドメイン経由で web(SPA)にアクセスでき、`/api/*` は api、`/auth/*` は auth に到達する

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: `app/auth` を **ECS Fargate(ARM64)サービス**として既存 ALB 配下に追加する。専用 target group + ヘルスチェック(`GET /.well-known/openid-configuration` 200)、専用 ECR リポジトリを定義する
- [ ] R2: `app/web`(SPA)を **S3(非公開)+ CloudFront(OAC)** で静的配信する。SPA フォールバック(存在しないパスを `/index.html` に解決)を CloudFront 側で担保する
- [ ] R3: **単一 CloudFront ディストリビューション**で `default → web(S3 オリジン)` / `/api/* → ALB(api target group)` / `/auth/* → ALB(auth target group)` のパスベースルーティングを定義する(既存 `modules/cdn` / `modules/app` の拡張)
- [ ] R4: **ARM64/Graviton 向けイメージビルドの強制**と ECR への push 経路を提供する(api / auth を `docker buildx --platform linux/arm64` 等で必ず arm64 生成 → ECR)。web は `dist` ビルド + `aws s3 sync` + CloudFront invalidation を手順化する。実体は Makefile ターゲット(and/or `.github` の任意ジョブ)とし、`apply` と実配置は人間判断
- [ ] R5: auth の **OIDC issuer / discovery の絶対 URL 整合**を保つ。`/auth/*` パスプレフィックス配下で `ISSUER` と各エンドポイント URL が実アクセス経路と一致すること(解決方式は §4 / §リスクで planner が確定。app/auth の base-path 対応が要るなら cross-stack 課題として扱う)
- [ ] R6: 秘密情報の平文記載なし・NAT Gateway 不使用など SPEC-001 のコスト/セキュリティ方針を維持し、ALB は既存 1 本を共用(auth 用に別 ALB を増やさない)
- [ ] R7: `modules/` + `envs/dev` のレイアウトを維持し、環境差分は変数・tfvars のみで表現する。各モジュール README にコスト/設計理由(採用・不採用)を追記する

### 非機能要件

- **サンプルグレード維持**: SPEC-001 と同じく HTTP・CloudFront/ALB のデフォルトドメイン・カスタムドメイン/ACM なしを前提とする。auth の issuer もサンプル用途の HTTP とする(本番 OIDC 用の HTTPS issuer + ドメインはスコープ外・将来対応)
- **コスト**: 既存 ALB / CloudFront を共用し、auth 用に増えるのは小さい Fargate タスク 1 + target group + ECR。web は S3 + 既存 CloudFront で追加固定費が小さい(3 つ目の常時稼働 ECS を増やさない)
- **`.claude/rules/iac.md` 準拠**: バージョン固定・`for_each`/変数駆動・type/description 必須・タグ付与・remote backend 前提
- **既存の非退行**: SPEC-001 の api デプロイ経路(CloudFront → WAF → ALB → ECS → RDS)を壊さない。web/auth の追加は既存 api 経路の上に重ねる

### スコープ外(やらないこと)

- `terraform apply` の実行・実際の image push / S3 配置(手順の提供まで。実行は人間判断)
- 本番グレードの HTTPS issuer / カスタムドメイン / Route53 / ACM 証明書(SPEC-001 のサンプル方針を踏襲し HTTP + デフォルトドメイン)
- CI からの自動 apply / 自動デプロイ(build-push は手動 Make/任意ジョブとして提供するが、apply は自動化しない)
- prod 環境の実体化(`envs/dev` のみ)
- app/auth / app/web の**機能追加**(R5 の issuer 整合のために app/auth の base-path 対応が必要になった場合の**最小限のコード対応は本 Spec の依存**として扱うが、それ以外の業務機能は対象外)
- RDS への app/api 実接続(ISSUE-001 の領域。本 Spec は auth/web の配信経路に集中する)

## 4. 設計(どう実現するか)

### 方針

**SPEC-001 のサンプルグレード設計(HTTP・デフォルトドメイン・低コスト・NAT なし)を崩さず、既存モジュール(`network` / `cdn` / `app` / `db`)を拡張して auth/web を「重ねる」。** 新規に ALB や CloudFront を増やさず、共用インフラにサービスとオリジンを足すことで追加固定費を最小化する。

### アーキテクチャ / データ / インターフェース

```
Internet → CloudFront(単一ディストリビューション, + WAFv2)
   ├─ default (/*)     → S3(web / SPA, OAC・非公開)   ← 新規(web 配信)
   ├─ /api/*           → ALB → api  ECS Fargate(ARM64) → RDS   ← 既存(SPEC-001)
   └─ /auth/*          → ALB → auth ECS Fargate(ARM64)         ← 新規(auth 配信)
                          (ALB listener rule でパスごとに target group を振り分け)
build-push:
   api/auth: docker buildx --platform linux/arm64 → ECR(リポジトリはアプリごと)
   web:      bun run build → dist/ → aws s3 sync → CloudFront invalidation
```

- **auth(`modules/app` の一般化 or 複数呼び出し)**: 既存 `modules/app` は api 専用に固定されている。サービス名 / コンテナポート / ヘルスチェックパス / イメージ / ALB リスナールール(パス)を変数化して **api と auth の 2 サービス**を表現できるようにする(汎用化して 2 回呼ぶ、あるいは変数駆動で 2 サービス定義)。正確なリファクタ形は planner が決める。auth の health check は `/.well-known/openid-configuration`。
- **web(`modules/cdn` の拡張、または `modules/web` 新設)**: S3(非公開)+ CloudFront OAC。CloudFront の default behavior を S3 オリジンに、`/api/*` と `/auth/*` の behavior を ALB オリジンに向ける。SPA フォールバックは CloudFront の custom error response(403/404 → `/index.html`, 200)または CloudFront Function で実装(方式は planner)。**AWS では nginx は不要**(S3+CloudFront がリバースプロキシと静的配信を担う)。DOCKER-001 の nginx は**ローカル compose 専用**であり本 Spec とは別物、という二重性を README に明記する。
- **ECR**: アプリごとにリポジトリ(api / auth)。`for_each` で複数リポジトリを定義。
- **build-push**: Terraform は ECR リポジトリと S3 バケットの「箱」まで(SPEC-001 の「イメージのビルド・プッシュは対象外」を踏襲)。実ビルド/push/sync は Makefile ターゲット(`docker buildx --platform linux/arm64 ... --push`、`bun run build && aws s3 sync dist s3://...`、`aws cloudfront create-invalidation`)and/or `.github` の任意ジョブとして提供。ARM64 を明示指定して amd64 誤ビルドを防ぐ。
- **issuer 整合(R5)**: `/auth/*` パスプレフィックス配下では、`ISSUER` を `http://<cloudfront-domain>/auth` とし、discovery/authorize/token/userinfo/jwks の各 URL がその配下に載る必要がある。app/auth はルートに各エンドポイントを登録するため、(i) ALB/CloudFront でプレフィックスを剥がさず app/auth を **base-path 対応**にする、(ii) プレフィックスを剥がして ISSUER にはドメイン直下を使う、のいずれかを選ぶ。ドメイン直下だと api と衝突するため、サンプルでは (i) を第一候補とし、app/auth への最小限の base-path 対応(env で mount prefix を受ける等)を本 Spec の依存とする。**確定は planner + spec-owner**(§リスク)。

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| auth/web にそれぞれ別 ALB を立てる | ALB は 1 本 約$16/月。共用 1 本 + リスナールールで振り分ければ追加固定費ゼロ。サンプルのコスト方針(SPEC-001)に反する |
| web を 3 つ目の ECS(nginx コンテナ)で配信 | 常時稼働 Fargate が 1 増える。SPA は S3+CloudFront が標準かつ安価で、nginx リバースプロキシ役は CloudFront の behavior が代替できる。DOCKER-001 の nginx はローカル専用に留める |
| カスタムドメイン + ACM + HTTPS issuer(本番相当) | SPEC-001 が明示的にスコープ外とした方針を踏襲。ドメイン所有が前提になりサンプルの再現性を下げる。本番 OIDC issuer 化は将来別対応 |
| host ベースルーティング(auth を別サブドメイン) | サブドメインにはカスタムドメインが要る(サンプル外)。パスベース `/auth/*` で単一ディストリビューション内に収める |
| Terraform(docker/null provider)でイメージ build/push まで内包 | SPEC-001 の「ビルド・プッシュは対象外(ECR の用意まで)」と整合させ、build-push は Make/CI の手順として分離。IaC は箱の定義に集中 |
| `modules/app` を api 専用のままコピーして auth 用モジュールを新設 | 定義の二重化はメンテコストと乖離を生む(`.claude/rules/iac.md` の変数駆動方針)。変数化して再利用する |

## 5. 実装計画

詳細は **`docs/plans/SPEC-004-plan.md`(planner 作成済み)が正**(方針・変更ファイル・手順・テスト戦略・リスクは同ファイル参照)。確定した要点:

- **R5 の結論 = strip 方式 / app/auth・app/api とも無改修 / T2.5 は発火しない**。CloudFront で `/api`・`/auth` の先頭プレフィックスを CloudFront Function で剥がし、コンテナはルート実装のまま。auth は `ISSUER=http://<cloudfront-domain>/auth` を注入し、discovery の各絶対 URL(`/auth/authorize` 等)は `/auth/*` behavior 経由で到達=issuer と実経路が一致する。DOCKER-001 のローカル nginx 契約(`/api` を剥がして api コンテナのルートへ)と同型。ALB は strip 後にパスで判別できないため **ヘッダ(`X-Target-Service: auth`)で api/auth の target group を振り分ける**。
- **モジュール分割** = 現 `modules/app` を `modules/platform`(共有 ALB/リスナー/ECS クラスタ)+ `modules/service`(汎用サービス。api/auth で 2 回呼ぶ)に分解。auth issuer が CloudFront ドメインに依存するため、ALB を ECS から切り離し `platform → cdn → service` の一方向 DAG にして循環を回避(単一モジュール+`for_each` 案は循環するため不採用)。
- **web** = `modules/cdn` を拡張し S3(非公開)+ OAC + 単一 CloudFront に 3 オリジン/3 behavior(default→S3 / `/api/*`→api / `/auth/*`→auth)。SPA フォールバックは **default behavior 限定の CloudFront Function**(distribution 全体の `custom_error_response` は API エラーを化けさせるため不採用)。
- **build-push** = root `Makefile`(`push-images`=`docker buildx --platform linux/arm64 --push`、`deploy-web`=`bun run build`→`aws s3 sync`→invalidation)+ 任意の `.github/workflows/deploy.yml`(`workflow_dispatch` のみ・自動 apply 無し)。

タスク(担当と依存は plan の「手順」が正):

- [x] T1: (planner) 設計確定(上記)。plan 作成・R5 確定。
- [x] T2: (impl-iac) `modules/platform` 抽出 → `modules/service` 汎用実装 → api 再配線(+`moved` で非退行)→ `modules/cdn` 拡張 → auth 追加 + behavior 切替。README 更新。(commit `3afdf9f`)
- [x] T3: (impl-ci) root `Makefile` build-push 追加 + `.github/workflows/deploy.yml`(workflow_dispatch)。(commit `9e19531`)
- [x] ~~T2.5: (impl-api) app/auth base-path 対応~~ → **不発火**(R5 の strip 方式で無改修。実コードで無改修を確認)。
- [x] T4: (checker) `fmt-check` / `validate` green。tflint / trivy は環境未導入(レビューで手動補完)。
- [x] T5: (review-security / review-performance / review-spec) レビュー実施。security / performance はクリーン、spec は Major 1 件を検出。
- [x] T6: Major(ForceNew 名ドリフトで `moved` が実 replace)を impl-iac が override 変数で修正(commit `6b7b0ac`)→ review-spec 再検証で解消確認。ISSUE-014 を resolved・ISSUE-002 に本番移行項目を追記。

> 注: T2(iac)と T3(build-push)は並列可。T2.5 は不発火。`terraform apply` はスコープ外(plan まで)。

## 6. 経緯(時系列・追記のみ)

### 2026-07-09

- 初版作成。プロジェクト全体レビューで特定した **ISSUE-014**(`app/iac` が SPEC-001 の api 単体世代のまま、DOCKER-001 でコンテナ化された auth/web の AWS デプロイ経路が無い乖離)を起点に、auth/web を AWS に展開する経路を設計するもの。
- 推奨デフォルトとして以下をユーザー提示前提で置いた(approved 時に確定): **(1) サンプルグレード維持**(SPEC-001 と同じく HTTP・デフォルトドメイン・ACM/カスタムドメインなし)/ **(2) web = S3 + CloudFront(OAC)静的配信**で既存 `modules/cdn` を拡張(nginx はローカル compose 専用に留める)/ **(3) auth = 2 つ目の ECS Fargate(ARM64)サービス**を既存 ALB 配下にパスベースルーティング(`/auth/*`)で追加。別 ALB / 3 つ目の常時稼働 ECS / カスタムドメインは、いずれもコスト・サンプル再現性の観点で不採用(§4 代替案表)。
- **未確定(要 planner + spec-owner 確定)**: R5 の auth issuer/base-path 整合。`/auth/*` プレフィックス配下で `ISSUER` と各 OIDC エンドポイント URL を一致させるには app/auth の base-path 対応(cross-stack の最小コード変更)が要る可能性がある。ここを確定するまで T2.5 の要否が決まらない。
- status は `draft`。ユーザー承認(approved)後に planner へ実装計画作成を委譲し、着手する(機能開発は Spec を approved にしてから着手、の原則に従う)。
- ISSUE-014 と相互リンク(frontmatter `issues: [ISSUE-014]`)。ISSUE-014 側の対応方針は本 Spec の設計・approved 判断と同期させる。
- ユーザー承認を得て status を `draft` → **`approved`** に更新した。推奨デフォルト(サンプルグレード維持 / web = S3+CloudFront / auth = 既存 ALB 共用のパスベースルーティング)で確定。planner に実装計画(`docs/plans/SPEC-004-plan.md`)の作成を委譲する。**R5(auth issuer/base-path)は planner が精査**し、`/auth/*` プレフィックス整合のための app/auth 最小変更の要否(T2.5 の発火)を確定する。
- planner が実装計画 `docs/plans/SPEC-004-plan.md` を作成。着手に伴い status を `approved` → **`in-progress`** に更新(updated: 2026-07-09)。§5 を計画の確定内容に更新した。確定した主要判断:
  - **R5 = strip 方式に確定。app/auth・app/api とも無改修で、T2.5(impl-api の base-path 対応)は発火しない。** 調査事実: app/auth・app/api はともにルート直下実装(`route/router.go`)で、discovery は `issuer` 文字列連結で絶対 URL を作る(`service/discovery_service.go`)。DOCKER-001 のローカル nginx が既に `/api` を剥がして api コンテナのルートへ渡す契約。よって AWS でも CloudFront Function で `/api`・`/auth` を剥がし、auth は `ISSUER=http://<cloudfront-domain>/auth` を注入すれば、discovery の各 URL(`/auth/authorize` 等)は `/auth/*` behavior 経由で到達し issuer と実経路が一致する。Spec §4 が案 ii(剥がす)を退けた前提は「issuer をドメイン直下にする」場合であり、本計画は「issuer に `/auth` を残しコンテナには剥がして渡す」に精緻化して衝突を回避した。strip 後は ALB がパスで判別できないため、ヘッダ(`X-Target-Service: auth`)で target group を振り分ける。
  - **モジュール分割**: 現 `modules/app` を `modules/platform`(共有 ALB/リスナー/ECS クラスタ)+ `modules/service`(汎用サービス、api/auth で 2 回呼ぶ)に分解。auth の issuer が CloudFront ドメイン(`module.cdn` 出力)に依存するため、単一モジュール + `for_each` 案は `module.app ↔ module.cdn` の循環を生む。ALB を ECS から切り離し `platform → cdn → service` の一方向 DAG にして単一 apply で解決する。api リソースのアドレス移動には `moved` ブロックを網羅して非退行を担保する。
  - **web / SPA フォールバック**: `modules/cdn` を拡張し S3(非公開)+ OAC + 単一 CloudFront に 3 オリジン/3 behavior。SPA フォールバックは default behavior 限定の CloudFront Function(distribution 全体 `custom_error_response` は API/auth の正当な 404/403 を index.html に化けさせるため不採用)。
  - **build-push**: root `Makefile` に `push-images`(`docker buildx --platform linux/arm64 --push`)/ `deploy-web`(`bun run build`→`aws s3 sync`→invalidation)、任意で `.github/workflows/deploy.yml`(`workflow_dispatch` のみ・自動 apply 無し)。impl-ci 担当。impl-iac(T2)と並列可。
  - **auth の運用制約**: app/auth は起動毎に RSA 鍵を生成しトークンが発行インスタンス限定のため、auth サービスは `desired_count=1` 既定とする(既知の app/auth 性質。マルチタスク化・鍵外部化はサンプル範囲外)。
  - **残余**: web を OIDC RP として auth に実接続する配線(redirect_uri 登録等)は本 Spec スコープ外。`terraform apply` はスコープ外(plan/静的検証まで)。
- 実装・レビューを完了した(status は `in-progress` を維持。理由は末尾)。
  - **実装**: impl-iac が `app/iac` を再構成・拡張(`modules/platform` 抽出 / `modules/service` 汎用化(api・auth の 2 呼び出し)/ `modules/cdn` に S3+OAC+3 オリジン/behavior+CloudFront Function 2 本 / `envs/dev` 配線 + `moved.tf` 網羅、commit `3afdf9f`)。impl-ci が build-push ツーリング(root `Makefile` の `push-images`(`docker buildx --platform linux/arm64`)/ `deploy-web`、`.github/workflows/deploy.yml`(workflow_dispatch のみ)、commit `9e19531`)。R5 の結論どおり **app/auth・app/api は無改修**(実コードで確認)。
  - **checker**: `terraform fmt -check -recursive` / `terraform validate`(`init -backend=false`)が green。**tflint / trivy は本環境に未導入**のため未実行(セキュリティ観点はレビューで手動補完)。
  - **レビュー(3 並列)**: review-security = Blocker/Major 0(二層防御(prefix-list SG + `X-Origin-Verify`)・S3 非公開+OAC(SourceArn 限定)・WAF 継続・秘密平文なし・IAM 最小権限を api/auth 両経路で確認)。review-performance = Blocker/Major/Minor 0(追加固定費は auth の最小 Fargate 1 本に収まり ALB/CloudFront 共用・NAT 不使用を維持)。review-spec = R1〜R7・スコープ外遵守・R5(app/auth 無改修)を満たすが、**Major 1 件**を検出。
  - **Major の対応**: `modules/service` 汎用化で api の 4 ForceNew リソース名(TG / task_execution role / task role / secrets inline policy)がずれ、`moved` を張っても実 `plan` では replace になる欠陥。impl-iac が per-resource の name override 変数を追加し api インスタンスに SPEC-001 の旧名を厳密復元(auth は別名維持)、commit `6b7b0ac`。review-spec の再検証で **解消を確認**(旧名と文字列一致、fmt/validate green)。
  - **トリアージ**: 残る Minor/Info(strip_prefix のパストラバーサル apply 後検証 / deploy.yml の OIDC 信頼ポリシー範囲 / GitHub Environment 承認ゲート / S3・CloudFront アクセスログ・バージョニング未設定 / CloudWatch Logs KMS / CloudFront↔ALB HTTP / auth の desired_count=1+Spot=SPOF)は、いずれも「退行ではなくサンプル省略 / apply 後検証」として **ISSUE-002(本番移行チェックリスト)に追記**。**ISSUE-014 は `resolved`** に更新(IaC コードレベルで auth/web デプロイ経路が実装・レビュー済み)。
  - **`done` にしない理由**: 「価値の検証方法」のうち tflint/trivy(環境未導入)と `terraform plan`(AWS 認証情報が必要)は本環境で実行できず未達。SPEC-001 と同様、認証情報のある環境での `terraform plan`(api が move/no-op で replace されないこと、auth/web リソースが新規に出ること)と apply、tflint/trivy 実行は**ユーザーの手動確認**として残す。それらが満たされた時点で `done` に更新する。
