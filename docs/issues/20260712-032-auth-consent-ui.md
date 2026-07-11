---
id: ISSUE-032
title: app/auth 同意(consent) UI — スコープ承認画面の実装
status: open
severity: medium
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-032: app/auth 同意(consent) UI

## 1. ユーザー価値への影響(なぜ対応するか)

> **エンドユーザー** が **RP(client)に渡すスコープを理解し承認する** 体験が、**自動承認のままでは提供されない**。

- **影響を受けるユーザー**: OIDC RP にログインするユーザー
- **損なわれる価値**: 透明性(どのアプリが profile/email 等にアクセスするか)、OIDC Core の consent ステップ
- **影響範囲・頻度**: 初回 authorize またはスコープ変更時(現状は常にスキップ)
- **回避策**: なし

## 2. 現象(何が起きているか)

### 期待する動作

- 要求スコープを表示し、ユーザーが Accept / Deny できる
- 同一 client + 同一スコープ集合は consent 記録により再提示を省略可能
- Deny 時は OAuth error(`access_denied`)で redirect_uri へ返す

### 実際の動作

- authorize 成功時に即認可コード発行。同意画面なし

## 3. 原因(なぜ起きているか)

AUTH-001 で consent UI をスコープ外とし、authorize フローを最小化。

## 4. 対応(どう解決するか)

### 対応方針

- ISSUE-031(ログイン UI)完了後に着手
- consent 記録の永続化(ユーザー × client × scope ハッシュ)を新集約または既存 user/client 拡張で設計
- `prompt=consent` 対応は ISSUE-040 と連携可

### 実施内容(チェックリスト)

- [ ] consent 記録スキーマ(goose) + repository
- [ ] consent 画面(HTML または web 連携)
- [ ] Accept / Deny 分岐と OAuth エラーマッピング
- [ ] 再 authorize 時のスキップ条件
- [ ] テスト + review-spec

### 関連

- ロードマップ: AUTH-002 Phase 1.2
- 依存: ISSUE-031

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 1.2。
