---
id: ISSUE-047
title: app/api の JWKS フェッチに singleflight が無く、キャッシュ失効/未知 kid 時に thundering herd で app/auth の JWKS へバースト
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-047: app/api の JWKS フェッチに singleflight が無く thundering herd が起きる

**深刻度: Minor(review) / severity: low**(有界だが 5 分毎のバースト。可用性への軽微なリスク)

## 1. ユーザー価値への影響(なぜ対応するか)

> **API 利用中の全ユーザー** の **リクエスト応答の安定性** が **JWKS キャッシュ失効の瞬間の同時再フェッチにより、わずかに悪化し得る**。

- **影響を受けるユーザー**: app/api に高頻度アクセスする全ユーザー(間接影響)
- **損なわれる価値**: 応答の安定性・app/auth の JWKS エンドポイント負荷
- **影響範囲・頻度**: cacheTTL(5 分)失効の瞬間、または未知 kid 到達時にバースト(bounded)
- **回避策**: あり(現状でも致命的ではない。フェッチは有界)

## 2. 現象(何が起きているか)

### 期待する動作

JWKS キャッシュが失効した瞬間や未知 kid の検証時、複数リクエストが到達しても再フェッチは 1 本にまとめられ、app/auth の `/.well-known/jwks.json` への同時 HTTP は 1 回に収束する。

### 実際の動作

`app/api/infra/jwt/verifier.go:184-214`(`getKey` / `fetchJWKS`)では、キャッシュ失効・未知 kid 時の再フェッチが一本化されていない。cacheTTL=5min の失効の瞬間に到達した複数リクエストが、それぞれ個別に app/auth の JWKS エンドポイントへ HTTP フェッチを行う。bounded ではあるが 5 分毎にバーストが発生し得る。

### 再現手順

1. `app/api/infra/jwt/verifier.go:184-214` の getKey/fetchJWKS を読み、フェッチが singleflight / sync.Once 等で一本化されていないことを確認する。
2. cacheTTL 失効直後に同時に複数の検証リクエストを送ると、複数の JWKS フェッチが並行して発生する。

### 環境・条件

- 対象: app/api の JWT 検証(Bearer 保護)経路。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/api/infra/jwt/verifier.go:184-214` の再フェッチ経路にフェッチ重複排除が無い。
- 事実: cacheTTL = 5 分。失効の瞬間に到達した複数リクエストが個別フェッチする。
- 仮説: キャッシュ実装がヒット/ミスの二値で、ミス時の同時実行制御(coalescing)を持たない。

### 根本原因

JWKS 再フェッチに重複排除(singleflight)が無く、キャッシュミスの瞬間に同一フェッチが並行実行される。

## 4. 対応(どう解決するか)

### 対応方針

impl-api が JWKS フェッチを `singleflight.Group` か使い捨て `sync.Once` で一本化する。

### 実施内容

- [ ] `app/api/infra/jwt/verifier.go` の getKey/fetchJWKS を singleflight.Group もしくは per-fetch sync.Once で coalescing する(impl-api)
- [ ] 同時到達時にフェッチが 1 回に収束することをテストで確認(tester)

### 再発防止

- 外部フェッチ + キャッシュのパターンでは重複排除を既定にする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。パフォーマンスレビューで検出。`app/api/infra/jwt/verifier.go:184-214` の再フェッチが一本化されていないことを確認した。
- 関連: SPEC-015(app/api Bearer 保護)。
