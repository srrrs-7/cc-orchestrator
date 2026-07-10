---
id: ISSUE-016
title: SPEC-005 の DB 接続が最小権限でない(api/auth 共有マスターユーザ)/ アプリ側 DB_SSLMODE 既定が "disable" で平文接続へ fail-open する
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-10
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-016: SPEC-005 の DB 接続が最小権限でない(api/auth 共有マスターユーザ)/ アプリ側 DB_SSLMODE 既定が "disable" で平文接続へ fail-open する

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api・app/auth を本番運用する運用者と、そのデータ(特に auth の認可コード・ユーザー資格情報)の機密性に依存するエンドユーザー** の **DB のスキーマ間権限境界と DB 接続の転送時機密性** が **(1) api/auth が同一 RDS マスターユーザで接続するため片方の接続情報流出で他方のスキーマも読み書き可能、(2) アプリ側 `DB_SSLMODE` 既定が `"disable"` のため注入漏れ時に静かに平文接続へ後退する、という 2 点で損なわれ得る**。

- **影響を受けるユーザー**: app/api・app/auth を本番運用する運用者と、auth の認可コード・ユーザーレコードの機密性に依存するエンドユーザー
- **損なわれる価値(条件下)**: (1) スキーマ間の権限境界 — api の接続情報が漏れると auth スキーマも読み書き可能(逆も同様)。(2) DB 資格情報・データの転送時機密性 — `DB_SSLMODE` 注入漏れ時に静かに平文接続へ後退する
- **影響範囲・頻度**: いずれも**現時点で実害なし**。(1) は「接続情報流出」という前提条件が必要な defense-in-depth ギャップ。(2) は iac が現在 `"require"` を明示注入しているため現状は暗号化されており、将来の注入漏れ(新環境・設定ミス)でのみ顕在化する fail-open
- **回避策**: あり((1) は現状 Spec スコープ = 名前空間分離として意図どおり。(2) は iac が `"require"` を注入している限り発生しない)。恒久的なハードニング(スキーマ毎の専用ロール / fail-closed な sslmode)は未実装

## 2. 現象(何が起きているか)

### 期待する動作

- **(R-c / 最小権限)** api と auth はそれぞれ自スキーマにのみ権限を持つ専用 DB ロールで接続し、片方の接続情報流出が他方のスキーマに波及しない(真の最小権限)。
- **(m-2 / fail-closed sslmode)** 本番相当環境で `DB_SSLMODE` が未設定なら、平文接続に静かに後退せず起動を失敗させる(fail-closed)。app/auth のセキュリティ思想(接続情報が無ければ起動失敗 = fail-closed。`db.go` の `SelectMode`)を sslmode にも適用する。

### 実際の動作

- **(R-c)** api・auth はいずれも同一 RDS のマスターユーザ(`module.db.master_user_secret_arn`)で接続する。両サービスの `DB_USER` / `DB_PASSWORD` が同一シークレットを参照する(`app/iac/envs/dev/main.tf:151-152` api / `:242-243` auth ほか)。`DB_SCHEMA` / `search_path` による分離は**名前空間の分離であって権限の分離ではない**(`app/iac/modules/db/README.md:52-54` が明記)。api の接続情報で auth スキーマ(`authorization_codes` / `users` 等)を読み書きできる(逆も同様)。
- **(m-2)** アプリ側 `app/auth/infra/postgres/db.go` の `DB_SSLMODE` 既定は `"disable"`(`:46` `defaultSSLMode`、`:117` `envOrDefault` で未設定時にフォールバック)。iac は本番で `"require"` を明示注入する(`app/iac/envs/dev/variables.tf:113-115` `db_sslmode` 既定 `"require"` を `DB_SSLMODE` として注入)ため現状は暗号化されるが、**注入漏れ時は静かに `sslmode=disable` の平文接続へ後退する**。

### 再現手順

**(R-c) 共有マスターユーザ:**

1. `app/iac/envs/dev/main.tf` を開き、`module.service_api` と `module.service_auth` の `DB_USER` / `DB_PASSWORD` がいずれも `${module.db.master_user_secret_arn}:username::` / `:password::`(同一シークレット = 同一マスターユーザ)であることを確認する(`:151-152`, `:178-179`, `:242-243`, `:263-264`)。
2. `app/iac/modules/db/README.md:52-54` で「同じ RDS マスターユーザで接続する」「`search_path` による分離は名前空間の分離であって権限の分離ではない」と明記されていることを確認する。
3. api の DB 資格情報で、`search_path` を変えれば auth スキーマのテーブルに SQL を発行できることを確認する(同一ユーザ・同一 database)。

