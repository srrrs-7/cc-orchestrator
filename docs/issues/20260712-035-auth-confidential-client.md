---
id: ISSUE-035
title: app/auth Confidential client 対応(client_secret 認証)
status: resolved
severity: low
created: 2026-07-12
updated: 2026-07-12
specs: []
---

# ISSUE-035: Confidential client 対応

## 1. ユーザー価値への影響(なぜ対応するか)

> **サーバーサイド RP 開発者** が **client_secret による token endpoint 認証** を使えず、**public client + PKCE のみ** に限定される。

- **影響を受けるユーザー**: BFF / サーバーサイド OAuth クライアントを構築する開発者
- **損なわれる価値**: confidential client パターン、token endpoint の client 認証多様性
- **影響範囲・頻度**: confidential client を登録したい場合に常時
- **回避策**: public client + PKCE(SPEC-015 / web はこのパターン)

## 2. 現象(何が起きているか)

### 期待する動作

- `token_endpoint_auth_methods_supported` に `client_secret_post` または `client_secret_basic`
- `/token` / `/revoke` で secret 検証
- client 集約に secret hash 保持(平文非保存)

### 実際の動作

- `token_endpoint_auth_methods_supported: ["none"]` のみ
- client secret フィールドなし

## 3. 原因(なぜ起きているか)

AUTH-001 で SPA 主軸の public client を採用し、secret 管理の複雑さを回避。

## 4. 対応(どう解決するか)

### 対応方針

- ISSUE-039(client 管理)とセットで設計が効率的
- secret は bcrypt/argon2 等でハッシュ保存(ISSUE-005 と同型)

### 実施内容(チェックリスト)

- [ ] client スキーマ拡張 + migration
- [ ] token / revoke handler の client 認証分岐
- [ ] Discovery メタデータ更新
- [ ] テスト + review-security

### 関連

- ロードマップ: AUTH-002 Phase 3.1

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 3.1。

### 2026-07-12 (resolved)

- `client_secret_hash`(bcrypt)、`client_secret_post`/`client_secret_basic` 認証、`/token`/`/revoke` 対応。migration 000006。
- 検証: `REQUIRE_DB=1 make -C app/auth check` 緑。
