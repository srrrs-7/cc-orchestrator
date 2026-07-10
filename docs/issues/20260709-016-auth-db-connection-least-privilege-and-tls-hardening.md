---
id: ISSUE-016
title: SPEC-005 の DB 接続が最小権限でない(api/auth 共有マスターユーザ)/ アプリ側 DB_SSLMODE 既定が "disable" で平文接続へ fail-open する
status: open  # open | investigating | fixing | resolved | closed | wontfix
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

- **今回のスコープ(SPEC-005)では対応しない。** 以下 2 項目をハードニングとして将来対応・追跡する。SPEC-005 plan §6.1(R-c / m-2)で「現状踏襲」と明示評価済み。
- 参照: `app/iac/envs/dev/main.tf`(DB_USER/DB_PASSWORD の同一シークレット参照)、`app/iac/modules/db/README.md:52-54`(名前空間分離の明記)、`app/auth/infra/postgres/db.go:46,117`(`DB_SSLMODE` 既定 `disable` フォールバック)、SPEC-005 plan §6.1(review-security E3)。

### 実施内容(将来対応時のチェックリスト)

- [ ] (最小権限 / R-c) auth スキーマ・api スキーマそれぞれに専用 DB ロールを作り、各ロールに自スキーマのみの権限を付与する。api / auth のアプリに別々の資格情報(別 Secrets)を注入する(iac: `modules/db` / `modules/service` / `envs`、impl-iac)。マイグレーション実行ロールと実行時ロールの権限分離も検討する
- [ ] (fail-closed sslmode / m-2) `APP_ENV ∈ {local, test}` 以外で `DB_SSLMODE` 未設定を起動エラーにする(または既定を `"require"` にし、平文を許すのは明示 opt-in のみ)。`app/auth/infra/postgres/db.go` の `ConfigFromEnv` / `SelectMode` 相当で fail-closed を sslmode にも適用する(impl-db)。app/api 側の同等コードがあれば併せて対応する
- [ ] 上記に伴う統合テスト(fail-closed 分岐、専用ロールでの権限境界)を追加する(tester)
- [ ] レビュー(review-security)で権限境界と転送暗号化が意図どおりであることを確認する

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