**(m-2) DB_SSLMODE の fail-open:**

1. `app/auth/infra/postgres/db.go:46` の `defaultSSLMode = "disable"`、`:117` で `DB_SSLMODE` 未設定時にこの既定が使われることを確認する。
2. `DB_HOST` 等は設定しつつ `DB_SSLMODE` を未設定にして起動すると、DSN の `sslmode` が `disable`(平文)で組み立てられることを確認する(`:156-169` `DSN`)。
3. 一方、起動可否判定(`:71-81` `SelectMode`)は `DB_HOST` の有無で fail-closed するが、sslmode にはこの fail-closed 思想が適用されていないことを確認する(必須変数チェック `:121-136` も `DB_HOST` / `DB_NAME` / `DB_USER` / `DB_PASSWORD` のみで、`DB_SSLMODE` は未設定をエラーにしない)。

### 環境・条件

- 対象: app/auth・app/api の Postgres 永続化(SPEC-005)の DB 接続。app/iac の接続配線(`envs/dev`、`modules/db`、`modules/service`)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実(R-c): api・auth とも接続資格情報は `module.db.master_user_secret_arn`(同一マスターユーザ)。分離は `DB_SCHEMA="api"/"auth"`(`app/iac/envs/dev/main.tf:141,236` ほか)+ 接続 `search_path` のみ。`app/iac/modules/db/README.md:52-54` と `app/iac/modules/service/README.md:148-158` が「名前空間分離であって権限分離ではない」と明記している。
- 事実(m-2): `app/auth/infra/postgres/db.go:44-48` の `defaultSSLMode = "disable"`、`:110-119` `ConfigFromEnv` が `DB_SSLMODE` 未設定時にこの既定へフォールバック。`DB_HOST` / `DB_NAME` / `DB_USER` / `DB_PASSWORD` は未設定をエラーにする(`:121-136`)が、`DB_SSLMODE` はエラーにせず `disable` にフォールバックする。
- 事実: iac は `db_sslmode` 既定 `"require"`(`app/iac/envs/dev/variables.tf:113-115`)を `DB_SSLMODE` として両サービスに注入するため、現在デプロイされる本番は暗号化接続。fail-open は「注入を忘れた / 新環境で設定しなかった」場合にのみ発生する。
- 事実: いずれも SPEC-005 のスコープ(R3 = 別スキーマ・`search_path` 分離、R6 = 接続情報注入)は満たしている。真の最小権限ロール分離・sslmode の fail-closed は Spec のスコープ外。
- 仮説: なし(両点とも Spec で意図的に現状踏襲と評価済み。SPEC-005 plan §6.1 R-c / m-2)。

### 根本原因

- **(R-c)** SPEC-005 はバウンデッドコンテキストの分離を「同一 database の別スキーマ + `search_path`」で実現する設計を採り、権限境界(スキーマ毎の専用ロール)までは範囲に含めなかった。RDS マスターユーザ共有は `modules/db` の `manage_master_user_password` をサンプルとしてそのまま流用した結果。
- **(m-2)** アプリ側の `DB_SSLMODE` 既定を、ローカル開発(compose の postgres、TLS なし)で動くよう `"disable"` にした。fail-closed 思想(`DB_HOST` 無ければ起動失敗)は「接続有無」には適用されたが、「転送暗号化(sslmode)」には適用されていない。

## 4. 対応(どう解決するか)

### 対応方針

- 起票当初は「今回のスコープ(SPEC-005)では対応しない」ハードニング追跡項目だったが、**2 項目((m-2)fail-open sslmode / (R-c)最小権限 DB ユーザー)とも修正済み**。(m-2) は 2026-07-10 に既定 `disable`→`require` の fail-closed 化で解消、(R-c) は 2026-07-10 に random_password + Secrets Manager 方式(ユーザー選択)で最小権限ロール分離を実装し解消した(詳細は §5 経緯)。
- 参照: `app/iac/envs/dev/main.tf`(アプリコンテナは scoped secret、migrate init は master 継続)、`app/iac/modules/db`(`random_password` + api_app/auth_app 専用 Secrets Manager + outputs)、`app/migrator/domain/migration`(AppRole VO + RoleProvisioner ポート)/ `app/migrator/infra/postgres/role.go`(冪等な最小権限付与)、SPEC-005 plan §6.1(review-security E3)。

