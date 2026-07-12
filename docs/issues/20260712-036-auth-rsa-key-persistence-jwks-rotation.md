---
id: ISSUE-036
title: app/auth RSA 署名鍵の永続化と JWKS 複数鍵ローテーション
status: resolved
severity: high
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]
---

# ISSUE-036: RSA 署名鍵の永続化と JWKS 複数鍵ローテーション

## 1. ユーザー価値への影響(なぜ対応するか)

> **本番運用者** の **トークン検証の継続性** が、**プロセス再起動のたびに RSA 鍵が再生成され全 JWT が無効になる** ことで損なわれる。

- **影響を受けるユーザー**: ECS デプロイ・ローリング再起動を行う運用者、app/api(JWKS 検証)利用者
- **損なわれる価値**: 無停止デプロイ中のトークン有効性、鍵ローテーション、複数 `kid` の JWKS
- **影響範囲・頻度**: auth コンテナ再起動のたび(compose / ECS)
- **回避策**: なし(現状は再起動 = 全トークン失効)

## 2. 現象(何が起きているか)

### 期待する動作

- 署名鍵が Secrets Manager / ボリューム等から読み込まれ再起動後も同一またはローテーション計画通りに継続
- JWKS に複数鍵を掲載し、旧鍵署名 JWT も検証可能な overlap 期間を持つ

### 実際の動作

- `cmd/authz/main.go` で起動時 `GenerateKey`、メモリのみ
- JWKS は単一 `kid`

## 3. 原因(なぜ起きているか)

AUTH-001 で「サンプル自己完結」を優先し秘密鍵永続化をスコープ外化。

## 4. 対応(どう解決するか)

### 対応方針

- 秘密鍵は **絶対にリポジトリに載せない**。AWS では Secrets Manager + ECS task secret、ローカルは compose secret ファイル(gitignore)
- app/api `infra/jwt` の JWKS キャッシュとローテーション overlap を設計
- multi-task auth は現状 desired_count=1 前提(IaC README) — 鍵共有が前提

### 実施内容(チェックリスト)

- [ ] 鍵ロードポート(`domain/token`) + infra 実装
- [ ] ローテーション手順(新鍵追加 → 署名切替 → 旧鍵 JWKS 残存 → 失効)
- [ ] compose / IaC env 契約
- [ ] 再起動後もトークン検証が継続する integration テスト
- [ ] review-security

### 関連

- ロードマップ: AUTH-002 Phase 2.2
- ブロッカー: 本番デプロイ前の必須項目

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。AUTH-002 ロードマップ Phase 2.2。

### 2026-07-12 (resolved)

- `SIGNING_KEYS_FILE` による鍵永続化、`KeyRing`/`MultiKeyVerifier`/`MultiKeyProvider`、JWKS 複数鍵、`cmd/keygen` + `make auth-signing-keys`。
- 検証: `REQUIRE_DB=1 make -C app/auth check` 緑(永続化 integration test 含む)。
