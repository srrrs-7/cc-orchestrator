---
id: ISSUE-049
title: app/auth の /login にアプリ層のブルートフォース対策(lockout/スロットリング)が無く、緩和が CloudFront WAF の粗い rate limit のみに依存する
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-049: app/auth の /login にアプリ層のブルートフォース対策が無い

**深刻度: Minor(review) / severity: low**(パスワード総当たりに対する専用防御の欠如)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth のユーザー** の **アカウントの保護** が **ログイン専用のブルートフォース対策不在により、総当たり攻撃に晒される**。

- **影響を受けるユーザー**: app/auth に登録された全ユーザー(特に弱いパスワードのアカウント)
- **損なわれる価値**: アカウントの保護(パスワード総当たり耐性)
- **影響範囲・頻度**: /login への総当たり攻撃で顕在化。現状 bcrypt コストが自然な減速要因になるのみ
- **回避策**: なし(WAF の粗い rate limit のみ)

## 2. 現象(何が起きているか)

### 期待する動作

/login に対し、username 単位 / IP 単位の試行回数制限(一定失敗で一時ロック / スロットリング等)があり、総当たりが実用的でなくなる。

### 実際の動作

`app/auth/route/login_handler.go` / `app/auth/service/authentication_service.go` にアプリ層のブルートフォース対策(lockout / スロットリング)が無い。緩和は CloudFront WAF の rate_based ルール(2000 req / 5min / IP)のみで、ログイン専用の粒度ではない。実質、bcrypt のコストのみが自然な減速要因になっている。

### 再現手順

1. `app/auth/route/login_handler.go` / `authentication_service.go` を読み、失敗試行のカウント / ロック / スロットリングが無いことを確認する。
2. 単一 username に対し高頻度(WAF 閾値未満の分散 IP 等)でパスワードを総当たりしても、アプリ側で拒否・遅延が入らないことを確認する。

### 環境・条件

- 対象: app/auth の /login。緩和は CloudFront WAF のみ(本番経路)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/route/login_handler.go` / `authentication_service.go` に試行回数制限・ロック機構が無い。
- 事実: 緩和は CloudFront WAF rate_based(2000 req / 5min / IP)のみでログイン専用でない。
- 仮説: 認証機能を先に成立させる段階で、ブルートフォース対策が後回しになった。

### 根本原因

/login にアプリ層の試行回数制限がなく、防御がインフラ層の粗い rate limit に依存している。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が username 単位 / IP 単位の試行回数制限を検討・実装する(一定失敗で一時ロック、指数的バックオフ等)。ストレージ設計(in-memory か Postgres か)は idpsession/authcode の永続化方針と整合させる。

### 実施内容

- [ ] username 単位 / IP 単位の失敗試行カウントと一時ロック / スロットリングを設計・実装(impl-auth)
- [ ] カウンタの永続化方針(in-memory purge 有 or Postgres)を決定(ISSUE-044 の idpsession 方針と整合)
- [ ] ロック挙動をテストで確認(tester)

### 再発防止

- 認証エンドポイントはアプリ層の試行制限を必須要件として Spec 化する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/route/login_handler.go` / `authentication_service.go` にブルートフォース対策が無く、緩和が CloudFront WAF の粗い rate limit のみであることを確認した。
- 関連: SPEC-015。カウンタ永続化は ISSUE-044(idpsession purge)と方針を整合させる。