### 実施内容

- [x] (最小権限 / R-c) auth スキーマ・api スキーマそれぞれに専用 DB ロールを作り、各ロールに自スキーマのみの権限を付与。api / auth のアプリに別々の資格情報(専用 Secrets Manager シークレット)を注入する(iac: `modules/db` / `envs/dev/main.tf`、impl-iac)。マイグレーション実行ロール(master 継続)と実行時ロール(scoped)を分離。ロール付与は goose up 後・up コマンド時のみ実行(`app/migrator` の `domain/migration` + `infra/postgres/role.go` + `service.Migrate`)
- [x] (fail-closed sslmode / m-2) 既定 `defaultSSLMode` を `"disable"`→`"require"` に変更し、平文は明示 opt-in のみに(`app/api/cmd/api/env.go` / `app/auth/cmd/authz/env.go` / `app/migrator/config.go` の 3 箇所を一括対応、impl-db)。※ 2026-07-10 の先行ラウンドで解消済み
- [x] 上記に伴う統合テスト(fail-closed 分岐、専用ロールでの権限境界)を追加(m-2 の既定値テスト 3 件、および `app/migrator/infra/postgres/role_integration_test.go`(`//go:build integration`)+ CI `migrator-integration` job(postgres service)、tester / impl-db)
- [x] レビュー(review-security)で権限境界(DML 可 / DDL 不可 / クロス DB CONNECT 不可・双方向 / master 接続維持)と転送暗号化が意図どおりであることを実 Postgres で確認(Blocker 0、Major 2 件是正済み)

### 再発防止

- 「`search_path` / スキーマ分離は名前空間分離であって権限境界ではない」「接続の fail-closed は接続有無だけでなく転送暗号化(sslmode)にも適用する」を `.claude/rules/db.md` / iac の設計チェックに明記することを検討する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-005(app/api・app/auth の Postgres 永続化)のレビュー(review-security、E3)で「今回は対応せず追跡する」と判断された 2 つのハードニング項目(R-c 共有マスターユーザ / m-2 `DB_SSLMODE` の fail-open 既定)を 1 Issue に統合して記録。
- 事実確認: (R-c) api・auth とも `module.db.master_user_secret_arn` の同一マスターユーザで接続(`app/iac/envs/dev/main.tf:151-152,178-179,242-243,263-264`)、分離は `DB_SCHEMA` + `search_path` のみで `app/iac/modules/db/README.md:52-54` が「権限分離ではない」と明記。(m-2) `app/auth/infra/postgres/db.go:46` `defaultSSLMode="disable"`、`:117` で未設定時にフォールバック。iac は `db_sslmode` 既定 `"require"`(`app/iac/envs/dev/variables.tf:113-115`)を注入するため現状の本番は暗号化。
- severity は **medium** と判定。判定根拠: いずれも現時点で実害なし(sslmode は iac が `require` を注入、マスターユーザ共有は接続情報流出という前提が必要)で回避策あり(= critical/high ではない)。ただし DB 資格情報の転送が単一の設定漏れで静かに平文へ後退し得る fail-open と、認証基盤における権限境界の欠如という、軽微(low)には収まらないセキュリティハードニングのため medium。両点とも Spec スコープ(R3 / R6)は満たしており意図的な現状踏襲。
- 次にやること: 将来 planner がスキーマ毎の最小権限ロールと sslmode の fail-closed 化を計画化し、impl-iac / impl-db が実装、tester / review-security を通す。

### 2026-07-10(env 集約リファクタ後: DB_SSLMODE fail-open 既定が 3 箇所に拡散していることをレビューで確認)

