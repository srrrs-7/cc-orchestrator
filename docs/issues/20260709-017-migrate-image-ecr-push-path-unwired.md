---
id: ISSUE-017
title: SPEC-005 の migrate イメージ(app/{api,auth}/Dockerfile.migrate)を ECR に build & push する経路が未配線で、init コンテナが :migrate を pull できず terraform apply が成立しない
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-09
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-017: SPEC-005 の migrate イメージ(app/{api,auth}/Dockerfile.migrate)を ECR に build & push する経路が未配線で、init コンテナが :migrate を pull できず terraform apply が成立しない

## 1. ユーザー価値への影響(なぜ対応するか)

> **`app/iac` を apply して app/api・app/auth を AWS(ECS)へデプロイしようとする運用者** の **「apply すればスキーマ適用込みでアプリが起動する」という期待** が **prod マイグレーション用の `:migrate` イメージを ECR に配布する経路がどこにも存在せず、init コンテナがそのイメージを pull できないことで損なわれる**。

- **影響を受けるユーザー**: `app/iac/envs/dev`(および将来の prod)を apply して api/auth を新規デプロイ / 更新デプロイする運用者
- **損なわれる価値**: SPEC-005 の DB マイグレーション init コンテナ(`dependsOn: SUCCESS`)を含む ECS デプロイが、`:migrate` イメージ不在のため成立しない(migrate タスクがイメージ pull に失敗し、後続のアプリコンテナが起動しない)
- **影響範囲・頻度**: 特定条件下(`migration_environment` を渡すデプロイ、= dev envs のデフォルト構成で apply / サービス更新するとき)。**既存の running タスクは維持されるため即時の可用性影響は無い**(新規タスクの起動・ローリング更新が成立しない)
- **回避策**: あり(運用者が手動で `docker buildx build --platform linux/arm64 --push -t <repo>:migrate app/api`(および auth)を実行して ECR に push してから apply する)。恒久的な push ターゲット / CI 経路は未実装

## 2. 現象(何が起きているか)

### 期待する動作

`terraform apply` の前提として、`app/{api,auth}/Dockerfile.migrate` から build した `:migrate` イメージが対象サービスの ECR リポジトリに push されており、ECS の migrate init コンテナがそれを pull → `CREATE SCHEMA IF NOT EXISTS <schema>` + `goose up` を実行して `SUCCESS` で終了し、続いてアプリコンテナが起動する。この build & push を担う手段(ルート `Makefile` のターゲット、または CI/CD)が存在する。

### 実際の動作

`:migrate` タグのイメージを ECR に build & push する経路がどこにも無い。

- ルート `Makefile` の `push-images`(`Makefile:84-91`)は **api/auth のアプリイメージ(`app/api` / `app/auth` の `Dockerfile`)を ECR の `:$(IMAGE_TAG)`(既定 `latest`)に push するのみ**で、`Dockerfile.migrate` の `:migrate` イメージには一切触れない。
- CI/CD(`.github/`)にも `:migrate` を build/push するジョブは無い。
- 一方 iac 側は init コンテナのイメージ URI として `:migrate` を既定参照する(`app/iac/modules/service/main.tf:35` = `"${aws_ecr_repository.this.repository_url}:migrate"`)。envs/dev はこの既定を使い、コメントで「`:migrate` は現時点でどこにも push されていない(NOT yet pushed anywhere)」と明記している(`app/iac/envs/dev/main.tf:166-168`)。

結果、`migration_environment` を渡した状態(dev のデフォルト)で apply / サービス更新すると、ECS が起動する migrate init コンテナが `:migrate` イメージの pull に失敗し、`dependsOn: [{containerName: "<service>-migrate", condition: "SUCCESS"}]`(`app/iac/modules/service/ecs.tf:19-31`)によってアプリコンテナが起動しない。既存 running タスクは維持されるため即時の停止は起きないが、新規タスクの立ち上げ・ローリング更新が完了しない。

### 再現手順

1. `app/{api,auth}/Dockerfile.migrate` の `:migrate` イメージを **ECR に push していない**状態(現状のクリーンなリポジトリ)を用意する。
2. `app/iac/envs/dev` を、`migration_environment` が非空(現状のデフォルト構成)のまま `terraform apply` する(注: 本リポジトリの規約では agent は apply しない。第三者が手動実行する前提の手順)。
3. ECS サービスが新しいタスクを起動する際、`<service>-migrate` init コンテナが `<ecr_repo>:migrate` を pull しようとする。ECR に該当タグが無いため pull に失敗する。
4. `dependsOn: SUCCESS`(`app/iac/modules/service/ecs.tf`)により、migrate コンテナが SUCCESS で終了しない限りアプリコンテナが起動しない → 新規タスクがヘルシーにならず、デプロイ(ローリング更新)が完了しない。ECS のイベント / タスク停止理由に `CannotPullContainerError`(該当タグ不在)相当が記録される。
5. 対比: `push-images`(`Makefile:84-91`)を実行しても push されるのは `<repo>:latest`(アプリイメージ)のみで、`:migrate` は依然として不在のままである。

