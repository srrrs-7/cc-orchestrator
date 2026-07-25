---
id: ISSUE-034
title: app/auth トークン失効エンドポイント POST /revoke (RFC 7009)
status: resolved
severity: medium
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-034: トークン失効エンドポイント POST /revoke

## 1. ユーザー価値への影響(なぜ対応するか)

> **RP / 運用者** が **access token / refresh token を明示的に失効** できず、**盗用・ログアウト後のトークン残存リスク** が残る。

- **影響を受けるユーザー**: セキュリティ要件を持つ RP、コンプライアンス対応が必要な運用者
- **損なわれる価値**: 能動的なトークン失効(RFC 7009)。現状は refresh 再利用検知時の family 失効のみ
- **影響範囲・頻度**: ログアウト・端末紛失・権限剥奪時
- **回避策**: access token は TTL(1h)待ち。refresh はローテーション依存

## 2. 現象(何が起きているか)

### 期待する動作

- `POST /revoke` で refresh_token または access token(実装方針による)を失効
- 成功/未知トークンとも HTTP 200(RFC 7009 §2.2)
- public / confidential client 双方の client 認証パターン(ISSUE-035 と整合)

### 実際の動作

- `/revoke` エンドポイントなし

## 3. 原因(なぜ起きているか)

README「将来拡張点」として意図的に未実装。

## 4. 対応(どう解決するか)

### 対応方針

- refresh_token: DB 上の hash 行削除または revoked フラグ
- access token(JWT): 短命のため blocklist は任意。まず refresh 失効を優先
- ISSUE-033 ログアウトフローから revoke を呼ぶ

### 実施内容(チェックリスト)

- [ ] `route/revoke_handler.go` + service ユースケース
- [ ] Discovery への記載(optional、`revocation_endpoint`)
- [ ] web logout 連携
- [ ] RFC 7009 準拠テスト
- [ ] review-security

### 関連

- ロードマップ: AUTH-002 Phase 2.1

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 2.1。

### 2026-07-12 (resolved)

- `POST /revoke` 実装(RFC 7009)。refresh token は family 単位で失効。Discovery に `revocation_endpoint` 追加。web Sign out から revoke 呼び出し。
- 検証: `REQUIRE_DB=1 make -C app/auth check` 緑、`make -C app/web check` 140 tests 緑。コミット `e609154`。
