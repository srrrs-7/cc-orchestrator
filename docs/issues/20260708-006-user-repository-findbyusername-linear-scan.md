---
id: ISSUE-006
title: UserRepository.FindByUsername が map の線形走査(O(n))で、ユーザーストアが多数ユーザーに拡張されると劣化する
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-08
updated: 2026-07-12
specs: []  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-006: UserRepository.FindByUsername が map の線形走査(O(n))で、ユーザーストアが多数ユーザーに拡張されると劣化する

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth のインメモリユーザーストアを多数ユーザーを持つ設計へ拡張する将来の開発者(および認可を待つエンドユーザー)** の **`/authorize` の応答性能** が **`FindByUsername` が全ユーザーを線形走査(O(n))する実装のままユーザー数が増えると損なわれる**。

- **影響を受けるユーザー**: インメモリストアを seed 1 件から多数ユーザー(または別ストア実装)へ拡張する開発者と、その `/authorize`(`login_hint` 突合)を通るエンドユーザー
- **損なわれる価値(将来条件下)**: ユーザー名解決のレイテンシ。ユーザー数 n に比例して `FindByUsername` の走査コストが増える
- **影響範囲・頻度**: **現時点では実害ゼロ。** seed ユーザーは 1 件で、線形走査でも実質 O(1)。多数ユーザーを持つ設計に拡張された場合にのみ性能問題として顕在化する
- **回避策**: あり(`username → *User` の二次インデックスを追加する)

## 2. 現象(何が起きているか)

### 期待する動作

ユーザー名によるルックアップ(`FindByUsername`)が、ユーザー数に依存しない平均 O(1) で解決される(ID ルックアップ `FindByID` が `byID` マップで O(1) なのと同様)。

### 実際の動作

`FindByUsername` は `byID` マップ全体を `range` で線形走査し、`u.Username() == username` に一致する最初のユーザーを返す(O(n))。`FindByID` は `byID` マップ直引きで O(1) なので、ルックアップ経路によって計算量が非対称になっている。

### 再現手順

1. `app/auth/infra/memory/user_repository.go:61-77` の `FindByUsername` を開き、`for _, u := range r.byID { if u.Username() == username { ... } }` で map 全体を走査していることを確認する(O(n))。
2. 同ファイル `:42-57` の `FindByID` が `r.byID[id]` の直引き(O(1))であることと対比する。
3. `app/auth/infra/memory/user_repository.go` に `username → *User` の二次インデックスが無いことを確認する(`UserRepository` は `byID` マップのみ保持。`:13-16`)。

### 環境・条件

- 対象: `app/auth`(OAuth 2.0 認可サーバー / OpenID Provider 基盤サンプル、Go)
- 発見文脈: AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-performance)で Minor として挙がった、将来拡張時の計算量に関する指摘

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `UserRepository` は `byID map[user.UserID]*user.User` のみを保持し、username 索引を持たない(`app/auth/infra/memory/user_repository.go:13-16`)。
- 事実: `FindByUsername` は `byID` を線形走査する(`:61-77`)。`FindByID` は直引き O(1)(`:42-57`)。
- 事実: 現状 seed ユーザーは 1 件(計画の「デモ用の client / user / RSA 鍵は起動時に生成・seed」。`docs/plans/AUTH-001-plan.md` 方針 4、`app/api` の in-memory ストア踏襲=永続化しない)。n=1 では線形走査でも実質 O(1) のため現行の性能影響はない。
- 事実: `FindByUsername` は `/authorize` の `login_hint` 突合(`resolveOwner`。`app/auth/service/authorization_service.go:164-180`)と既定ユーザー解決から呼ばれる。将来ユーザー数が増えると認可リクエストごとに O(n) 走査が走る。

### 根本原因

**現行のバグではない。** 基盤サンプルが seed 1 件の in-memory ストア(`app/api` と同型)であり、二次インデックスを持たないのは実装の簡潔さを優先した設計判断。多数ユーザーを扱う設計(seed 多数化・別ストア実装への差し替え)に拡張したときにのみ、username 索引の不在が O(n) 走査として性能問題化する。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 本件は AUTH-001(`docs/plans/AUTH-001-plan.md`)の**基盤サンプル**における簡略化であり、seed 1 件の現状では実害ゼロ。**今回のスコープでは対応せず**、ユーザーストアが多数ユーザーを持つ設計に拡張される場合の将来対応候補として記録・追跡する。
- 対応する場合の方針(仮説含む): `UserRepository` に `byUsername map[user.Username]*user.User` の二次インデックスを追加し、`Seed`(および将来の Save)で `byID` と同時に更新、`FindByUsername` を直引き O(1) にする。`sync.RWMutex` の保護範囲・clone(`cloneUser`)方針は既存と同じに保つ。
- 参照: `docs/plans/AUTH-001-plan.md`(方針 4「デモ用 client/user の seed」、テスト戦略 infra レベルの Save/Find)、`app/auth/infra/memory/user_repository.go:13-16`(構造体)・`:61-77`(`FindByUsername`)・`:42-57`(`FindByID` の O(1) 対比)。
- 手順: 拡張を決めた時点で planner が計画化し、impl-api が二次インデックスを実装、tester が「username での取得(正)/ 未存在=`ErrNotFound`(異)/ 索引と `byID` の整合(境界)」を table-driven で検証、checker(`make check`)・review-performance を通す。

### 実施内容

- [ ] `UserRepository` に `username → *User` の二次インデックス(`byUsername` マップ)を追加する
- [ ] `Seed`(将来 Save を追加する場合はそれも)で `byID` と `byUsername` を一貫して更新する
- [ ] `FindByUsername` をマップ直引き(O(1))に置き換える
- [ ] 二次インデックスと `byID` の整合を検証する infra テストを追加する

### 再発防止

- in-memory リポジトリを多数エントリ設計へ拡張する際は、全ルックアップ経路(ID / 属性)の計算量を揃える(必要な属性に索引を張る)ことをレビュー観点にする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-08

- 起票。AUTH-001 基盤(`docs/plans/AUTH-001-plan.md`)のレビュー(review-performance)で Minor として挙がった、`UserRepository.FindByUsername` の O(n) 線形走査を将来拡張時の対応候補として記録。
- 事実確認: `app/auth/infra/memory/user_repository.go:61-77` が `byID` を線形走査(O(n))、`:42-57` の `FindByID` は直引き(O(1))であることを確認。username 索引は未保持(`:13-16`)。現状 seed 1 件のため実害ゼロ。
- severity は **low** と判定。判定根拠: seed 1 件の現状では O(n)=O(1) 相当で性能影響がなく、多数ユーザー設計へ拡張した場合にのみ顕在化する予防的性能改善。回避策(二次インデックス追加)ありのため low(critical/high/medium ではないのは現に応答性能が損なわれていないため)。
- 次にやること: ユーザーストアの多数ユーザー化を決めた時点で planner に計画化を依頼し、`byUsername` 二次インデックスを impl-api/tester/checker/review-performance で実施する。

### 2026-07-12

- 解消確認。SPEC-011 により `app/auth/infra/memory/user_repository.go` は削除済み(残存は `idp_session_store.go` のみ)。永続化は Postgres 一本化。
- 事実: `users.username` に `UNIQUE` 制約(`app/auth/infra/postgres/schema/migrations/000001_create_auth.sql`)、`FindByUsername` は sqlc の `GetUserByUsername`(`WHERE username = $1`)で O(1) インデックスルックアップ(`app/auth/infra/postgres/user_repository.go`)。インメモリ O(n) 走査の問題は Postgres 移行により該当なし。
