---
id: ISSUE-031
title: app/auth ログイン UI と IdP セッション(/authorize の自動 demo-user 割り当てを置き換える)
status: resolved
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-031: app/auth ログイン UI と IdP セッション

## 1. ユーザー価値への影響(なぜ対応するか)

> **Task Manager 等の RP 利用者** の **本人確認されたログイン体験** が、**/authorize が常に demo-user を自動割り当てする現状のままでは得られない**。

- **影響を受けるユーザー**: app/auth を IdP として使うエンドユーザー、および本番相当の認証フローを検証する開発者
- **損なわれる価値**: 資格情報による認証、セッション維持、未ログイン時のログイン画面提示、複数ユーザーでの検証
- **影響範囲・頻度**: `/authorize` 利用のたび(現状は全リクエストが自動承認)
- **回避策**: なし(仕様上の簡略化)

## 2. 現象(何が起きているか)

### 期待する動作

1. 未認証ユーザーが `/authorize` に到達するとログイン画面が表示される
2. 正しい資格情報で認証後、IdP セッションが確立される
3. 同一ブラウザセッション内の再 authorize は再ログインを省略できる(セッション存続時)
4. 認証成功後に consent(ISSUE-032)または認可コード発行へ進む

### 実際の動作

- `service.AuthorizationService.resolveOwner` が `login_hint` または既定 `demo-user` を自動割り当て
- `User.VerifyPassword` は未配線(ISSUE-005)
- ログイン HTML / IdP セッション Cookie なし

### 再現手順

1. ブラウザで `GET /auth/authorize?...` を実行
2. ログイン画面なしで即 redirect_uri へ 302 される

## 3. 原因(なぜ起きているか)

AUTH-001 計画で「基盤サンプルとして seed ユーザ自動割り当て」を意図的に採用。差し込み位置は `route/authorize_handler.go` と `resolveOwner` にコメント済み。

## 4. 対応(どう解決するか)

### 対応方針

- **ISSUE-005(パスワードハッシュ化)を先行**してから `VerifyPassword` を配線
- IdP セッションは HttpOnly / Secure(SameSite) Cookie + サーバー側セッション store(初期は Postgres テーブル or インメモリ+TTL)を検討
- ログイン UI は app/auth が HTML を返すか、app/web の `/auth/login` プロキシ配下に置くか planner が決定(SPEC-015 の nginx `/auth` 経路を活用可)

### 実施内容(チェックリスト)

- [x] ISSUE-005 解消(パスワードハッシュ + 定数時間比較)
- [x] `/authorize` 未ログイン時の login redirect(issuer 配下 `/auth/login`)
- [x] ログイン成功後の authorize 再開(state/PKCE 保持、pending cookie)
- [x] IdP セッション Cookie + in-memory store(TTL 24h)。ログアウトは ISSUE-033
- [x] route 統合テスト(`TestAuthorize_Unauthenticated_RedirectsToLogin` 等)
- [ ] Spec 起票または SPEC-015 後継 Spec でログインフロー要件を確定(最小実装は SPEC-015 スコープ外解消として経緯に記録)

### 関連

- ロードマップ: `docs/plans/AUTH-002-oauth-oidc-gap-roadmap-plan.md` Phase 1
- 依存: ISSUE-005 → 本 Issue → ISSUE-032

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。SPEC-015 完了後の OAuth/OIDC ギャップ整理の一環。AUTH-002 ロードマップ Phase 1.1。

### 2026-07-12 (解消)

- ISSUE-005 完了後に実装。`GET/POST /login` HTML、`AuthenticationService` + in-memory IdP session、`/authorize` 未ログイン時は `{issuer}/login` へ redirect。compose `DEMO_PASSWORD=demo` + `DEMO_LOG_PASSWORD=1`。`make -C app/auth check` green。status を `resolved` に更新。consent UI は ISSUE-032、IdP logout は ISSUE-033 へ。