### 環境・条件

- 対象: AWS(ECS)へのデプロイ経路のみ。**ローカル(compose)と CI は影響を受けない** — ローカルのマイグレーションはルート `Makefile` の `migrate`(→ 各スタックの `migrate-up` = goose を `go run` で実行、`Makefile:62-65`)で行い `:migrate` イメージを使わない。CI は postgres service container を使う(SPEC-005 plan D2)。
- 前提: `app/{api,auth}/Dockerfile.migrate` は作成済み(E4 でスキーマ作成 + libpq 接続に修正済み)。init コンテナ定義・`migration_image` の既定参照(`:migrate`)も配線済み。欠けているのは **イメージを `:migrate` として ECR に配布する手段**のみ。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: iac の init コンテナは `migration_image`(既定 `"${aws_ecr_repository.this.repository_url}:migrate"`、`app/iac/modules/service/main.tf:30-35`)を参照し、`var.migration_environment` が非空のときにのみ使われる(`app/iac/modules/service/variables.tf:130` の description)。
- 事実: `app/iac/envs/dev/main.tf:157-175` は api サービスに `migration_environment` を渡しており(dev のデフォルトで init コンテナが有効)、同ファイル `:166-168` のコメントが「`var.migration_image` はこのサービス自身の ECR リポジトリの `:migrate` を既定参照するが、それはこの plan 時点でどこにも push されていない — apply 前に README の SPEC-005 節を参照せよ」と明記している。
- 事実: `app/{api,auth}/Dockerfile.migrate` が存在する(`app/api/Dockerfile.migrate` / `app/auth/Dockerfile.migrate`)。これらは goose CLI + `db/migrations` + entrypoint を焼き込む migrate 専用イメージのビルド定義。
- 事実: ルート `Makefile` の `push-images`(`Makefile:84-91`)は `docker buildx build --platform linux/arm64 --push -t "$$api_repo:$(IMAGE_TAG)" app/api` と同 `app/auth` の 2 本のみで、`Dockerfile.migrate` / `:migrate` を参照しない(`-f Dockerfile.migrate` も `:migrate` タグも登場しない)。
- 事実: `.github/` の workflow にも `:migrate` を build/push するステップは無い(`grep -rn migrate Makefile .github/` は `:migrate` の build/push を返さない)。
- 事実: これは意図的なスコープ外化。`docs/plans/SPEC-005-plan.md` §6.2 R-h が「`Dockerfile.migrate` の image をどの ECR に push しどう参照するか。apply しないため plan 段階では task 定義に image URI 参照を置くのみ。実運用の push 経路(ルート Makefile 拡張)は本 Spec では plan 記述に留め、実配線は後続に委ねる(過剰スコープ回避)」と記載している。`app/iac/envs/dev/main.tf:164-165` のコメントも「§6.2 R-g/R-h for ... the deferred image-push wiring」と後続委譲を明記。
- 仮説: 既存の running タスクがある状態では、新規デプロイが失敗してもローリング更新の仕組みにより旧タスクが維持され、即時のサービス停止には至らない(= 可用性影響は「新規デプロイ / スケールアウトが成立しない」に留まる)。ECS の circuit breaker / デプロイ設定次第で挙動は変わり得る(実測は未実施)。

### 根本原因

SPEC-005 は「plan まで(apply しない)」をスコープとし、prod マイグレーションの実行手段として init コンテナ(`:migrate` イメージ参照)を iac に配線するところまでを対象とした。migrate イメージを ECR に build & push する実配線は、新たなデプロイ / ツーリング要素を要し過剰スコープになるため plan §6.2 R-h で意図的に後続へ委ねた。その結果、**iac は `:migrate` を参照するのに、それを供給する build & push 経路(ルート `Makefile` ターゲット or CI/CD ジョブ)が存在しない**という配線の欠落が残っている。

## 4. 対応(どう解決するか)

### 対応方針

- **`:migrate` イメージを ARM64 で build して各サービスの ECR リポジトリに push する経路を追加する**ことを、`terraform apply`(api/auth の新規デプロイ / 更新)の前提条件として明記する。SPEC-005 本体のスコープ(plan まで / 永続化実装 / runtime 依存は pgx のみ)を超える後続作業として追跡する。
- 実装候補(いずれか、または併用):
  - **ルート `Makefile` に migrate イメージ用ターゲットを追加**(既存 `push-images` と同型)。`docker buildx build --platform linux/arm64 -f app/api/Dockerfile.migrate --push -t "$$api_repo:migrate" app/api`(および auth)を、`terraform output` から得た ECR リポジトリ URL に対して実行する。既存 `push-images` と同様に「apply 済み + AWS 認証情報が前提、agent は実行しない・手動実行前提、ARM64 明示は ISSUE-014 と同じ理由」の注意書きを付す(担当: impl-db または impl-ci)。
  - **CI/CD(`.github/`)への組み込み**(担当: impl-ci)。アプリイメージの push と同じトリガで `:migrate` も build/push する。
