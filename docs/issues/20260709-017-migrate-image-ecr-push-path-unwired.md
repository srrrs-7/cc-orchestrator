---
id: ISSUE-017
title: SPEC-005 の prod マイグレーション用イメージ(リファクタで共有 app/migrator 単一イメージに集約)を ECR に build & push する経路が未配線で、init コンテナがイメージを pull できず terraform apply が成立しない
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-12
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-017: SPEC-005 の prod マイグレーション用イメージ(リファクタで共有 app/migrator 単一イメージに集約)を ECR に build & push する経路が未配線で、init コンテナがイメージを pull できず terraform apply が成立しない

> **注(2026-07-10)**: SPEC-005 リファクタリング(別データベース + `app/migrator` 集約)により、prod マイグレーション用イメージは per-stack `:migrate` 2 本(旧 `app/{api,auth}/Dockerfile.migrate`、削除済み)から**共有 `app/migrator` 単一イメージ**に変わった。**push 経路が未配線という本 Issue の本質は不変で open のまま。** §4 対応方針・実施内容は単一イメージ前提へ更新済み。§2 現象・§3 原因は起票時(per-stack `Dockerfile.migrate` / `:migrate` 前提)の記録として残す。差分は §5 経緯 2026-07-10 を参照。

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

> **更新(2026-07-10 / SPEC-005 リファクタ後)**: prod マイグレーション用イメージは per-stack `:migrate` 2 本(旧 `app/{api,auth}/Dockerfile.migrate`、削除済み)から**共有 `app/migrator` 単一イメージ**(`-target api|auth` でランタイム分岐)に集約された。以下は単一イメージ前提に更新した対応方針。旧構成の記述は §5 経緯 2026-07-10 を参照。

- **共有 `app/migrator` イメージ 1 本を ARM64 で build して migrator 用 ECR リポジトリに push する経路を追加する**ことを、`terraform apply`(api/auth の新規デプロイ / 更新)の前提条件として明記する。SPEC-005 本体のスコープ(plan まで / 永続化実装 / runtime 依存は pgx のみ)を超える後続作業として追跡する。
- ビルドは **リポジトリルートをコンテキストに `-f app/migrator/Dockerfile`**(この Dockerfile は `app/api/db/migrations` と `app/auth/db/migrations` の両方を焼き込むため、コンテキストが `app/migrator` ではなくリポジトリルートである必要がある。`app/migrator/Dockerfile:11-15`)。push 先はサービスごとではなく**共有 migrator ECR リポジトリ 1 本**(`app/iac/envs/dev/migrator.tf` の `aws_ecr_repository.migrator`、URL は `terraform output migrator_ecr_repository_url`。`app/iac/envs/dev/outputs.tf:36-38`)。タグは iac が参照する `:latest`(`app/iac/envs/dev/main.tf:187,276` の `migration_image = "${aws_ecr_repository.migrator.repository_url}:latest"`)。
- 実装候補(いずれか、または併用):
  - **ルート `Makefile` に migrator イメージ用ターゲットを追加**(既存 `push-images` と同型)。`docker buildx build --platform linux/arm64 -f app/migrator/Dockerfile --push -t "$$migrator_repo:latest" .` を、`terraform output migrator_ecr_repository_url` から得た URL に対して実行する。既存 `push-images` と同様に「apply 済み + AWS 認証情報が前提、agent は実行しない・手動実行前提、ARM64 明示は ISSUE-014 と同じ理由」の注意書きを付す(担当: impl-db または impl-ci)。**1 イメージで api/auth 双方をカバーするため、旧構成のような 2 本 build は不要**。
  - **CI/CD(`.github/`)への組み込み**(担当: impl-ci)。アプリイメージの push と同じトリガで migrator イメージも build/push する。
- **前提条件としての明記**: `app/iac/modules/service/README.md`「イメージ・並行実行・代替案」節 / `app/iac/envs/dev/migrator.tf` のコメント / `app/iac/envs/dev` の README に、「apply 前に共有 migrator イメージ(`:latest`)を push すること」を運用手順として残す(現状はコメントに留まる)。
- 参照: `app/iac/envs/dev/migrator.tf`(共有 migrator ECR リポジトリと未 push の明記)、`app/iac/envs/dev/main.tf:187,276`(`migration_image` の `:latest` 参照)、`app/iac/modules/service/ecs.tf`(`dependsOn: SUCCESS`)、`Makefile:101-108`(アプリイメージのみ push する既存 `push-images`。migrator には触れない)、`docs/plans/SPEC-005-plan.md` §RF。

