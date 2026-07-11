---
id: ISSUE-037
title: OAuth リソースサーバー向け access token audience 設計(app/api 連携)
status: open
severity: medium
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-037: リソースサーバー向け access token audience 設計

## 1. ユーザー価値への影響(なぜ対応するか)

> **API 利用者・セキュリティレビュア** の **トークン用途の明確化** が、**access token の `aud=iss`(UserInfo 向け)を app/api が再利用している** 現状では OAuth ベストプラクティスから外れる。

- **影響を受けるユーザー**: app/api をリソースサーバーとして運用する開発者
- **損なわれる価値**: audience によるトークン用途分離(UserInfo 用 vs API 用)、漏洩トークンの blast radius 低減
- **影響範囲・頻度**: API 呼び出しのたび(現状は iss=aud 検証で通過)
- **回避策**: 現状の簡略検証(SPEC-015 で意図的に採用)

## 2. 現象(何が起きているか)

### 期待する動作

- access token の `aud` が API リソース識別子(例: `https://<host>/api` または resource indicator)を含む
- app/api は `AUTH_AUDIENCE` で aud を検証
- UserInfo 用 access token と API 用 token の分離、または single token with multiple aud(設計選択)

### 実際の動作

- `AuthorizationService` が access token に `aud=issuer` を設定(UserInfo 向け)
- app/api `AUTH_ISSUER` と同一値で aud 検証

## 3. 原因(なぜ起きているか)

AUTH-001 サンプル設計。SPEC-015 で横断 E2E を最短化するため aud 設計を据え置き。

## 4. 対応(どう解決するか)

### 対応方針

- OAuth 2.0 Resource Indicators(RFC 8707)または audience を API URL にする方式を planner が選択
- **破壊的変更**: web トークン、api 検証、auth 発行の三方同期 + OpenAPI/contract 再生成
- Spec 後継起票必須

### 実施内容(チェックリスト)

- [ ] audience 設計 Spec 確定
- [ ] app/auth token 発行変更
- [ ] app/api verifier `AUTH_AUDIENCE` 分離
- [ ] app/web 影響確認(通常は透過)
- [ ] contract drift 検査 green
- [ ] review-spec + review-security

### 関連

- ロードマップ: AUTH-002 Phase 3.2
- 依存: ISSUE-036 推奨(鍵ローテ中の検証)

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 3.2。
