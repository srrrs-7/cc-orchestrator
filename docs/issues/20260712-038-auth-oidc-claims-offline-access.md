---
id: ISSUE-038
title: app/auth OIDC クレーム強化(auth_time / at_hash) と offline_access ゲート
status: open
severity: low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-006]
---

# ISSUE-038: OIDC クレーム強化と offline_access ゲート

## 1. ユーザー価値への影響(なぜ対応するか)

> **OIDC 準拠を重視する RP 開発者** の **ID token / refresh token セマンティクス** が、**簡略実装のままでは OIDC Core の推奨クレーム・offline_access パターンを満たさない**。

- **影響を受けるユーザー**: 厳密な OIDC クライアント実装者
- **損なわれる価値**: `auth_time` による再認証判断、`at_hash` による access token バインディング、refresh 発行の consent ゲート
- **影響範囲・頻度**: ID token 検証・refresh 発行時
- **回避策**: 現状 web は ID token payload を表示用途のみで署名検証なし(SPEC-015 スコープ外)

## 2. 現象(何が起きているか)

### 期待する動作

- ID token に `auth_time`(ログイン時刻)を設定
- 必要に応じ `at_hash` を access token から算出して ID token に含める
- `offline_access` スコープ要求時のみ refresh token 発行(OIDC Core §11 パターン)

### 実際の動作

- `auth_time` フィールドは型定義のみ未設定
- `at_hash` 未実装
- refresh_token グラント対応 client には常に refresh 発行(SPEC-006 設計判断)

## 3. 原因(なぜ起きているか)

SPEC-006 で offline_access ゲートをスコープ外。auth_time はログイン UI 未実装(ISSUE-031)のため設定元がない。

## 4. 対応(どう解決するか)

### 対応方針

- `auth_time` は ISSUE-031 完了後に IdP セッション確立時刻から設定
- `offline_access` ゲートは ISSUE-032(consent)とセットで検討
- Discovery `claims_supported` 更新

### 実施内容(チェックリスト)

- [ ] `auth_time` 設定
- [ ] `at_hash` 算出(オプションだが OIDC 推奨)
- [ ] offline_access ポリシー決定 + 実装
- [ ] テスト + review-spec

### 関連

- ロードマップ: AUTH-002 Phase 2.3
- 依存: ISSUE-031, ISSUE-032(offline_access 時)

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 2.3。