### 実施内容

> **更新(2026-07-10)**: 単一 `app/migrator` イメージ前提に置き換え。

- [x] 共有 `app/migrator` イメージを ARM64 で build & ECR push する手段を追加する(ルート `Makefile` ターゲット追加 or CI/CD ジョブ)。ECR リポジトリ URL は `terraform output migrator_ecr_repository_url`(`app/iac/envs/dev/outputs.tf:36-38` の契約)から取得する
- [x] build コンテキスト(=リポジトリルート)・`-f app/migrator/Dockerfile` 指定・タグ(`:latest`)が iac の `migration_image` 参照(`app/iac/envs/dev/main.tf:187,276`)と一致することを確認する
- [x] apply の前提条件(共有 migrator イメージ `:latest` を先に push)を iac の README / `migrator.tf` コメントの運用手順として明記する
- [x] 1 イメージで api・auth 双方をカバーする(migrator ECR リポジトリは共有 1 本。旧: サービスごと 2 本という項目は不要になった)
- [x] `push-images` と migrator push の実行順序 / 依存(両方を 1 コマンドで回すか、別ターゲットにするか)を決める → 別ターゲット `push-migrator-image` として独立させ、`push-images`(api/auth アプリイメージ)と対称的に管理する

### 再発防止

- iac のタスク定義が参照するイメージタグ(`:latest` / `:migrate` 等)には、必ず対応する build & push 経路(Makefile ターゲット or CI ジョブ)をセットで用意する、を SPEC-005 系のデプロイ設計チェックに加えることを検討する。「iac がイメージ URI を参照するのに供給経路が無い」状態を plan 段階で洗い出す。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-005(app/api・app/auth の Postgres 永続化基盤)のレビュー(review-spec、E3)で挙がった **R-h**(migrate イメージの ECR push 経路が未配線)を記録。SPEC-005 と相互リンク(frontmatter `specs: [SPEC-005]`、Spec 側 `issues` に ISSUE-017 を追記)。
- 事実確認: iac の migrate init コンテナは `:migrate` イメージを既定参照する(`app/iac/modules/service/main.tf:35`)が、それを ECR に build & push する経路が存在しない — ルート `Makefile` の `push-images`(`Makefile:84-91`)はアプリイメージ(`:$(IMAGE_TAG)`)のみを push し、`Dockerfile.migrate` / `:migrate` に触れない。`.github/` にも当該ジョブは無い。`app/iac/envs/dev/main.tf:166-168` のコメントが「`:migrate` は現時点でどこにも push されていない」と明記。init コンテナの `dependsOn: SUCCESS`(`app/iac/modules/service/ecs.tf:19-31`)により、`:migrate` 不在では migrate タスクが pull に失敗しアプリコンテナが起動しない。
- スコープ確認: `docs/plans/SPEC-005-plan.md` §6.2 R-h が「実運用の push 経路(ルート Makefile 拡張)は本 Spec では plan 記述に留め、実配線は後続に委ねる」と意図的にスコープ外化した項目であることを確認。既存の SPEC-005 起票分(ISSUE-005 平文パスワード / ISSUE-015 authcode 無制限増加 / ISSUE-016 DB 最小権限・TLS)、および ISSUE-014(app `:latest` イメージ + web デプロイ + ARM64、resolved)のいずれとも重複しない独立テーマ(= migrate 専用イメージ `:migrate` の配布経路)であることを確認した。
- severity は **medium** と判定。判定根拠: 実運用デプロイ(apply / サービス更新)の前提条件が欠けており放置すると新規デプロイ・スケールアウトが成立しない一方で、(1) ローカル / CI 経路には影響せず、(2) 既存の running タスクは維持され即時の可用性影響が無く、(3) 手動 `docker buildx build --platform linux/arm64 -f Dockerfile.migrate --push -t <repo>:migrate` という回避策が存在する。主要機能が「使えない」(critical)や主要価値が損なわれる(high)には至らず、回避策のある不具合(medium)に該当する。
- 次にやること: 後続で impl-db / impl-ci が `:migrate` の ARM64 build & ECR push 経路(ルート `Makefile` ターゲット or CI/CD ジョブ)を追加し、iac README に apply 前提として明記する。

### 2026-07-10