- **前提条件としての明記**: `app/iac/modules/service/README.md`「マイグレーション init コンテナ」節 / `app/iac/envs/dev` の README(SPEC-005 節)に、「apply 前に `:migrate` を push すること」を運用手順として残す(現状はコメントに留まる)。
- 参照: `app/iac/modules/service/main.tf:35`(`:migrate` 既定参照)、`app/iac/envs/dev/main.tf:166-168`(未 push の明記)、`app/iac/modules/service/ecs.tf:19-31`(`dependsOn: SUCCESS`)、`Makefile:84-91`(アプリイメージのみ push する既存 `push-images`)、`docs/plans/SPEC-005-plan.md` §6.2 R-h。

### 実施内容

- [ ] `:migrate` イメージを ARM64 で build & ECR push する手段を追加する(ルート `Makefile` ターゲット追加 or CI/CD ジョブ)。ECR リポジトリ URL は `terraform output`(`app/iac/envs/dev/outputs.tf` の契約)から取得する
- [ ] `Dockerfile.migrate` の build コンテキスト・`-f` 指定・タグ(`:migrate`)が iac の `migration_image` 既定(`app/iac/modules/service/main.tf:35`)と一致することを確認する
- [ ] apply の前提条件(`:migrate` を先に push)を iac の README(`modules/service` / `envs/dev`)に運用手順として明記する
- [ ] api・auth の 2 サービス分をカバーする(それぞれ別 ECR リポジトリ)
- [ ] `push-images` と `:migrate` push の実行順序 / 依存(両方を 1 コマンドで回すか、別ターゲットにするか)を決める

### 再発防止

- iac のタスク定義が参照するイメージタグ(`:latest` / `:migrate` 等)には、必ず対応する build & push 経路(Makefile ターゲット or CI ジョブ)をセットで用意する、を SPEC-005 系のデプロイ設計チェックに加えることを検討する。「iac がイメージ URI を参照するのに供給経路が無い」状態を plan 段階で洗い出す。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-005(app/api・app/auth の Postgres 永続化基盤)のレビュー(review-spec、E3)で挙がった **R-h**(migrate イメージの ECR push 経路が未配線)を記録。SPEC-005 と相互リンク(frontmatter `specs: [SPEC-005]`、Spec 側 `issues` に ISSUE-017 を追記)。
- 事実確認: iac の migrate init コンテナは `:migrate` イメージを既定参照する(`app/iac/modules/service/main.tf:35`)が、それを ECR に build & push する経路が存在しない — ルート `Makefile` の `push-images`(`Makefile:84-91`)はアプリイメージ(`:$(IMAGE_TAG)`)のみを push し、`Dockerfile.migrate` / `:migrate` に触れない。`.github/` にも当該ジョブは無い。`app/iac/envs/dev/main.tf:166-168` のコメントが「`:migrate` は現時点でどこにも push されていない」と明記。init コンテナの `dependsOn: SUCCESS`(`app/iac/modules/service/ecs.tf:19-31`)により、`:migrate` 不在では migrate タスクが pull に失敗しアプリコンテナが起動しない。
- スコープ確認: `docs/plans/SPEC-005-plan.md` §6.2 R-h が「実運用の push 経路(ルート Makefile 拡張)は本 Spec では plan 記述に留め、実配線は後続に委ねる」と意図的にスコープ外化した項目であることを確認。既存の SPEC-005 起票分(ISSUE-005 平文パスワード / ISSUE-015 authcode 無制限増加 / ISSUE-016 DB 最小権限・TLS)、および ISSUE-014(app `:latest` イメージ + web デプロイ + ARM64、resolved)のいずれとも重複しない独立テーマ(= migrate 専用イメージ `:migrate` の配布経路)であることを確認した。
- severity は **medium** と判定。判定根拠: 実運用デプロイ(apply / サービス更新)の前提条件が欠けており放置すると新規デプロイ・スケールアウトが成立しない一方で、(1) ローカル / CI 経路には影響せず、(2) 既存の running タスクは維持され即時の可用性影響が無く、(3) 手動 `docker buildx build --platform linux/arm64 -f Dockerfile.migrate --push -t <repo>:migrate` という回避策が存在する。主要機能が「使えない」(critical)や主要価値が損なわれる(high)には至らず、回避策のある不具合(medium)に該当する。
- 次にやること: 後続で impl-db / impl-ci が `:migrate` の ARM64 build & ECR push 経路(ルート `Makefile` ターゲット or CI/CD ジョブ)を追加し、iac README に apply 前提として明記する。