- プロジェクト全体レビューで、課題 (m-2) の `DB_SSLMODE` fail-open 既定(`"disable"`)が、env 集約リファクタ後は **app/auth だけでなく 3 箇所に拡散**していることを確認した。起票時(2026-07-09)は app/auth の `infra/postgres/db.go` のみを記録していたが、現行コードでは接続設定の env 解決が各スタックの `cmd` / `config` に移っており、以下の 3 箇所すべてが未設定時に `sslmode=disable`(平文)へフォールバックする:
  - `app/api/cmd/api/env.go`: `defaultSSLMode = "disable"`(`:16`)、`DBSSLMode: orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode)`(`:48`)、`SSLMode: e.DBSSLMode`(`:61`)。
  - `app/auth/cmd/authz/env.go`: `defaultSSLMode = "disable"`(`:18`)、`orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode)`(`:57`)、`SSLMode: e.DBSSLMode`(`:71`)。
  - `app/migrator/config.go`: `defaultSSLMode = "disable"`(`:37`)、`SSLMode: envOrDefault("DB_SSLMODE", defaultSSLMode)`(`:55`)、DSN 組み立て `values.Set("sslmode", cfg.SSLMode)`(`:93`)。
- **migrator の影響範囲が広い点を明記**: `app/migrator` は master 資格情報で `CREATE DATABASE`(`app/migrator/database.go` の `ensureDatabase`)を実行する高権限経路であり、その接続が `DB_SSLMODE` 未設定で平文へ後退すると、最も機密性の高い master 資格情報が平文で転送され得る。api / auth のアプリ接続よりも影響が大きい。
- **修正時の対象を一括化**: 課題 (m-2) の fail-closed 化(`APP_ENV ∈ {local, test}` 以外で `DB_SSLMODE` 未設定を起動エラーにする、または既定を `"require"` にし平文は明示 opt-in のみ)は、上記 **3 箇所(app/api/cmd/api/env.go・app/auth/cmd/authz/env.go・app/migrator/config.go)を一括対象**にする。1 箇所だけ直すと他経路で fail-open が残る。§4「実施内容」の (m-2) 項目はこの 3 箇所すべてに適用する前提で読むこと。
- 現状の実害は起票時と変わらず無い(iac が本番で `DB_SSLMODE="require"` を注入する。`app/iac/envs/dev/variables.tf` の `db_sslmode` 既定 `"require"`)。fail-open は注入漏れ・新環境・migrator への sslmode 未注入時にのみ顕在化する。severity は **medium** を維持(拡散の判明で対象箇所は増えたが、現状実害なし・回避策ありの性質は不変)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: 変わらず。ただし planner / impl が (m-2) fail-closed 化を計画化する際は、対象を上記 3 箇所に明示して漏れを防ぐ。migrator は高権限経路のため優先度を上げて検討する。

### 2026-07-10(修正ラウンド: 課題 (m-2) DB_SSLMODE の fail-open 既定を解消)

- 今回の修正ラウンドで、課題 **(m-2)(アプリ側 `DB_SSLMODE` 既定が `"disable"` で、注入漏れ時に静かに平文接続へ fail-open する)を解消**した。上記 2026-07-10 追記で「3 箇所に拡散」と特定した全経路を一括対象にして修正した。
- 実施内容(impl-db): `defaultSSLMode` を `"disable"` → `"require"`(fail-closed な既定)に変更。対象は特定済みの 3 箇所すべて — `app/api/cmd/api/env.go` / `app/auth/cmd/authz/env.go` / `app/migrator/config.go`。既定値を検証するテスト 3 件(各 `*_test.go`)を新既定 `"require"` に更新。`.claude/rules/db.md` の DB env 契約を新既定に更新し、「ローカルの非 TLS postgres へ接続する場合は `DB_SSLMODE=disable` を明示する」旨の注記を追加した。
- 影響監査(impl-db): 全 `DB_SSLMODE` 注入点(compose / Makefile / iac / CI / 統合テストの DSN)を監査し、いずれも `DB_SSLMODE` を明示注入済み(または明示 `disable` が必要なローカル / test 経路)で、既定変更による設定済み環境への影響が無いことを確認した。migrator の高権限経路(master 資格情報での `CREATE DATABASE`)も新既定で fail-closed 側に倒れる。
- 検証(checker): api / auth / migrator の `make check` green を確認。
- **status は open を維持。** 理由: 課題 **(R-c)(最小権限 DB ユーザー)が未対応で残る**ため。現状 api / auth は同一 RDS のマスターユーザーを共有し、`search_path` による名前空間分離のみで権限境界が無い。RDS のサービス別ロール分離 + サービス別 secret の配線は、iac 変更を伴うため別途 iac 計画で対応予定。
- severity は **medium** を維持(fail-open 側は解消したが、残る (R-c) の権限境界欠如という medium 相当のハードニングが未対応のため)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: 残る課題 (R-c) を将来 planner が計画化し、impl-iac(`modules/db` / `modules/service` / `envs` でスキーマ毎の専用ロール + 別 secret 注入)/ impl-db(必要なら実行時ロールとマイグレーション実行ロールの権限分離)で実装、tester / review-security を通す。(R-c) が解消した時点で本 Issue をクローズ可能。