- SPEC-005 リファクタリング(2026-07-09〜、別データベース + `app/migrator` 集約。plan §RF)に伴い本 Issue の前提を更新した。**prod マイグレーション用イメージが、per-stack `:migrate` 2 本(旧 `app/{api,auth}/Dockerfile.migrate` + `db/migrate-entrypoint.sh`、いずれも削除済み)から、両スタックの `db/migrations` を焼き込んだ共有 `app/migrator` 単一イメージに集約された。** 実行時に `-target api|auth`(`app/migrator/main.go`)で対象 DB を選び、`CREATE DATABASE` 冪等化(`app/migrator/database.go` の `ensureDatabase`)+ goose 適用を行う。ビルドコンテキストは**リポジトリルート**(`-f app/migrator/Dockerfile`。`app/api/db/migrations` と `app/auth/db/migrations` の両方を COPY するため。`app/migrator/Dockerfile:11-15`)。
- iac 側は**共有 migrator ECR リポジトリ 1 本を追加済み**(`app/iac/envs/dev/migrator.tf` の `aws_ecr_repository.migrator`、出力 `migrator_ecr_repository_url`、`app/iac/envs/dev/outputs.tf:36-38`)。api/auth の migrate init コンテナはどちらもこの同じイメージ(`:latest`)を `migration_image` として参照し(`app/iac/envs/dev/main.tf:187,276`)、`migration_command` の `-target api` / `-target auth` だけで挙動を分ける。旧 `:migrate`(サービス自身の ECR)参照・per-stack 2 本 build は解消。
- **push 経路(CI/Makefile 拡張)は依然として未配線のため本 Issue は open のまま。** ルート `Makefile` の `push-images`(`Makefile:101-108`)は今も api/auth のアプリイメージ(`:$(IMAGE_TAG)`)のみを build & push し、`app/migrator` イメージには一切触れない。`.github/` にも当該ジョブは無い。`app/iac/envs/dev/migrator.tf` のコメントが「イメージが push されるまで両サービスの init コンテナが pull に失敗しロールアウトが詰まる(既存タスクは running のまま残る)」と明記している。
- これに合わせて §4 対応方針・実施内容を単一イメージ前提へ更新した(共有 migrator ECR 1 本への push・リポジトリルートコンテキスト + `-f app/migrator/Dockerfile`・タグ `:latest`)。§2 現象・§3 原因は起票時(per-stack `Dockerfile.migrate` / `:migrate` 前提)の記録として残す。frontmatter は status=open 維持・updated=2026-07-10・title を単一イメージ前提へ更新。SPEC-005 とのリンク(`specs: [SPEC-005]`、Spec 側 `issues`)は既存のまま。
- severity は **medium** を維持。判定根拠は 2026-07-09 エントリのとおりで、リファクタ後も変わらない: 実運用デプロイ(apply / サービス更新)の前提条件が欠けている一方で、(1) ローカル(`make migrate` は `app/migrator` を `go run`)/ CI 経路には影響せず、(2) 既存 running タスクは維持され即時の可用性影響が無く、(3) 手動 `docker buildx build --platform linux/arm64 -f app/migrator/Dockerfile --push -t <migrator_repo>:latest .` という回避策がある。回避策のある不具合(medium)に該当。

### 2026-07-12

- **対応完了、status を resolved に変更。** ルート `Makefile` に `push-migrator-image` ターゲットを追加した(既存 `push-images` と対称)。`terraform -chdir=$(TF_ENV_DIR) output -raw migrator_ecr_repository_url` で ECR URL を取得し、`docker buildx build --platform linux/arm64 --push -f app/migrator/Dockerfile -t "$$migrator_repo:latest" .` でリポジトリルートをコンテキストに ARM64 build & push する。タグは `:latest`(iac の `migration_image` 既定値と一致。`app/iac/envs/dev/main.tf` の api/auth 両 `migration_image`)。`push-images` とは別ターゲット(独立した手動実行前提)として対称的に管理する構成を採用した。
- `app/iac/envs/dev/README.md` の「apply 前の前提条件」を更新し、`make push-migrator-image` を実行してから apply することを明示した(ISSUE-017 参照付き)。
- `make -C app/iac check`(fmt-check + validate + lint + security)が exit 0 であることを確認。trivy の findings(migrator.tf の MUTABLE タグ / S3 logging 等)は既存のもので今回の変更による新規ではない。
