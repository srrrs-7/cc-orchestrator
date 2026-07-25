---
id: ISSUE-050
title: app/auth の /login と /consent にアプリ独自のリクエストボディサイズ上限(1MiB)が未適用で、Go 標準の暗黙上限に依存している
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-050: app/auth の /login と /consent にボディサイズ上限が未適用

**深刻度: Minor(review) / severity: low**(ISSUE-010 と同型の堅牢化不足。login/consent が例外的に未適用)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth を利用する全ユーザー** の **サービス可用性** が **login/consent への過大ボディによる緩やかな DoS で、わずかに損なわれ得る**。

- **影響を受けるユーザー**: app/auth に依存する全ユーザー(間接影響)
- **損なわれる価値**: サービス可用性(メモリ・帯域の防御)
- **影響範囲・頻度**: login/consent への過大リクエストボディで顕在化(現状は Go 標準の暗黙上限 ~10MB に依存)
- **回避策**: あり(Go 標準の暗黙上限で極端な事態は緩和されるが、他エンドポイントと非一貫)

## 2. 現象(何が起きているか)

### 期待する動作

/login と /consent も、他のフォーム受理エンドポイント(/token /revoke /introspect)と同様にアプリ独自のボディサイズ上限(maxFormBodySize = 1<<20 = 1MiB、ISSUE-010 parity)を適用する。

### 実際の動作

`app/auth/route/login_handler.go:40` と `app/auth/route/consent_handler.go:72` は生の `r.ParseForm()` を呼んでおり、`app/auth/route/form.go` の `parseFormBody`(maxFormBodySize = 1<<20)を使っていない。parseFormBody は /token /revoke /introspect のみで使われ、login/consent は Go 標準の暗黙上限(~10MB)に依存している。

### 再現手順

1. `app/auth/route/login_handler.go:40` / `consent_handler.go:72` が生の `r.ParseForm()` を使うことを確認する。
2. `app/auth/route/form.go` の `parseFormBody`(1MiB 上限)が login/consent で使われていないことを確認する。
3. login/consent に 1MiB 超のフォームボディを送ると 1MiB でエラーにならず、Go 標準上限まで受理される。

### 環境・条件

- 対象: app/auth の /login / /consent(フォーム受理)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/route/login_handler.go:40` / `consent_handler.go:72` は生 `r.ParseForm()`。
- 事実: `app/auth/route/form.go` の `parseFormBody`(maxFormBodySize = 1<<20、ISSUE-010 parity)は /token /revoke /introspect のみで使用。
- 仮説: login/consent を後発で追加した際、既存の parseFormBody へ寄せず標準 ParseForm を使ってしまった。

### 根本原因

login/consent がフォーム受理の共通ヘルパ(parseFormBody)を経由せず、ボディサイズ上限の一貫適用から漏れている。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が /login / /consent のフォーム解析を `parseFormBody` に統一する(他エンドポイントと parity)。

### 実施内容

- [ ] `login_handler.go` / `consent_handler.go` の `r.ParseForm()` を `parseFormBody`(1MiB 上限)に置き換える(impl-auth)
- [ ] 1MiB 超で拒否されることをテストで確認(tester)

### 再発防止

- フォーム受理は必ず `parseFormBody` 経由にする(生 `ParseForm` を lint / レビューで検出)。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/route/login_handler.go:40` / `consent_handler.go:72` が生 `r.ParseForm()` を使い、`form.go` の `parseFormBody`(1MiB)を経由していないことを確認した。
- 関連: ISSUE-010(app/api のボディサイズ / タイムアウト堅牢化、resolved)、SPEC-015。
