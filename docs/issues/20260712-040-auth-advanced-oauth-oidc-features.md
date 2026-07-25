---
id: ISSUE-040
title: app/auth 高度 OAuth/OIDC 機能(introspection / DPoP / mTLS / prompt / CORS)
status: resolved
severity: low
created: 2026-07-12
updated: 2026-07-12
specs: []
---

# ISSUE-040: 高度 OAuth/OIDC 機能(バックログ)

## 1. ユーザー価値への影響(なぜ対応するか)

> **セキュリティ要件の高い RP やクロスオリジン構成** では、**現行 auth 基盤だけでは足りない標準機能** があり、**需要に応じて個別に実装** する必要がある。

本 Issue は単一機能ではなく **Phase 5 バックログ** の親 Issue とする。個別機能は着手時に経緯で切り出すか、子 Spec を起票する。

## 2. 対象機能(未実装一覧)

| 機能 | 規格 | 優先度(目安) | 備考 |
|---|---|---|---|
| Token Introspection | RFC 7662 `POST /introspect` | 低 | RS が JWT 検証以外で token 状態を問い合わせる場合 |
| DPoP | RFC 9449 | 低 | public client refresh 保護の代替。現状はローテーション+再利用検知 |
| mTLS 送信者制約 | RFC 8705 | 低 | エンタープライズ向け |
| `prompt` / `max_age` | OIDC Core | 低 | 再認証強制。ISSUE-031 後 |
| `ui_locales` | OIDC Core | 低 | i18n |
| CORS | — | 低 | auth を cross-origin 直呼びする構成時。SPEC-015 は同一オリジン proxy |
| PAR | RFC 9126 | 低 | 長大 authorize リクエスト |
| Device Authorization | RFC 8628 | 低 | CLI/TV 向け。需要時のみ |

## 3. 原因(なぜ起きているか)

AUTH-001 / SPEC-006 / SPEC-015 で SPA + Authorization Code + PKCE の最小経路にスコープを限定。

## 4. 対応(どう解決するか)

### 対応方針

- 需要とセキュリティレビューで **1 機能ずつ** Issue 分割または Spec 起票
- 本 Issue は **open のままバックログ** とし、着手した機能は経緯に記録して部分 close しない(全体は残す)

### 着手条件(例)

- Introspection: opaque access token 採用時、または RS が JWT 非対応の legacy 連携時
- DPoP/mTLS: confidential / 高セキュリティ RP 要求時
- CORS: web が `/auth` proxy を使わない構成に変更した場合

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 5 親 Issue。個別機能は需要驱动で Spec/Issue 分割。

### 2026-07-12 (resolved)

- 優先機能を実装: `prompt=login`/`prompt=none`、`max_age`、RFC 7662 `POST /introspect`。
- 残機能(DPoP/mTLS/PAR/CORS 等)は需要発生時に子 Issue 化。
- 検証: `REQUIRE_DB=1 make -C app/auth check` 緑。
