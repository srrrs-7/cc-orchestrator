---
id: ISSUE-033
title: app/auth RP-initiated logout / OIDC end_session_endpoint
status: open
severity: medium
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-033: RP-initiated logout / end_session_endpoint

## 1. ユーザー価値への影響(なぜ対応するか)

> **RP 利用者** が **Sign out で IdP セッションも終了できる** 体験が、**web 側の sessionStorage クリアのみでは不完全**。

- **影響を受けるユーザー**: 共有端末利用者、セキュリティ要件を持つ RP 開発者
- **損なわれる価値**: IdP セッションの残存(ブラウザで再 authorize すると無操作ログイン相当)
- **影響範囲・頻度**: ログアウト操作のたび
- **回避策**: 部分(クライアント側トークン削除のみ。IdP セッションは残る)

## 2. 現象(何が起きているか)

### 期待する動作

- RP が `end_session_endpoint` へ redirect または front-channel logout を実行
- IdP セッション Cookie が失効
- 任意で `post_logout_redirect_uri` へ戻る(OIDC RP-Initiated Logout)

### 実際の動作

- エンドポイント未実装
- app/web は sessionStorage クリア + `/login` redirect のみ(SPEC-015)

## 3. 原因(なぜ起きているか)

AUTH-001 スコープ外。IdP セッション自体が ISSUE-031 まで存在しない。

## 4. 対応(どう解決するか)

### 対応方針

- OIDC RP-Initiated Logout 1.0 に沿い `end_session_endpoint` を Discovery に追加
- ISSUE-031(IdP セッション)および ISSUE-034(/revoke)と設計整合
- app/web `logout()` から end_session へ誘導

### 実施内容(チェックリスト)

- [ ] `GET /logout` または `/end_session` ハンドラ
- [ ] Discovery `end_session_endpoint` 公開
- [ ] `id_token_hint` / `post_logout_redirect_uri` / `state` 検証
- [ ] web Sign out フロー更新
- [ ] テスト + review-security

### 関連

- ロードマップ: AUTH-002 Phase 1.3
- 依存: ISSUE-031 推奨、ISSUE-034 と並行可

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 1.3。
