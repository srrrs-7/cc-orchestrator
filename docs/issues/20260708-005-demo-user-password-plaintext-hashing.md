---
id: ISSUE-005
title: デモユーザーのパスワードが平文保持・平文比較(将来ログイン/同意画面を実装して VerifyPassword を配線する際はハッシュ化+定数時間比較が必須)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-09
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-005: デモユーザーのパスワードが平文保持・平文比較(将来ログイン/同意画面を実装して VerifyPassword を配線する際はハッシュ化+定数時間比較が必須)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth に実際のログイン/同意画面を実装して認証を有効化する将来の開発者(および認証されるエンドユーザー)** の **資格情報(パスワード)の保護** が **User 集約がパスワードを平文で保持し、`VerifyPassword` が平文比較を行う実装のまま配線されると損なわれる**。

- **影響を受けるユーザー**: 本基盤にログインフローを差し込んで実運用化する開発者と、そこで認証されるエンドユーザー
- **損なわれる価値(将来条件下)**: パスワードの機密性(平文保持はストア/ログ露出時に即漏洩)と、タイミング攻撃耐性(`==` の非定数時間比較)
- **影響範囲・頻度**: **現時点では実害なし。** `VerifyPassword` はどこからも呼ばれておらず(未配線)、パスワードは検証に一切使われていない。将来ログイン画面を実装してここに配線したときにのみ実害となる
- **回避策**: あり(将来の配線時に本 Issue のチェックリストに沿ってハッシュ化+定数時間比較へ置き換える)

## 2. 現象(何が起きているか)

### 期待する動作

実際にパスワード認証を行う認可サーバーでは:

1. パスワードは平文で保持せず、`bcrypt` / `scrypt` / `argon2` 等の**ソルト付きハッシュ**で保存する
2. 照合は保存済みハッシュに対して行い、比較は `subtle.ConstantTimeCompare` 相当の**定数時間比較**でタイミング攻撃を防ぐ

### 実際の動作(基盤サンプルの現状)

- `User` 集約は `password string` を**平文で保持**する(`app/auth/domain/user/user.go:14`、`New`/`Reconstruct` が平文をそのまま格納 `:21-30`)。
- `VerifyPassword` は `candidate != "" && candidate == u.password` の**平文・非定数時間比較**(`app/auth/domain/user/user.go:41-43`)。
- ただし `VerifyPassword` は現在どこからも呼ばれていない(`resolveOwner` は password を検証せず seed ユーザーを割り当てる。`app/auth/service/authorization_service.go:153-180`)。コメント自体も「production では hash が必要」と明記している(`user.go:19-20`、`:32-40`)。

### 再現手順

1. `app/auth/domain/user/user.go:14` を開き、`User` 構造体の `password string`(平文フィールド)を確認する。
2. 同ファイル `:41-43` の `VerifyPassword` が `candidate == u.password` の平文比較であることを確認する。
3. `app/auth/service/authorization_service.go:164-180` の `resolveOwner` を開き、`VerifyPassword` を呼ばず(=パスワード未検証で)resource owner を決めていることを確認する(= 現状は未配線で実害なし)。
4. 将来ログイン画面を差し込む際に、この `VerifyPassword` をそのまま配線するとパスワードの平文保持・平文比較が本番経路に乗ることを確認する。

### 環境・条件

- 対象: `app/auth`(OAuth 2.0 認可サーバー / OpenID Provider 基盤サンプル、Go)
- 発見文脈: AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-security)で Minor として挙がった、将来のログイン実装時に必須となるパスワード保護のチェックリスト

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `User.password` は平文の `string`(`app/auth/domain/user/user.go:14`)。`New`/`Reconstruct` は平文をそのまま保持(`:21-30`)。
- 事実: `VerifyPassword` は `candidate == u.password` の平文・非定数時間比較(`:41-43`)。
- 事実: `VerifyPassword` は現行フローで未配線(`resolveOwner` は password を使わない。`app/auth/service/authorization_service.go:164-180`)。よって現時点でパスワードの機密性・タイミング攻撃は実害に至らない。
- 事実: 計画は「エンドユーザ認証は基盤簡略化として seed デモユーザへ自動割り当て」「本来ログイン画面・同意画面を差し込む箇所」と方針化しており(`docs/plans/AUTH-001-plan.md` 方針 3、リスク欄「エンドユーザ認証の簡略化」)、平文パスワードはこの簡略化に沿った意図的な暫定。
- 事実: 秘密情報を直書きしない方針(`.claude/rules/auth.md` セキュリティ規約、`project.md`)に沿い、デモパスワードは起動時 seed で注入されコードには直書きされていない(平文保持は「保存形式」の問題で、直書き禁止とは別論点)。

### 根本原因

