---
id: ISSUE-039
title: app/auth クライアント / ユーザー管理 API(seed 以外の登録・更新)
status: resolved
severity: medium
created: 2026-07-12
updated: 2026-07-12
specs: []
---

# ISSUE-039: クライアント / ユーザー管理 API

## 1. ユーザー価値への影響(なぜ対応するか)

> **IdP 運用者** が **新規 RP やユーザーをコード変更・再起動なしで登録** できず、**demo-client / demo-user seed のみ** に依存する。

- **影響を受けるユーザー**: 複数 RP を接続する開発者、ステージング/本番で client を増やす運用者
- **損なわれる価値**: 動的な client ライフサイクル、ユーザー CRUD、監査可能性
- **影響範囲・頻度**: 新 RP 追加・ユーザー追加のたび
- **回避策**: `cmd/authz/main.go` seed 変更 + 再デプロイ

## 2. 現象(何が起きているか)

### 期待する動作

- 管理 API または Dynamic Client Registration(RFC 7591)で client 登録
- ユーザー登録(パスワード設定) — ISSUE-031/005 と連動
- client.Repository は現状 read-only(`FindByID` のみ)

### 実際の動作

- 起動時 `UpsertClient` / user seed のみ
- DCR 未実装

## 3. 原因(なぜ起きているか)

基盤サンプルとして固定 seed で十分と判断(AUTH-001)。

## 4. 対応(どう解決するか)

### 対応方針

- 管理 API は admin 認証必須(mTLS / API key / 別 IdP — planner 決定)
- DCR は public 登録のリスクが高いため、まず **管理 API + 手動登録** を推奨
- ISSUE-035(confidential client secret)とスキーマを共設計

### 実施内容(チェックリスト)

- [ ] client / user の write repository
- [ ] 管理 route(または migrator/CLI 第一版)
- [ ] redirect_uri 検証・監査ログ
- [ ] テスト + review-security

### 関連

- ロードマップ: AUTH-002 Phase 4.1
- 関連: ISSUE-035, ISSUE-031

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 4.1。

### 2026-07-12 (resolved)

- `POST /admin/clients` / `POST /admin/users` 管理 API。`ADMIN_API_KEY` 認証(fail-closed)。
- 検証: `REQUIRE_DB=1 make -C app/auth check` 緑。