### 2026-07-10(修正ラウンド: 残課題 (R-c) 最小権限 DB ユーザーを実装・検証し resolved へ)

- 残っていた課題 **(R-c)(api/auth が同一 RDS マスターユーザーを共有し、`search_path` による名前空間分離のみで権限境界が無い)を解消**した。方式は **random_password + Secrets Manager**(= ユーザー選択)。これにより (m-2)(先行ラウンドで解消済み)と合わせ、本 Issue の 2 課題が両方とも修正済みとなった。
- 実施内容(impl-iac): `modules/db` に `random_password` + api_app / auth_app 専用 Secrets Manager シークレット + outputs を追加。`envs/dev/main.tf` でアプリコンテナ(api / auth)を各 scoped secret 参照に切り替え、migrate init コンテナは master 資格情報を継続しつつ `APP_DB_USER` / `APP_DB_PASSWORD` を受領する構成にした。
- 実施内容(impl-db / app/migrator): `domain/migration`(`AppRole` VO + `RoleProvisioner` ポート)を追加。`infra/postgres/role.go` に冪等なロール付与(CREATE ROLE / ALTER PASSWORD / REVOKE CONNECT FROM PUBLIC / GRANT 最小権限 / ALTER DEFAULT PRIVILEGES。DDL 不可、クロス DB 遮断、並行競合リトライ)を実装。`service.Migrate` は goose up 後・かつ up コマンド時のみロール付与を行う。`cmd/migrator/env.go` は `APP_DB_*` 未設定なら後方互換でスキップする(fail-safe)。
- **検証(review-security)**: 実 Postgres で権限境界を確認 — DML 可 / DDL 不可 / クロス DB CONNECT 不可(双方向)/ master 接続維持。**Blocker 0**。指摘の Major 2 件を是正: (Major-2) `APP_DB_USER == master` の衝突ガードを migrator env + iac variable validation で fail-closed 化、(Major-1) 権限境界の統合テスト `app/migrator/infra/postgres/role_integration_test.go`(`//go:build integration`)+ CI ジョブ `migrator-integration`(postgres service で実行)を追加。
- **検証(tester / checker)**: tester が env / service gating の配線テストを追加。checker が migrator `make check`(gosec 0)/ iac fmt + validate / CI YAML green を確認。
- **受容済み残差(今回スコープ外・文書化済み・実害限定的)**: (1) migrator 自身は当面 master 資格情報を継続使用(専用 migrator ロール化・IAM 分離は将来)、(2) task execution role が migrate 用に master secret を読める(ECS では task role と分離され、コンテナプロセスからの直接取得は不可と評価済み)、(3) Minor-3(maintenance DB `postgres` への REVOKE CONNECT 未実施)、(4) Minor-5(並行競合検知のロケール依存メッセージ一致・fail-closed)、(5) ロールパスワードが Terraform state(S3 + 暗号化)に載る(random_password 方式のトレードオフ、ユーザー選択)。将来ハードニングする場合は別 Issue 化を検討する。
- **apply は未実施**: 実 backend + AWS 認証情報を要する `terraform apply` はユーザー判断に委ねる(agent は実行しない)。iac は fmt + validate まで検証済み。
- **status を resolved に更新**(updated=2026-07-10)。判定根拠: 本 Issue の 2 課題((m-2)fail-open sslmode / (R-c)最小権限 DB ユーザー)がいずれも修正され、review-security が実 Postgres で権限境界を検証(Blocker 0)し checker green を確認したため。残差は受容済み・文書化済みで、恒久デプロイ反映(apply)のみユーザー判断待ち。
- 次にやること: 恒久反映は `terraform apply` をユーザーが判断・実行する。受容済み残差((1)〜(5))を将来ハードニングする場合は別 Issue として起票する。