**現行のバグではない。** 基盤サンプルがログイン/同意 UI を持たず `VerifyPassword` を配線しないため、集約の形だけ実 IdP に合わせて残した平文パスワード実装が、現時点では安全に無害化されている。将来ログイン認証を実装してここに配線する時点で、ハッシュ化・定数時間比較への置き換えが必須要件になる。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 本件は AUTH-001(`docs/plans/AUTH-001-plan.md`)の**基盤サンプル**における簡略化であり、`VerifyPassword` は未配線で現行の実害はない。**今回のスコープでは対応せず**、将来ログイン/同意画面を実装して認証を有効化する際に必須で実施するチェックリストとして記録・追跡する。
- 参照: `docs/plans/AUTH-001-plan.md`(方針 3・リスク欄「エンドユーザ認証の簡略化」)、`app/auth/domain/user/user.go:14`(平文フィールド)・`:41-43`(平文比較)、`app/auth/service/authorization_service.go:164-180`(未配線の `resolveOwner`)、`.claude/rules/auth.md` セキュリティ規約。
- 手順: 将来ログイン実装を決めた時点で planner が計画化し、impl-api が実装、tester がハッシュ照合の正常/異常/境界を検証、checker(`make check`)・review-security を通す。

### 実施内容(将来ログイン実装時のチェックリスト)

- [ ] パスワードを平文保持せず、`bcrypt` / `scrypt` / `argon2` 等のソルト付きハッシュで保持する(`User` の `password` をハッシュ値に置換)
- [ ] `VerifyPassword` を保存済みハッシュとの照合に変更し、`subtle.ConstantTimeCompare` 相当の定数時間比較(またはハッシュライブラリの定数時間比較 API)を用いる
- [ ] ハッシュ用の外部依存を導入する場合は、計画の「外部依存ゼロ」方針との整合を planner/レビューで確認する(`golang.org/x/crypto/bcrypt` 採用可否を含めて判断)
- [ ] `Password()` getter が平文ハッシュ以外を漏らさないこと、ログ・エラーに資格情報が出ないことを確認する
- [ ] ログイン失敗時の応答・タイミングがユーザー存在有無を漏らさないことを確認する
- [ ] (SPEC-005 で追加された永続化面) `users.password` 列(`app/auth/db/migrations/000001_create_auth.sql:37-43` の `password text NOT NULL`)に平文を保存しない。ハッシュ化に合わせて列名を `password_hash` に改名し、`app/auth/infra/postgres/seed.go:72-84`(`SeedUser` の `u.Password()` 平文 UPSERT)・`db/queries/users.sql` の Upsert・sqlc 生成コードを更新する
- [ ] `users.password` 列を扱うマイグレーションは破壊的変更(列改名・内容変更)を含むため、SPEC-005 の「マイグレーション安全性」に沿ってレビューで明示・報告する

### 再発防止

- 認証基盤では「資格情報は平文で保持・比較しない」を規約(`.claude/rules/auth.md`)として明示済み。ログイン経路を配線する変更では review-security を必須ゲートにし、本チェックリストを参照する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-security)で Minor として挙がった、デモユーザーのパスワード平文保持・平文比較を、将来ログイン実装時のチェックリストとして記録。
- 事実確認: `app/auth/domain/user/user.go:14`(平文 `password`)・`:41-43`(`candidate == u.password` の平文比較)を確認。`VerifyPassword` は `app/auth/service/authorization_service.go:164-180` の `resolveOwner` から呼ばれておらず未配線で、現時点の実害はゼロ。
- severity は **low** と判定。判定根拠: `VerifyPassword` が未配線でパスワードが検証に一切使われないため現行の実害なし。将来ログイン認証を配線したときにのみ資格情報保護・タイミング攻撃の実害となる予防的ハードニング。回避策(配線時にハッシュ化+定数時間比較へ置換)ありのため low(critical/high/medium ではないのは現に価値が損なわれていないため)。
- 次にやること: ログイン/同意画面の実装を決めた時点で planner に計画化を依頼し、ハッシュ化+定数時間比較を impl-api/tester/checker/review-security で実施する。

### 2026-07-09

- SPEC-005(app/api・app/auth の Postgres 永続化)により、本 Issue が指摘する平文パスワードが `infra/memory`(再起動でリセット)から Postgres の恒久ストレージへ移った。`app/auth/db/migrations/000001_create_auth.sql:37-43` が `users.password text NOT NULL` を定義し、`app/auth/infra/postgres/seed.go:72-84` の `SeedUser` が `u.Password()` を平文のまま UPSERT する(`:78`)。SPEC-005 plan §6.1 R-b で「現状踏襲・ハッシュ化は将来 Issue」と明示評価され、review-security(E3)で本 Issue に集約して追跡すると判断された。
- 対応(将来ログイン実装時)チェックリストに、平文を at-rest 保存しないための Postgres 面の項目(列名 `password`→`password_hash` 改名、`seed.go` / `db/queries/users.sql` / sqlc 生成の更新、破壊的マイグレーションのレビュー)を追記した。frontmatter の `specs` に SPEC-005 を相互リンク、`updated` を 2026-07-09 に更新した。
- severity は **low** を維持。判定根拠: `VerifyPassword` は依然として未配線で、seed される demo password は起動毎に生成され検証に一切使われないため現行の実害はゼロ。Postgres 化で「再起動による自然消去」が失われた点は将来の footgun を増すが、現行フローで資格情報が実際に保護対象として使われていない状況は変わらないため low を据え置く。
- 次にやること: 変更なし(ログイン/同意画面の実装を決めた時点で planner に計画化を依頼)。その計画では SPEC-005 の永続化面(列改名・seed / query / 生成コード更新)も同時に実施する。
